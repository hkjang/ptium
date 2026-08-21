package generation

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// Fallback builds a complete deck without contacting any AI provider. It is
// what an air-gapped deployment runs on, so it has to produce something a
// person would actually present: a real narrative arc, layout variety and
// speaker notes — not placeholder text.
func Fallback(presentation model.Presentation, profile model.Profile, template Template) Deck {
	count := presentation.RequestedSlideCount
	if count < 1 {
		count = 8
	}
	if count > 50 {
		count = 50
	}
	phrases := localizedCopy(presentation.Language)
	audience := strings.TrimSpace(presentation.Audience)
	if audience == "" {
		audience = phrases.DefaultAudience
	}
	// The prompt is read for what it asks about, then written as deck source and
	// compiled against the template — the same path a model's output takes, so
	// both produce a deck the template actually designed.
	outline := outlinePrompt(presentation.Prompt, presentation.Title, phrases)
	plan := newDeckPlan(outline, presentation, phrases, audience, presenterLine(profile))
	source := writeSource(outline, plan, count)

	result := CompileSource(source, presentation, profile, template)
	// Compiling always yields as many slides as the source has; if the source came
	// out short of the requested count the deck is still coherent, and saying so
	// is better than padding it with filler.
	return result
}

// CompileSource binds deck source to a template and returns a stored deck. It is
// the one path from source to slides, used by generation and by the editor.
func CompileSource(source string, presentation model.Presentation, profile model.Profile, template Template) Deck {
	return CompileSourceWithImages(source, presentation, profile, template, nil)
}

// CompileSourceWithImages is CompileSource with a resolver for the images a deck
// places. Generation has no access to the store, so the caller supplies one.
func CompileSourceWithImages(source string, presentation model.Presentation, profile model.Profile,
	template Template, resolveImage func(string) (deck.ContentImage, bool)) Deck {
	return CompileSourceWith(source, presentation, profile, template, resolveImage, nil)
}

// CompileGenerated is CompileSource for source the model wrote rather than a
// person: its @layout lines are suggestions, so a slide is moved to a layout that
// can hold it rather than losing a component or three points.
func CompileGenerated(source string, presentation model.Presentation, profile model.Profile, template Template) Deck {
	return compileSource(source, presentation, profile, template, nil, nil, true)
}

// CompileSourceWith is CompileSource with resolvers for the things a deck refers
// to but does not contain: images, and grid definitions.
func CompileSourceWith(source string, presentation model.Presentation, profile model.Profile,
	template Template, resolveImage func(string) (deck.ContentImage, bool),
	resolveGrid func(string) (pptx.GridSpec, bool)) Deck {
	return compileSource(source, presentation, profile, template, resolveImage, resolveGrid, false)
}

func compileSource(source string, presentation model.Presentation, profile model.Profile,
	template Template, resolveImage func(string) (deck.ContentImage, bool),
	resolveGrid func(string) (pptx.GridSpec, bool), generated bool) Deck {
	parsed := deck.ParseSource(source)
	compiled := deck.Compile(parsed, template.Manifest, deck.CompileOptions{
		Language:              presentation.Language,
		Accent:                func(position int) string { return profileAccent(profile, position) },
		ResolveImage:          resolveImage,
		ResolveGrid:           resolveGrid,
		LayoutsAreSuggestions: generated,
	})
	outlineJSON, err := json.Marshal(compiled.Outline)
	if err != nil {
		outlineJSON = json.RawMessage("[]")
	}
	return Deck{
		Source:   source,
		Slides:   compiled.Slides,
		Outline:  outlineJSON,
		Warnings: compiled.Warnings,
	}
}

func fillSlots(content *deck.Content, layout pptx.Layout, title, subtitle string, bullets []pptx.Paragraph) {
	if placeholder, ok := layout.Slot(pptx.SlotTitle); ok && strings.TrimSpace(title) != "" {
		content.SetField(pptx.SlotTitle, fitParagraphs([]pptx.Paragraph{{Text: title}}, placeholder))
	}
	if placeholder, ok := layout.Slot(pptx.SlotSubtitle); ok && strings.TrimSpace(subtitle) != "" {
		content.SetField(pptx.SlotSubtitle, fitParagraphs([]pptx.Paragraph{{Text: subtitle}}, placeholder))
	}
	bodies := layout.BodySlots()
	if len(bodies) == 0 || len(bullets) == 0 {
		if len(bullets) > 0 && subtitle == "" {
			if placeholder, ok := layout.Slot(pptx.SlotSubtitle); ok {
				content.SetField(pptx.SlotSubtitle, fitParagraphs(bullets[:1], placeholder))
			}
		}
		return
	}
	if len(bodies) == 1 {
		content.SetField(bodies[0].Slot, fitParagraphs(bullets, bodies[0]))
		return
	}
	// Spread the points evenly so a two-column layout never looks lopsided.
	perColumn := (len(bullets) + len(bodies) - 1) / len(bodies)
	if perColumn < 1 {
		perColumn = 1
	}
	for index, placeholder := range bodies {
		start := index * perColumn
		if start >= len(bullets) {
			break
		}
		end := start + perColumn
		if end > len(bullets) {
			end = len(bullets)
		}
		content.SetField(placeholder.Slot, fitParagraphs(bullets[start:end], placeholder))
	}
}

