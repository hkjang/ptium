package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/library"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/settings"
)

type SettingReader interface {
	Get(ctx context.Context, key string, target any) error
}

type Generator struct {
	settings SettingReader
	client   *http.Client
	// maxOutputTokens bounds one completion. A deck's source is a few thousand
	// tokens; without a bound a reasoning model will spend the whole context on
	// thinking, and with too small a bound the deck arrives truncated.
	maxOutputTokens int
	// outputTokensSet marks a budget an administrator chose. Theirs is used as
	// it stands; the default is stretched to the deck being asked for.
	outputTokensSet bool
	// thinkingRejected remembers a provider that refuses the thinking switch, so
	// the next run does not spend a round trip learning it again. It is shared
	// between runs on purpose — it is about the provider, not about one deck —
	// and it is the only thing that is.
	thinkingRejected *atomic.Bool
	// reasoning says whether to ask the provider not to think.
	reasoning reasoningMode
	// repairs bounds how many slides a generation may send back to the model
	// after measuring them. Zero turns the repair pass off.
	repairs int
	// Library reads the slides this owner registered, and Used records that one
	// was put into a deck. A company's fixed slides — the introduction, the org
	// chart, the security architecture — are already written and agreed, and a
	// deck that writes its own version of them is how a company's decks drift
	// apart. Both are optional: without them nothing changes.
	Library func(ctx context.Context, ownerID string) []library.Entry
	// Pictures is what this account has already uploaded, most used first. A
	// deck may place one; it may not invent one, and a name nobody uploaded is
	// reported rather than drawn.
	Pictures func(ctx context.Context, ownerID string) []string
	// ResolveImage turns a name a deck wrote into the stored picture it means.
	// Without it a deck can name a picture and not carry one: the source says
	// ::image 현장 자동화 and the slide is drawn without it.
	ResolveImage func(ctx context.Context, ownerID, reference string) (deck.ContentImage, bool)
	// Reached says which pass a generation is in. A self-hosted model takes a
	// minute or three to write a deck, and a screen that says the same sentence
	// for all of it cannot be told from one that has stopped.
	Reached func(ctx context.Context, presentationID, stage string)
	Used    func(ctx context.Context, ownerID, snippetID string)
	// Now is what day it is, for the brief. A model has no clock, and a brief
	// that says "하반기" without a year makes it guess one — a deck written this
	// week came back titled 2024. Injected so a test can hold the date still.
	Now func() time.Time
}

// Deck is the generator's output: the outline shown in the workspace plus the
// slides persisted for editing and export.
type Deck struct {
	Outline json.RawMessage `json:"outline"`
	Slides  []model.Slide   `json:"slides"`
	// Source is the deck as written in Ptium's slide language. It is what the
	// editor shows and edits, and recompiling it reproduces the slides exactly,
	// so the text is the deck rather than a description of it.
	Source string `json:"source,omitempty"`
	// Warnings record what compiling adjusted — a layout that does not exist, a
	// component that did not fit — without failing a generation someone is
	// waiting for. They are written for whoever is reading the deck source or
	// the server log.
	Warnings []string `json:"warnings,omitempty"`
	// Notes are for the person who asked: what the deck does differently from
	// what was requested, in the language the deck is written in. A deck that
	// comes back shorter than the count asked for has to say so on the screen,
	// not in a log nobody reads.
	Notes []string `json:"notes,omitempty"`
}

// Template is the design a deck is written into. The manifest tells the model
// which layouts exist and how much text each slot can hold, which is what
// keeps generated copy inside the customer's own design.
type Template struct {
	ID       string
	Name     string
	Manifest pptx.Manifest
}

// Defaults for a self-hosted provider, which is what an air-gapped deployment
// has. A 100B-class model on quantised hardware answers in tens of seconds, so a
// two-minute timeout — a reasonable default for a hosted API — cuts off a deck
// that was on its way.
const (
	defaultRequestTimeout = 5 * time.Minute
	defaultOutputTokens   = 8000
)

func New(settings SettingReader) *Generator {
	return &Generator{
		settings:         settings,
		client:           &http.Client{Timeout: defaultRequestTimeout},
		maxOutputTokens:  defaultOutputTokens,
		thinkingRejected: &atomic.Bool{},
		reasoning:        reasoningAuto,
		repairs:          maximumRepairs,
		Now:              time.Now,
	}
}

// Generate produces a deck for a presentation. It plans the narrative first
// and writes slide copy second, so the result reads like a deck a consultant
// would build rather than a list of bullet points.
func (g *Generator) Generate(ctx context.Context, presentation model.Presentation, profile model.Profile, template Template) (Deck, error) {
	return g.generate(ctx, presentation, profile, template, false)
}

// Rewrite improves a deck that already exists rather than writing a new one.
//
// Everything in it is the author's — a deck brought in from a file, or one they
// wrote here — so the facts are kept and the craft is what changes. Without an
// AI provider there is nothing to do: writing a fresh deck from the brief would
// throw away the very thing being improved, so this says so instead.
func (g *Generator) Rewrite(ctx context.Context, presentation model.Presentation, profile model.Profile, template Template) (Deck, error) {
	if strings.TrimSpace(presentation.Source) == "" {
		return Deck{}, errors.New("this deck has no text to rewrite")
	}
	return g.generate(ctx, presentation, profile, template, true)
}

