package generation

import (
	"encoding/json"
	"fmt"
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
	audience := audienceName(presentation.Audience, phrases)
	// The prompt is read for what it asks about, then written as deck source and
	// compiled against the template — the same path a model's output takes, so
	// both produce a deck the template actually designed.
	outline := outlinePrompt(presentation.Prompt, presentation.Title, phrases)
	plan := newDeckPlan(outline, presentation, phrases, audience, presenterLine(profile))
	source := writeSource(outline, plan, count)

	result := CompileSource(source, presentation, profile, template)
	// Compiling always yields as many slides as the source has. A brief with one
	// subject cannot honestly fill twenty slides: each subject is argued from four
	// angles and the closing questions are asked once, and past that a deck can
	// only repeat itself. Saying so is better than padding it with filler.
	if short := count - len(result.Slides); short > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"%d slide(s) were asked for and %d were written: the brief names %d subject(s), and stretching them further would repeat pages",
			count, len(result.Slides), len(outline.Topics)))
	}
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
	// Language is the two-letter code this copy is written in.
	Language        string
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

// audienceNames turn the keys an administrator stores into words a deck can say.
//
// A default is stored as "general" or "executive", and a cover that reads
// "general 보고" is a deck telling its reader about its own settings. Anything a
// person typed themselves is left exactly as written.
var audienceNames = map[string]map[string]string{
	"ko": {"general": "일반 청중", "executive": "경영진", "executives": "경영진", "board": "이사회",
		"practitioner": "실무진", "practitioners": "실무진", "technical": "기술 담당자",
		"customer": "고객사", "customers": "고객사", "investor": "투자자", "investors": "투자자",
		"student": "교육생", "students": "교육생", "internal": "사내 구성원"},
	"en": {"general": "a general audience", "executive": "the executive team", "executives": "the executive team",
		"board": "the board", "practitioner": "the working team", "practitioners": "the working team",
		"technical": "the engineering team", "customer": "the customer", "customers": "customers",
		"investor": "investors", "investors": "investors", "student": "learners", "students": "learners",
		"internal": "the company"},
	"ja": {"general": "一般の聴衆", "executive": "経営層", "practitioner": "実務担当者", "investor": "投資家"},
	"zh": {"general": "一般听众", "executive": "管理层", "practitioner": "业务负责人", "investor": "投资人"},
}

// audienceName is who the deck addresses, in words.
func audienceName(audience string, phrases languageCopy) string {
	audience = strings.TrimSpace(audience)
	if audience == "" {
		return phrases.DefaultAudience
	}
	if named, ok := audienceNames[phrases.Language][strings.ToLower(audience)]; ok {
		return named
	}
	// A key nobody translated is still not something to print. Anything with a
	// space or a non-Latin letter is a phrase a person wrote.
	if isSettingKey(audience) {
		return phrases.DefaultAudience
	}
	return audience
}

// isSettingKey reports whether a value looks like a stored key rather than words
// someone wrote: one lowercase Latin word, no spaces.
func isSettingKey(value string) bool {
	if strings.ContainsAny(value, " ·,·") {
		return false
	}
	for _, letter := range value {
		if letter > 127 {
			return false
		}
		if !(letter >= 'a' && letter <= 'z') && letter != '-' && letter != '_' {
			return false
		}
	}
	return value != ""
}

var koreanCopy = languageCopy{Language: "ko", DefaultTopic: "제안 주제", DefaultAudience: "일반 청중"}

var englishCopy = languageCopy{Language: "en", DefaultTopic: "the proposal", DefaultAudience: "a general audience"}

var japaneseCopy = languageCopy{Language: "ja", DefaultTopic: "提案テーマ", DefaultAudience: "一般の聴衆"}

var chineseCopy = languageCopy{Language: "zh", DefaultTopic: "提案主题", DefaultAudience: "一般听众"}