// slide stays prose.
func stageItems(points []string) ([]pptx.Item, bool) {
	if len(points) < 3 {
		return nil, false
	}
	items := make([]pptx.Item, 0, len(points))
	for _, point := range points {
		separator := strings.Index(point, ": ")
		if separator < 1 {
			return nil, false
		}
		stage := strings.TrimSpace(point[:separator])
		detail := strings.TrimSpace(point[separator+2:])
		if utf8.RuneCountInString(stage) > 14 || detail == "" {
			return nil, false
		}
		items = append(items, pptx.Item{Label: stage, Detail: detail})
	}
	return items, true
}

func applyBlock(content *deck.Content, layout pptx.Layout, block pptx.Block) {
	bodies := layout.BodySlots()
	if len(bodies) == 0 || bodies[0].MaxLines < pptx.BlockMinimumLines(block.Kind) {
		return
	}
	content.SetBlock(bodies[0].Slot, block)
}

func fallbackRoles(count int, manifest pptx.Manifest) []string {
	roles := make([]string, 0, count)
	roles = append(roles, pptx.RoleTitle)
	if count == 1 {
		return roles
	}
	closing := count >= 3
	body := count - 1
	if closing {
		body--
	}
	hasTwoContent := hasRole(manifest, pptx.RoleTwoContent)
	hasSection := hasRole(manifest, pptx.RoleSection)
	hasQuote := hasRole(manifest, pptx.RoleQuote)
	quoteAt := -1
	if hasQuote && body >= 5 {
		quoteAt = body * 2 / 3
	}
	for index := 0; index < body; index++ {
		switch {
		case index == quoteAt:
			roles = append(roles, pptx.RoleQuote)
		case hasSection && body >= 6 && index > 0 && index%4 == 0:
			roles = append(roles, pptx.RoleSection)
		case hasTwoContent && index > 0 && index%3 == 2:
			roles = append(roles, pptx.RoleTwoContent)
		default:
			roles = append(roles, pptx.RoleContent)
		}
	}
	if closing {
		roles = append(roles, pptx.RoleClosing)
	}
	return roles
}

func hasRole(manifest pptx.Manifest, role string) bool {
	for _, layout := range manifest.Layouts {
		if layout.Role == role {
			return true
		}
	}
	return false
}

// josa appends the Korean particle that agrees with the preceding word, so
// generated copy reads as written Korean rather than a template with holes.
// The choice depends on whether the last syllable ends in a consonant.
func josa(word, withFinal, withoutFinal string) string {
	word = strings.TrimSpace(word)
	if word == "" {
		return word
	}
	runes := []rune(word)
	last := runes[len(runes)-1]
	switch {
	case last >= 0xAC00 && last <= 0xD7A3:
		// Hangul syllables encode the final consonant in the low 28 values.
		if (last-0xAC00)%28 != 0 {
			return word + withFinal
		}
		return word + withoutFinal
	case last >= '0' && last <= '9':
		// Read by Korean digit names: 0,1,3,6,7,8 end in a consonant.
		if strings.ContainsRune("013678", last) {
			return word + withFinal
		}
		return word + withoutFinal
	}
	// Latin and other scripts: assume a trailing vowel is read openly.
	if strings.ContainsRune("aeiouAEIOU", last) {
		return word + withoutFinal
	}
	return word + withFinal
}

// ("… 계획을 임원진에게 보고") that reads badly inside another sentence.
func subjectPhrase(presentation model.Presentation, fallback string) string {
	if title := strings.TrimSpace(presentation.Title); title != "" && utf8.RuneCountInString(title) <= 40 {
		return title
	}
	prompt := strings.TrimSpace(presentation.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(presentation.Title)
	}
	if prompt == "" {
		return fallback
	}
	// Keep the leading clause, which normally names the subject.
	if index := strings.IndexAny(prompt, ".,;·\n"); index > 4 {
		prompt = prompt[:index]
	}
	words := strings.Fields(prompt)
	for len(words) > 0 && utf8.RuneCountInString(strings.Join(words, " ")) > 28 {
		words = words[:len(words)-1]
	}
	if joined := strings.TrimSpace(strings.Join(words, " ")); joined != "" {
		return joined
	}
	return truncate(prompt, 28)
}

// presenterLine is who is presenting, for the cover: an organisation and a role,
// never a biography.
func presenterLine(profile model.Profile) string {
	parts := make([]string, 0, 2)
	if company := strings.TrimSpace(profile.Company); company != "" {
		parts = append(parts, company)
	}
	if role := strings.TrimSpace(profile.JobTitle); role != "" {
		parts = append(parts, role)
	}
	return strings.Join(parts, " ")
}

// languageCopy holds what a deck needs before its prompt has been read: the
// words to use when the prompt names no subject and the request names no
// audience. Everything else a deck says is written in plan.go, from the topics
// the prompt actually named.
type languageCopy struct {
	DefaultTopic    string
	DefaultAudience string
}

func localizedCopy(language string) languageCopy {
	switch {
	case strings.HasPrefix(strings.ToLower(language), "ja"):
		return japaneseCopy
	case strings.HasPrefix(strings.ToLower(language), "zh"):
		return chineseCopy
	case strings.HasPrefix(strings.ToLower(language), "ko"), strings.TrimSpace(language) == "":
		return koreanCopy
	}
	return englishCopy
}

var koreanCopy = languageCopy{DefaultTopic: "제안 주제", DefaultAudience: "일반 청중"}

var englishCopy = languageCopy{DefaultTopic: "the proposal", DefaultAudience: "a general audience"}

var japaneseCopy = languageCopy{DefaultTopic: "提案テーマ", DefaultAudience: "一般の聴衆"}

var chineseCopy = languageCopy{DefaultTopic: "提案主题", DefaultAudience: "一般听众"}