func (g *Generator) generate(ctx context.Context, presentation model.Presentation, profile model.Profile, template Template, rewrite bool) (Deck, error) {
	profile = g.withDefaultBrand(ctx, profile)
	if len(template.Manifest.Layouts) == 0 {
		return Deck{}, errors.New("the selected template does not expose any usable layout")
	}
	if presentation.RequestedSlideCount < 1 || presentation.RequestedSlideCount > 50 {
		return Deck{}, fmt.Errorf("requested slide count %d is outside the supported range 1-50", presentation.RequestedSlideCount)
	}

	provider, baseURL, modelName, apiKey := "fallback", "https://api.openai.com/v1", "gpt-4.1-mini", ""
	_ = g.settings.Get(ctx, "ai.provider", &provider)
	_ = g.settings.Get(ctx, "ai.base_url", &baseURL)
	_ = g.settings.Get(ctx, "ai.model", &modelName)
	_ = g.settings.Get(ctx, "ai.api_key", &apiKey)
	g = g.forRun(ctx, presentation.RequestedSlideCount)
	// An empty key used to mean "no provider", which is the one thing it cannot
	// mean on the deployments this product is built for: a model on a closed
	// network is reached without a key at all. A site that had named its host
	// and its model, and could see them on the admin screen, still got the
	// offline writer — and the deck it got said nothing about that.
	if strings.EqualFold(provider, "fallback") {
		if rewrite {
			// Rewriting is the one thing the offline writer cannot stand in for: it
			// would replace the author's deck with a new one about the brief.
			return Deck{}, errors.New("rewriting a deck needs an AI provider; ask an administrator to configure one")
		}
		// A deployment with no model writes a frame, and says so. Everything else
		// this product does tells the person what it did; this was the one place
		// it wrote scaffolding and let them assume it was the answer.
		written := g.fromLibrary(ctx, presentation, profile, template,
			g.fallbackDeck(ctx, presentation, profile, template))
		if phrases := localizedCopy(presentation.Language); phrases.NoModelNote != nil {
			note := phrases.NoModelNote()
			written.Notes = append([]string{note}, written.Notes...)
			written.Warnings = append([]string{note}, written.Warnings...)
		}
		return written, nil
	}
	if provider != "openai-compatible" && provider != "openai" {
		return Deck{}, fmt.Errorf("unsupported AI provider %q", provider)
	}

	endpoint, err := completionsEndpoint(baseURL)
	if err != nil {
		return Deck{}, err
	}
	request := writingRequest{Presentation: presentation, Profile: profile, Template: template, Today: g.today()}
	request.Registered = registeredNames(ctx, g.Library, presentation.OwnerID)
	request.Pictures = pictureNames(ctx, g.Pictures, presentation.OwnerID)
	// A deck that already has slides is being rewritten, not invented. Its own
	// text is the material, its structure is already decided, and planning a new
	// narrative for it would throw away the thing being improved.
	if material := strings.TrimSpace(presentation.Source); material != "" && rewrite {
		request.Material = material
		request.Instruction = presentation.RewriteInstruction
		written, err := g.writeDeck(ctx, endpoint, modelName, apiKey, request)
		if err != nil {
			return Deck{}, err
		}
		return g.fromLibrary(ctx, presentation, profile, template, written), nil
	}
	outlinePass := true
	_ = g.settings.Get(ctx, "generation.outline_pass", &outlinePass)
	planNote := ""
	if outlinePass && presentation.RequestedSlideCount > 2 {
		g.reached(ctx, presentation.ID, StagePlanning)
		plan, err := g.plan(ctx, endpoint, modelName, apiKey, request)
		if err != nil {
			// The plan is an aid, not a requirement. Whatever went wrong in this
			// first pass — the clock ran out, or the model answered with something
			// that is not a plan — the second pass can still write the deck, and
			// giving up here hands it to the offline writer when the model the
			// customer chose could have written it.
			//
			// Nothing is hidden by carrying on: a provider that is misconfigured
			// fails the writing pass a moment later, with the same cause, and that
			// failure is reported.
			planNote = AuthorMessage(err, presentation.Language)
		} else {
			request.Plan = plan
		}
	}
	g.reached(ctx, presentation.ID, StageWriting)
	written, err := g.writeDeck(ctx, endpoint, modelName, apiKey, request)
	if err != nil {
		return g.withoutTheModel(ctx, presentation, profile, template, err)
	}
	g.reached(ctx, presentation.ID, StageBinding)
	result := g.fromLibrary(ctx, presentation, profile, template, written)
	if planNote != "" {
		result.Warnings = append(result.Warnings, "the narrative pass was skipped: "+planNote)
		if phrases := localizedCopy(presentation.Language); phrases.NoPlanNote != nil {
			result.Notes = append(result.Notes, phrases.NoPlanNote(planNote))
		}
	}
	return result, nil
}

// withoutTheModel writes the deck offline when the model could not write it.
//
// Somebody asked for a deck and waited five minutes; handing them a failure
// screen leaves them with nothing, and the offline writer — the one an
// air-gapped deployment runs on — produces a deck a person can actually
// present. So it stands in, and the deck says so: what happened, and that
// trying again puts the model back on it.
//
// Not for every failure. A template that will not load stops both writers, and
// a rewrite must never be replaced by a deck written from the brief — that is
// the one case where standing in would throw away the author's own work.
func (g *Generator) withoutTheModel(ctx context.Context, presentation model.Presentation,
	profile model.Profile, template Template, cause error) (Deck, error) {
	if !modelCouldNotAnswer(cause) {
		return Deck{}, cause
	}
	result := g.fromLibrary(ctx, presentation, profile, template,
		g.fallbackDeck(ctx, presentation, profile, template))
	if len(result.Slides) == 0 {
		return Deck{}, cause
	}
	phrases := localizedCopy(presentation.Language)
	if phrases.ModelStoodDownNote != nil {
		result.Notes = append(result.Notes, phrases.ModelStoodDownNote(AuthorMessage(cause, presentation.Language)))
	}
	result.Warnings = append(result.Warnings, "the model did not write this deck: "+cause.Error())
	return result, nil
}

// modelCouldNotAnswer separates a model that was not available from one that is
// set up wrong.
//
// Not available — it timed out, the host refused the connection, it was too busy
// — is a moment in time: the author should get their deck and the model gets the
// next one. Set up wrong — the key is rejected, the model returns reasoning and
// no answer, the endpoint answers something that is not a completion — is a
// thing an administrator has to fix, and it has to reach them. Standing in for
// that failure would hide it: generation would succeed, no incident would be
// recorded, and every deck would quietly come out written offline.
func modelCouldNotAnswer(cause error) bool {
	if cause == nil {
		return false
	}
	var rejected rejectedRequest
	if errors.As(cause, &rejected) {
		// Too busy, or broken at their end. A rejected key is not this.
		return rejected.status == 429 || rejected.status >= 500
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return true
	}
	text := strings.ToLower(cause.Error())
	for _, mark := range []string{
		"timeout", "deadline exceeded", "connection refused", "no such host",
		"dial tcp", "network is unreachable", "connection reset", "eof",
	} {
		if strings.Contains(text, mark) {
			return true
		}
	}
	return false
}

// today is the date the brief carries, so a deck about "the second half" is
// about this year's second half.
func (g *Generator) today() string {
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	return now().Format("2006-01-02")
}

// registeredNames lists what this owner has in the slide library, for the brief
// to offer the model. A long library is trimmed: the point is to remind a deck
// that the standard slides exist, not to recite the whole shelf.
func registeredNames(ctx context.Context, read func(context.Context, string) []library.Entry, ownerID string) []string {
	if read == nil || strings.TrimSpace(ownerID) == "" {
		return nil
	}
	var names []string
	for _, entry := range read(ctx, ownerID) {
		if name := strings.TrimSpace(entry.Name); name != "" {
			names = append(names, name)
		}
		if len(names) >= 20 {
			break
		}
	}
	return names
}

// fallbackDeck writes a deck without a model, with whatever pictures the
// account has to illustrate it.
func (g *Generator) fallbackDeck(ctx context.Context, presentation model.Presentation,
	profile model.Profile, template Template) Deck {
	return FallbackWithPictures(presentation, profile, template,
		pictureEntries(ctx, g.Pictures, presentation.OwnerID), g.imageResolver(ctx, presentation.OwnerID))
}

// imageResolver binds a picture's name to the picture, for the account this deck
// belongs to.
func (g *Generator) imageResolver(ctx context.Context, ownerID string) func(string) (deck.ContentImage, bool) {
	if g.ResolveImage == nil || strings.TrimSpace(ownerID) == "" {
		return nil
	}
	return func(reference string) (deck.ContentImage, bool) {
		return g.ResolveImage(ctx, ownerID, reference)
	}
}

// pictureEntries is the account's pictures as the library matches names.
// Every one of them, not the first two dozen: matching a name against a name
// costs nothing, and the picture a deck wants is as likely to be the two
// hundredth uploaded as the first.
func pictureEntries(ctx context.Context, read func(context.Context, string) []string, ownerID string) []library.Entry {
	if read == nil || strings.TrimSpace(ownerID) == "" {
		return nil
	}
	names := read(ctx, ownerID)
	entries := make([]library.Entry, 0, len(names))
	for _, name := range names {
		entries = append(entries, library.Entry{Name: name})
	}
	return entries
}

// maximumOfferedPictures bounds the list a model is given. Every name is prompt
// a deck pays for, and a model handed two hundred of them is being asked to
// browse rather than to write.
const maximumOfferedPictures = 24

// The passes a deck goes through, in the order it goes through them. The names
// are keys rather than sentences: the workspace writes them in the reader's
// language, and a stage stored as prose would be stored in whichever language
// the deployment was speaking when it ran.
const (
	StagePlanning = "planning"
	StageWriting  = "writing"
	StageFitting  = "fitting"
	StageNotes    = "notes"
	StageBinding  = "binding"
)

// reached records a pass, if anybody is listening.
func (g *Generator) reached(ctx context.Context, presentationID, stage string) {
	if g.Reached == nil || strings.TrimSpace(presentationID) == "" {
		return
	}
	g.Reached(ctx, presentationID, stage)
}

// pictureNames is what this account can illustrate a deck with.
//
// Ptium cannot fetch a picture — an air-gapped deployment has no web to fetch
// from — so the pictures a deck can carry are the ones somebody already
// uploaded. Naming them is what lets a deck use them: the same thing that made
// the slide library work, where a model left to itself never guessed the name.
func pictureNames(ctx context.Context, read func(context.Context, string) []string, ownerID string) []string {
	if read == nil || strings.TrimSpace(ownerID) == "" {
		return nil
	}
	var names []string
	for _, name := range read(ctx, ownerID) {
		if trimmed := strings.TrimSpace(name); saysWhatItIs(trimmed) {
			names = append(names, trimmed)
		}
		if len(names) >= maximumOfferedPictures {
			break
		}
	}
	return names
}

// saysWhatItIs keeps the pictures whose names tell a model what is in them.
//
// The list is names and nothing else — a model cannot look at the pictures —
// so a name that says nothing is an invitation to guess. Importing a deck
// names its pictures after the file they came from: "해커톤 최종발표 ·
// image22.png" is a screenshot of something, and a live model put that exact
// picture on an English warehouse deck and again on a Japanese quarterly one.
// A camera's own name — IMG_4821.jpg — is the same problem.
func saysWhatItIs(name string) bool {
	said := strings.TrimSpace(name)
	if said == "" {
		return false
	}
	// An imported picture is "<the deck it came from> · <its part name>", and
	// the part name is what it would be offered as.
	if _, part, found := strings.Cut(said, " · "); found {
		said = strings.TrimSpace(part)
	}
	if index := strings.LastIndex(said, "."); index > 0 && len(said)-index <= 5 {
		said = said[:index]
	}
	// What is left after the counter, the date and the punctuation that holds
	// them on: a name that is only those is a file, not a subject.
	said = strings.TrimSpace(strings.Trim(said, "0123456789 -_()[]#·."))
	if said == "" {
		return false
	}
	return !genericPictureName.MatchString(said)
}

// genericPictureName is what a picture is called when nobody named it.
var genericPictureName = regexp.MustCompile(`(?i)^(image|img|imgp|photo|picture|pic|screenshot|screen ?shot|capture|untitled|파일|사진|그림|이미지|스크린샷|캡처|무제)$`)

// fromLibrary puts the owner's registered slides into a deck that wrote its own
// versions of them.
//
// The substitution happens on the source, before anything is drawn, so a
// registered slide arrives the way it was written and is compiled into this
// deck's template like every other slide. What was replaced is said out loud:
// a deck that quietly swapped a slide would be worse than one that did not.
func (g *Generator) fromLibrary(ctx context.Context, presentation model.Presentation,
	profile model.Profile, template Template, written Deck) Deck {
	if g.Library == nil || strings.TrimSpace(written.Source) == "" {
		return written
	}
	entries := g.Library(ctx, presentation.OwnerID)
	if len(entries) == 0 {
		return written
	}
	source, used := library.Substitute(written.Source, entries)
	if len(used) == 0 {
		return written
	}
	rebuilt := CompileGenerated(source, presentation, profile, template)
	if len(rebuilt.Slides) == 0 {
		// The library slide did not compile into this template. Keeping the deck
		// that does is better than a deck that does not.
		return written
	}
	for _, entry := range used {
		if g.Used != nil {
			g.Used(ctx, presentation.OwnerID, entry.ID)
		}
		rebuilt.Warnings = append(rebuilt.Warnings,
			fmt.Sprintf("%q 슬라이드는 라이브러리에 등록된 %q을(를) 그대로 썼습니다", entry.Title, entry.Name))
	}
	rebuilt.Warnings = append(rebuilt.Warnings, written.Warnings...)
	return rebuilt
}

// budgetForDeck stretches the output budget to the deck being asked for.
//
// Eight thousand tokens was chosen when a deck's source was the whole answer.
// Asked for the plan of a nine-slide deck, the model this was measured against
// spent 6,495 completion tokens to write 2,634 characters — four fifths of the
// budget went on thinking, and 81% of the default was gone on the smallest deck
// anyone asks for. A little more brief, a little more thinking, and the answer
// stops mid-sentence: half a plan is not JSON and half a deck is a deck missing
// its last slides.
//
// Tokenised with the model's own tokeniser, the decks written here run 90 to
// 116 tokens a slide, so the room the answer itself needs is small and the room
// the thinking needs is not. The floor covers the thinking; the slope covers
// the deck. A budget an administrator set is theirs and is left alone.
func (g *Generator) budgetForDeck(slides int) {
	if g.outputTokensSet || slides <= 0 {
		return
	}
	wanted := defaultOutputTokens + slides*250
	if wanted > g.maxOutputTokens {
		g.maxOutputTokens = min(wanted, 32000)
	}
}

// forRun is this generator, set up for one deck.
//
// One Generator serves the whole server, and the settings a request reads —
// the timeout, the output budget, whether to ask the provider not to think —
// used to be written onto it. Two requests at once wrote over each other: the
// race detector says so, and the consequence is worse than the warning. A
// fifty-slide deck stretches the budget to twenty thousand tokens; a slide
// rewrite starting a moment later put it back to eight thousand, and the deck
// in flight was cut off mid-answer.
//
// So a run takes a copy. The http client is copied too — its timeout is a
// field, and changing it under another request in flight is the same bug —
// while the transport, which holds the connections, is shared.
func (g *Generator) forRun(ctx context.Context, slides int) *Generator {
	run := *g
	run.client = &http.Client{Timeout: g.client.Timeout, Transport: g.client.Transport}
	run.applyProviderSettings(ctx)
	run.budgetForDeck(slides)
	return &run
}

// applyProviderSettings reads the knobs a self-hosted provider needs.
func (g *Generator) applyProviderSettings(ctx context.Context) {
	// The bounds live with the settings, so what is stored and what is honoured
	// cannot come apart: the API refuses anything outside them.
	seconds := int(defaultRequestTimeout / time.Second)
	if _ = g.settings.Get(ctx, "ai.timeout_seconds", &seconds); settings.Numbers["ai.timeout_seconds"].Holds(seconds) {
		g.client.Timeout = time.Duration(seconds) * time.Second
	}
	tokens := defaultOutputTokens
	g.outputTokensSet = false
	if _ = g.settings.Get(ctx, "ai.max_output_tokens", &tokens); settings.Numbers["ai.max_output_tokens"].Holds(tokens) {
		g.maxOutputTokens = tokens
		g.outputTokensSet = tokens != defaultOutputTokens
	}
	repairs := maximumRepairs
	if _ = g.settings.Get(ctx, "generation.repair_passes", &repairs); settings.Numbers["generation.repair_passes"].Holds(repairs) {
		g.repairs = repairs
	}
	mode := string(reasoningAuto)
	_ = g.settings.Get(ctx, "ai.reasoning", &mode)
	if g.thinkingRejected != nil && g.thinkingRejected.Load() && strings.EqualFold(mode, string(reasoningAuto)) {
		// Already learned, on a previous run, that this provider will not take it.
		mode = string(reasoningOn)
	}
	switch reasoningMode(strings.ToLower(strings.TrimSpace(mode))) {
	case reasoningOff:
		g.reasoning = reasoningOff
	case reasoningOn:
		g.reasoning = reasoningOn
	default:
		g.reasoning = reasoningAuto
	}
}

// completeSource asks for the deck, and asks the provider not to think.
//
// The order matters. Detecting a reasoning model from its answer requires the
// answer to arrive, and a large model thinking through its whole output budget
// takes longer than any sane timeout — so waiting to find out costs a full
// timeout and produces nothing. Asking not to think costs, at worst, one
// immediate rejection from a hosted API that does not know the field.
func (g *Generator) completeSource(ctx context.Context, endpoint, modelName, apiKey, system, user string,
	temperature float64) (string, error) {
	return g.completeWith(ctx, endpoint, modelName, apiKey, system, user, temperature, nil)
}

// completeWith is that order, once, for every pass that talks to the model.
//
// It used to be written out for the pass that writes the deck and not for the
// pass that plans it, so the planning pass never asked the provider not to
// think. Measured against a self-hosted reasoning model, that one omission cost
// 302 seconds and 6,495 tokens for an answer the same model gives in 65 seconds
// and 1,349 tokens when asked not to think — right up against the five-minute
// timeout and four fifths of the output budget. The plan was routinely cut off
// or timed out, and the deck was written with no design behind it. The setting
// an administrator chose was being honoured at one door and ignored at the
// other, so now there is one door.
func (g *Generator) completeWith(ctx context.Context, endpoint, modelName, apiKey, system, user string,
	temperature float64, format map[string]string) (string, error) {
	quiet := g.reasoning != reasoningOn
	raw, err := g.send(ctx, endpoint, modelName, apiKey, system, user, temperature, format, quiet)
	if err == nil {
		return raw, nil
	}
	// The provider thought instead of answering. Ask once more, plainly.
	if errors.Is(err, errReasonedWithoutAnswering) {
		return g.send(ctx, endpoint, modelName, apiKey, system+plainlyPrompt, noThinkPrefix+user,
			temperature, format, quiet)
	}
	if !quiet || g.reasoning != reasoningAuto {
		return "", err
	}
	var rejected rejectedRequest
	if errors.As(err, &rejected) && rejected.status >= 400 && rejected.status < 500 {
		// The provider does not take the thinking switch. Ask again without it,
		// stop sending it for the rest of this run, and remember it for the next.
		g.reasoning = reasoningOn
		if g.thinkingRejected != nil {
			g.thinkingRejected.Store(true)
		}
		return g.send(ctx, endpoint, modelName, apiKey, system, user, temperature, format, false)
	}
	return "", err
}

// plan asks the model to design the deck before writing any copy.
func (g *Generator) plan(ctx context.Context, endpoint, modelName, apiKey string, request writingRequest) (*deckPlan, error) {
	raw, err := g.complete(ctx, endpoint, modelName, apiKey, planSystemPrompt, planUserPrompt(request), 0.5)
	if err != nil {
		return nil, err
	}
	var plan deckPlan
	if err := json.Unmarshal(planJSON(raw), &plan); err != nil {
		// The answer itself, cut short: without it the next person to see this can
		// only guess what the model said, which is how a fenced outline went
		// undiagnosed. The author sees the plain explanation, not this.
		return nil, fmt.Errorf("AI provider returned an outline that could not be read: %s",
			truncate(strings.Join(strings.Fields(raw), " "), 160))
	}
	if len(plan.Slides) == 0 {
		return nil, errors.New("AI provider returned an empty outline")
	}
	return &plan, nil
}

// planJSON is the object in a model's answer.
//
// The planning pass asks for JSON and nothing else, and a model mostly obliges
// — but a run against a self-hosted model came back with the outline wrapped in
// a fence, the whole pass was thrown away, and the deck was written with no
// design behind it. What was asked for was there and unread. So the fence and
// whatever is said around the object are removed before it is read, and only an
// answer with no object in it at all is a failure.
func planJSON(raw string) []byte {
	trimmed := strings.TrimSpace(raw)
	if fence := strings.Index(trimmed, "```"); fence >= 0 {
		rest := trimmed[fence+3:]
		if line := strings.IndexByte(rest, '\n'); line >= 0 && !strings.Contains(rest[:line], "{") {
			rest = rest[line+1:]
		}
		if end := strings.Index(rest, "```"); end >= 0 {
			rest = rest[:end]
		}
		trimmed = strings.TrimSpace(rest)
	}
	start := strings.IndexByte(trimmed, '{')
	end := strings.LastIndexByte(trimmed, '}')
	if start < 0 || end <= start {
		return []byte(trimmed)
	}
	return []byte(trimmed[start : end+1])
}

// writeDeck asks the model for the deck and binds it to the template.
//
// The model writes Ptium's slide language, which is one construct per line: far
// less to get wrong than a nested schema, and readable by the person who has to
// check it. A provider that answers with the older JSON shape is still accepted,
// so a deployment pinned to a tuned model keeps working.
func (g *Generator) writeDeck(ctx context.Context, endpoint, modelName, apiKey string, request writingRequest) (Deck, error) {
	started := time.Now()
	system := sourceSystemPrompt + exampleDeck(request.Presentation.Language)
	if strings.TrimSpace(request.Material) != "" {
		system = rewriteSystemPrompt
	}
	raw, err := g.completeSource(ctx, endpoint, modelName, apiKey, system, sourceUserPrompt(request), 0.35)
	if err != nil {
		return Deck{}, err
	}
	writing := time.Since(started)
	source := cleanModelSource(raw, request.Presentation.Language)
	// A citation the brief cannot support is worse than none: it is printed at
	// the foot of the slide and read as evidence.
	source, invented, vague := keepAttributedSources(source, request.Presentation.Prompt+" "+
		request.Presentation.Title+" "+request.Material)
	if strings.HasPrefix(source, "{") {
		var written writtenDeck
		if json.Unmarshal([]byte(source), &written) == nil && len(written.Slides) > 0 {
			composed, err := compose(request, written)
			if err != nil {
				return Deck{}, err
			}
			// The JSON shape is another way of saying the same deck, so it goes
			// through the same doors. It used to return here, which meant a
			// deployment pinned to a model that answers in JSON got no repair
			// pass, no word about invented figures, and no word about a source
			// the brief named and the deck ignored.
			return g.finishDeck(ctx, request, composed, composed.Source, invented, vague, writing)
		}
	}
	parsed := deck.ParseSource(source)
	if len(parsed.Slides) == 0 {
		return Deck{}, errors.New("AI provider returned a deck without slides")
	}
	result := CompileGenerated(source, request.Presentation, request.Profile, request.Template)
	return g.finishDeck(ctx, request, result, source, invented, vague, writing)
}

// finishDeck is everything that happens to a written deck between the model's
// answer and the author's screen: what the deck says about what was invented or
// left uncited, the pass that measures it against the template and asks for the
// slides that do not fit to be written again, and the count the author asked
// for.
func (g *Generator) finishDeck(ctx context.Context, request writingRequest, result Deck,
	source string, invented, vague int, writing time.Duration) (Deck, error) {
	if invented > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("the model invented %d source(s) the brief does not mention", invented))
		result.Notes = append(result.Notes, inventedSourceNote(invented, request.Presentation.Language))
	}
	// The brief said where its figures came from and the deck cites nothing: the
	// author supplied the one thing a room asks for and it went unused.
	if !strings.Contains(source, "!source") && !strings.Contains(source, "!출처") &&
		BriefNamesASource(request.Presentation.Prompt+" "+request.Material) {
		result.Warnings = append(result.Warnings, "the brief names a source and no slide cites it")
		result.Notes = append(result.Notes, uncitedBriefNote(request.Presentation.Language))
	}
	if vague > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("the model gave %d locator(s) the brief does not; the source names were kept", vague))
		result.Notes = append(result.Notes, vagueLocatorNote(vague, request.Presentation.Language))
	}
	if len(result.Slides) == 0 {
		return Deck{}, errors.New("the deck the AI provider wrote could not be bound to this template")
	}
	// The deck is measured against the template before it is handed over, and the
	// slides that do not fit are sent back to the model with the measurement.
	if g.repairs > 0 {
		g.reached(ctx, request.Presentation.ID, StageFitting)
		result = g.repairDeck(ctx, request, result, writing)
		// A slide can fit its region perfectly and still have nothing written to
		// say over it, which no measurement of the drawing will ever catch.
		g.reached(ctx, request.Presentation.ID, StageNotes)
		result = g.writeMissingNotes(ctx, request, result, writing/2)
	}
	// A figure the brief never gave cannot be cut out of the sentence it sits in,
	// so the deck keeps it and names it. The room will ask about it. Measured
	// after repair, because a rewrite states figures too.
	if figures := figuresNotInBrief(deckSourceOf(result, source), request.Presentation.Prompt+" "+
		request.Presentation.Title+" "+request.Material); len(figures) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("the model states %d figure(s) the brief does not: %s",
				len(figures), strings.Join(figures, ", ")))
		result.Notes = append(result.Notes, inventedFigureNote(figures, request.Presentation.Language))
	}
	if requested := request.Presentation.RequestedSlideCount; requested > 0 && len(result.Slides) != requested {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("the model wrote %d slides for a request of %d", len(result.Slides), requested))
		// The person who asked for twenty slides and is looking at nine should be
		// told by the deck, not by a log.
		if phrases := localizedCopy(request.Presentation.Language); len(result.Slides) < requested && phrases.ShortDeckNote != nil {
			result.Notes = append(result.Notes, phrases.ShortDeckNote(requested, len(result.Slides), 0))
		}
		if len(result.Slides) > requested {
			// Extra slides are dropped from the end, where a deck's weakest
			// material sits, rather than failing a generation someone waits for.
			result = CompileGenerated(trimSourceSlides(source, requested), request.Presentation, request.Profile, request.Template)
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("kept the first %d slides", requested))
		}
	}
	return result, nil
}

// cleanModelSource strips what a model wraps around an answer even when told not
// to: a fenced code block, a leading label.
func cleanModelSource(raw, language string) string {
	source := strings.TrimSpace(raw)
	if strings.HasPrefix(source, "```") {
		if _, rest, found := strings.Cut(source, "\n"); found {
			source = rest
		}
		if index := strings.LastIndex(source, "```"); index >= 0 {
			source = source[:index]
		}
	}
	// What the model wrote, in the spacing a Korean writer would use.
	return tidyModelKorean(strings.TrimSpace(source), language)
}

// trimSourceSlides keeps the first count slides of a source document.
func trimSourceSlides(source string, count int) string {
	lines := strings.Split(source, "\n")
	slides := 0
	for index, line := range lines {
		// Any leading hash starts a slide, which is how the parser reads it: a
		// stricter rule here would miscount a model that wrote "## Title".
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			slides++
			if slides > count {
				return strings.Join(lines[:index], "\n")
			}
		}
	}
	return source
}

type completionRequest struct {
	Model          string              `json:"model"`
	Messages       []completionMessage `json:"messages"`
	Temperature    float64             `json:"temperature"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	ResponseFormat map[string]string   `json:"response_format,omitempty"`
	// ChatTemplateKwargs is how a self-hosted server is told not to think. A
	// reasoning model spends its whole output budget on reasoning and returns no
	// content at all, so on such a provider Ptium cannot produce a deck without
	// this. It is omitted unless asked for, because a hosted API rejects a body
	// field it does not know.
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

type completionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// completionChoice covers both shapes a chat completion comes back in: content on
// the message, and — on a reasoning model — an empty content with the thinking in
// a field of its own.
type completionChoice struct {
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		// Reasoning is never used as the answer. It is read only to recognise a
		// model that thought instead of answering.
		Reasoning        string `json:"reasoning"`
		ReasoningContent string `json:"reasoning_content"`
	} `json:"message"`
}

func (c completionChoice) reasoned() bool {
	return strings.TrimSpace(c.Message.Content) == "" &&
		(strings.TrimSpace(c.Message.Reasoning) != "" || strings.TrimSpace(c.Message.ReasoningContent) != "")
}

type completionResponse struct {
	Choices []completionChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ProviderCheck is what an operator learns by asking the provider whether it is
// there.
type ProviderCheck struct {
	Provider     string `json:"provider"`
	BaseURL      string `json:"baseUrl,omitempty"`
	Model        string `json:"model,omitempty"`
	Reachable    bool   `json:"reachable"`
	Answered     bool   `json:"answered"`
	Milliseconds int64  `json:"milliseconds"`
	Detail       string `json:"detail,omitempty"`
}

// CheckProvider asks the configured provider for one very short answer.
//
// A deployment of this is a box off the network with a model host somewhere
// beside it, and the only way to learn whether that host answers was to
// generate a deck and see it fail. This asks with the settings as they are
// stored, changes nothing, and says what came back — including how long it
// took, which on a self-hosted model is the number that decides whether a deck
// arrives before the meeting.
func (g *Generator) CheckProvider(ctx context.Context) ProviderCheck {
	check := ProviderCheck{Provider: "fallback", BaseURL: "", Model: ""}
	provider, baseURL, modelName, apiKey := "fallback", "", "", ""
	_ = g.settings.Get(ctx, "ai.provider", &provider)
	_ = g.settings.Get(ctx, "ai.base_url", &baseURL)
	_ = g.settings.Get(ctx, "ai.model", &modelName)
	_ = g.settings.Get(ctx, "ai.api_key", &apiKey)
	check.Provider, check.BaseURL, check.Model = provider, baseURL, modelName
	if strings.EqualFold(provider, "fallback") {
		check.Detail = "이 배포는 오프라인 작성기로 덱을 씁니다. 제공자를 설정하지 않았습니다."
		return check
	}
	endpoint, err := completionsEndpoint(baseURL)
	if err != nil {
		check.Detail = err.Error()
		return check
	}
	// Short enough that a working provider answers in a moment and a wedged one
	// is not waited on: this is a check, not a generation.
	asking, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	started := time.Now()
	answer, err := g.completeWith(asking, endpoint, modelName, apiKey,
		"Answer with the single word ok.", "ping", 0, nil)
	check.Milliseconds = time.Since(started).Milliseconds()
	if err != nil {
		check.Detail = err.Error()
		return check
	}
	check.Reachable = true
	check.Answered = strings.TrimSpace(answer) != ""
	if !check.Answered {
		check.Detail = "제공자가 응답했지만 내용이 비어 있습니다."
		return check
	}
	check.Detail = strings.TrimSpace(answer)
	if len([]rune(check.Detail)) > 120 {
		check.Detail = string([]rune(check.Detail)[:120]) + "…"
	}
	return check
}

func completionsEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/chat/completions")
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("AI base URL is invalid")
	}
	return parsed.String(), nil
}

func (g *Generator) complete(ctx context.Context, endpoint, modelName, apiKey, system, user string, temperature float64) (string, error) {
	return g.completeWith(ctx, endpoint, modelName, apiKey, system, user, temperature,
		map[string]string{"type": "json_object"})
}

// A reasoning model that ignores the switch and thinks through its whole budget
// returns nothing at all. Asking again, plainly, costs one round trip and turns
// a failed generation into a deck. "/no_think" is the switch several open
// weights answer to; to a model that does not know it, it is a stray token.
const (
	plainlyPrompt = "\n\nDo not think before answering. Do not write any analysis, plan or " +
		"explanation. Begin your reply with the first character of the answer itself."
	noThinkPrefix = "/no_think\n"
)

// reasoningMode says whether the provider should be told not to think.
type reasoningMode string

const (
	// reasoningAuto asks the provider not to think, and stops asking if it rejects
	// the request for that reason.
	reasoningAuto reasoningMode = "auto"
	// reasoningOff always asks the provider not to think.
	reasoningOff reasoningMode = "off"
	// reasoningOn never asks, for a provider whose thinking is wanted or whose
	// answer is unaffected by it.
	reasoningOn reasoningMode = "on"
)

// send performs one completion. quiet disables the provider's reasoning.
func (g *Generator) send(ctx context.Context, endpoint, modelName, apiKey, system, user string,
	temperature float64, format map[string]string, quiet bool) (string, error) {
	request := completionRequest{
		Model:          modelName,
		Messages:       []completionMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Temperature:    temperature,
		MaxTokens:      g.maxOutputTokens,
		ResponseFormat: format,
	}
	if quiet {
		request.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	// A self-hosted model on a closed network has no key to send, and an
	// Authorization header carrying "Bearer " and nothing else is a malformed
	// credential rather than an absent one.
	if key := strings.TrimSpace(apiKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return "", err
	}
	var completion completionResponse
	if json.Unmarshal(body, &completion) != nil {
		return "", fmt.Errorf("AI provider returned invalid JSON (status %d)", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := "provider error"
		if completion.Error != nil && completion.Error.Message != "" {
			message = completion.Error.Message
		}
		return "", rejectedRequest{status: response.StatusCode, message: truncate(message, 300)}
	}
	if len(completion.Choices) == 0 {
		return "", errors.New("AI provider returned no choices")
	}
	choice := completion.Choices[0]
	if choice.reasoned() {
		return "", errReasonedWithoutAnswering
	}
	if choice.FinishReason == "length" {
		// The answer stopped because the budget ran out, not because it was
		// finished. Half of one is not a shorter version of it: half a plan is
		// not valid JSON, and half a deck is a deck missing its last slides with
		// nothing said about them. This used to be caught only when the model
		// wrote nothing at all, so an answer cut off after five thousand
		// characters was passed on as if it were whole — the plan pass then
		// failed to read it and was skipped in silence.
		return "", fmt.Errorf("AI provider stopped at the output limit after %d characters; raise ai.max_output_tokens",
			len(strings.TrimSpace(choice.Message.Content)))
	}
	return strings.TrimSpace(choice.Message.Content), nil
}

// errReasonedWithoutAnswering marks a provider that spent its output budget on
// reasoning. It is recoverable: the same request with thinking disabled works.
var errReasonedWithoutAnswering = errors.New("AI provider returned reasoning but no answer")

// rejectedRequest is a provider refusing the request itself, as opposed to
// failing to answer it. A hosted API rejects a body field it does not know, which
// is how Ptium learns not to send the thinking switch again.
type rejectedRequest struct {
	status  int
	message string
}

func (e rejectedRequest) Error() string {
	return fmt.Sprintf("AI provider status %d: %s", e.status, e.message)
}

// withDefaultBrand fills in the deployment's own brand colour for an author who
// has not chosen one, so a house colour reaches the components of every deck.
// The colour this product seeds is left alone: a template's colours are the
// customer's until somebody says otherwise.
func (g *Generator) withDefaultBrand(ctx context.Context, profile model.Profile) model.Profile {
	preferences := map[string]any{}
	_ = json.Unmarshal(profile.Preferences, &preferences)
	for _, key := range []string{"brandColor", "brand_color"} {
		if color, ok := preferences[key].(string); ok && validHexColor(color) {
			return profile
		}
	}
	var color string
	if g.settings == nil || g.settings.Get(ctx, "branding.brand_color", &color) != nil || !validHexColor(color) {
		return profile
	}
	if strings.EqualFold(strings.TrimSpace(color), pptx.SeededAccent) {
		return profile
	}
	preferences["brandColor"] = strings.ToUpper(color)
	profile.Preferences, _ = json.Marshal(preferences)
	return profile
}

// profileAccent is the colour an author chose for the components of their
// decks — and nothing else.
//
// It used to answer with a colour whatever the author had said: the
// deployment's own default when they had set nothing, and a rotating palette
// when there was no deployment default either. That was harmless while nothing
// read the value, and the moment the drawing started reading it, every deck on
// every uploaded template would have had its components repainted in a purple
// nobody picked. A template's colours are the customer's; a colour overrides
// them only when a person actually chose one.
func profileAccent(profile model.Profile, position int) string {
	var preferences map[string]any
	if json.Unmarshal(profile.Preferences, &preferences) == nil {
		for _, key := range []string{"brandColor", "brand_color"} {
			if color, ok := preferences[key].(string); ok && validHexColor(color) {
				return strings.ToUpper(color)
			}
		}
	}
	return ""
}

func validHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

func truncate(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
