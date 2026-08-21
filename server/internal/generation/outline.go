package generation

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The deterministic writer reads the prompt rather than decorating it.
//
// A prompt is not a title. "2026년 하반기 클라우드 전환 로드맵과 투자 타당성" names two
// subjects, a period and a kind of argument, and a deck that ignores all of that
// in favour of a fixed outline is the thing people mean when they say the prompt
// was not reflected. So the prompt is read first: what it is about, what it asks
// for, and which figures it already carries.

// promptOutline is what a prompt says about the deck it wants.
type promptOutline struct {
	// Subject is the deck's subject, without the instructions around it.
	Subject string
	// Topics are the things the prompt asks about, in the order it names them.
	Topics []promptTopic
	// Figures are numbers the prompt supplies, which belong on a slide as figures
	// rather than being buried in a sentence.
	Figures []promptFigure
	// Period is a stated timeframe, such as "2026년 하반기".
	Period string
}

// promptTopic is one subject the deck has to cover.
type promptTopic struct {
	Name string
	// Frame is how the topic wants to be argued: what kind of slide it becomes.
	Frame string
}

type promptFigure struct {
	Label string
	Value string
}

// Frames a topic can take. They decide the shape of the slide, not its wording.
const (
	frameSituation = "situation" // where things stand
	frameSequence  = "sequence"  // stages, a roadmap, a plan
	frameCase      = "case"      // a justification: cost, return, feasibility
	frameOptions   = "options"   // a comparison or a decision
	frameRisk      = "risk"      // what could go wrong and what to do
	frameOutcome   = "outcome"   // what changes if this happens
)

// frameMarkers recognises what a topic is asking for. Korean and English share
// the table because a prompt routinely mixes them.
var frameMarkers = []struct {
	frame    string
	patterns []string
}{
	{frameSequence, []string{"로드맵", "일정", "단계", "절차", "계획", "이행", "전환", "마이그레이션", "도입", "추진",
		"roadmap", "timeline", "plan", "phase", "migration", "rollout", "steps"}},
	{frameCase, []string{"타당성", "효과", "roi", "투자", "비용", "예산", "수익", "절감", "근거", "정당성",
		"feasibility", "business case", "cost", "budget", "return", "savings", "justification"}},
	{frameOptions, []string{"비교", "대안", "선택", "방식", "옵션", "후보", "vs", "versus",
		"comparison", "alternative", "option", "trade-off", "tradeoff"}},
	{frameRisk, []string{"리스크", "위험", "우려", "장애", "대응", "완화", "보안", "규제",
		"risk", "mitigation", "concern", "compliance", "security"}},
	{frameOutcome, []string{"기대효과", "성과", "목표", "지표", "kpi", "결과", "개선",
		"outcome", "impact", "result", "target", "metric", "improvement"}},
	{frameSituation, []string{"현황", "배경", "문제", "이슈", "진단", "분석", "실태", "소개",
		"status", "background", "problem", "issue", "analysis", "overview", "introduction"}},
}

// instructionMarkers are the parts of a prompt that address Ptium rather than the
// audience. They are stripped from the subject so a slide title does not read
// "…를 3장으로 만들어줘".
var instructionPattern = regexp.MustCompile(
	`(?i)(\d{1,3}\s*(장|매|쪽|페이지|slides?|pages?)(짜리|정도|이내|분량)?(로|으로|의)?)|` +
		// "임원 보고용으로", "고객 발표용", "for the board": who it is for is a
		// setting, not part of the subject. The address itself goes with it —
		// stripping "에게 보고" out of "임원에게 보고" leaves a stray "임원" that then
		// travels into the deck's title.
		`([가-힣]{2,10}(에게|께|한테)\s*(보고|발표|공유|제출|설명|안내|소개)?(하는|할|해|하기\s*위한|하기\s*위해)?\s*(용|자료)?)|` +
		`((임원|경영진|고객|투자자|내부|사내|팀)?\s*(보고|발표|공유|제출|설명)\s*(용|자료)?(으로|로|에)?)|` +
		`(자료(로|를)?\s*(만들|작성|정리))|` +
		`(만들어\s*줘|만들어\s*주세요|만들어라|작성해\s*줘|작성해\s*주세요|정리해\s*줘|정리해\s*주세요|` +
		`요약해\s*줘|요약해\s*주세요|구성해\s*줘|준비해\s*줘|해\s*줘|부탁해|부탁드립니다|` +
		`please|make me|create|generate|write|prepare|summari[sz]e|` +
		// "for the executive team", "for our board": in English the audience sits
		// at the end of the sentence, and half of it left behind — "…two regions
		// team" — is what a slide title used to read.
		`for\s+(?:the|our|your|an?)?\s*(?:[a-z]+\s+){0,2}` +
		`(?:board|teams?|execs?|executives?|leadership|management|committee|stakeholders?|` +
		`customers?|clients?|investors?|partners?|staff|department|audience))`)

// periodPattern finds a stated timeframe, which belongs on the cover.
var periodPattern = regexp.MustCompile(
	`(?i)(20\d{2}\s*년?\s*(상반기|하반기|[1-4]\s*분기|[1-9]|1[0-2])?\s*(월|분기)?)|((FY|fy)\s?20\d{2})|(20\d{2}\s*[-~]\s*20\d{2})`)

// figurePattern finds a number with a unit attached, the kind a prompt supplies
// as a fact: "18%", "42개", "120억", "3배", "400M KRW", "12 months".
// Longer units come first: "12개월" is a duration, not twelve of something, and
// "400M KRW" is an amount rather than the letter M.
var figurePattern = regexp.MustCompile(`(?i)(\d[\d,.]*)\s*(%|퍼센트|억원|만원|개월|시간|주일|억|만|천|배|개|건|명|일|주|년|원|달러|` +
	`[kmb]n?\s*(?:KRW|USD|EUR|JPY|GBP)|KRW|USD|EUR|JPY|GBP|` +
	// The same units as the Korean ones, in the characters Japanese and Chinese
	// write them with: without these a brief's figures became slide titles.
	`億円|亿元|万円|万元|千円|か月|ヶ月|カ月|个月|小時|小时|時間|パーセント|億|亿|万|千|倍|割|円|元|人|名|件|個|个|日|天|週|周|月|年|` +
	`months?|weeks?|quarters?|years?|days?|hours?|minutes?|` +
	`people|users?|customers?|accounts?|systems?|teams?|sites?|stores?|cases?|` +
	`percentage points?|pp|x)\b?`)

// topicSplitter breaks a subject into the things it names. Korean conjunctions
// and list punctuation both appear constantly.
// A topic never spans a full stop: two sentences are two thoughts, and joining
// them produces a "topic" that reads like a paragraph of the brief.
var topicSplitter = regexp.MustCompile(`\s*(?:[。！？]+|[.!?]+\s|[.!?]+$|,|·|/|、|，|;|；|:|：|및|그리고|와\s|과\s|\band\b|\bplus\b|\+)\s*`)

// outlinePrompt reads a prompt into the structure of a deck.
func outlinePrompt(prompt, title string, phrases languageCopy) promptOutline {
	prompt = strings.TrimSpace(prompt)
	outline := promptOutline{}
	if period := periodPattern.FindString(prompt); strings.TrimSpace(period) != "" {
		outline.Period = strings.Join(strings.Fields(period), " ")
	}
	for _, match := range figurePattern.FindAllStringSubmatch(prompt, 8) {
		value := strings.TrimSpace(match[1] + match[2])
		// A year is a timeframe, not a figure, and the period already carries it.
		if outline.Period != "" && strings.Contains(outline.Period, match[1]) {
			continue
		}
		if match[2] == "년" && len(match[1]) == 4 && strings.HasPrefix(match[1], "20") {
			continue
		}
		label := figureLabel(prompt, match[0])
		outline.Figures = append(outline.Figures, promptFigure{Label: label, Value: value})
	}

	subject := instructionPattern.ReplaceAllString(prompt, " ")
	subject = strings.TrimSpace(strings.Join(strings.Fields(subject), " "))
	subject = cleanTopic(subject)
	if subject == "" {
		subject = strings.TrimSpace(title)
	}
	if subject == "" {
		subject = phrases.DefaultTopic
	}
	outline.Subject = subject

	for _, candidate := range topicSplitter.Split(subject, -1) {
		// "목표 가용성 99.95%" is a figure the deck should show, not a subject it
		// should argue. Treating one as a topic gave it its own slides — and a
		// twelve-slide deck came out with the same step diagram three times.
		if figureClause(candidate, outline.Figures) {
			continue
		}
		name := topicPhrase(cleanTopic(candidate))
		if name == "" || utf8.RuneCountInString(name) < 2 {
			continue
		}
		frame := frameFor(name)
		if frame == frameSituation {
			// Nothing in the words says how to argue this one. Three subjects that
			// all default to the same frame produce three slides with the same lead
			// and the same shape, so each takes a different angle instead.
			frame = defaultFrames[len(outline.Topics)%len(defaultFrames)]
		}
		outline.Topics = append(outline.Topics, promptTopic{Name: name, Frame: frame})
	}
	// A subject that does not split is still one topic.
	if len(outline.Topics) == 0 {
		outline.Topics = []promptTopic{{Name: topicPhrase(subject), Frame: frameFor(subject)}}
	}
	return outline
}

// figureClause reports whether a clause is one of the numbers the prompt gave
// rather than something the deck is about.
//
// A brief lists its figures the same way it lists its subjects — separated by
// commas — so the split cannot tell them apart. What tells them apart is that a
// figure clause is a label and a number and nothing else.
func figureClause(clause string, figures []promptFigure) bool {
	trimmed := strings.TrimSpace(clause)
	if trimmed == "" {
		return false
	}
	for _, figure := range figures {
		value := strings.TrimSpace(figure.Value)
		if value == "" || !strings.Contains(trimmed, value) {
			continue
		}
		// The clause is the figure plus its label, and nothing more: "예산 4억" is
		// the figure, "예산 4억을 어떻게 쓸지" is a subject that mentions it.
		rest := strings.TrimSpace(strings.Replace(trimmed, value, " ", 1))
		label := strings.TrimSpace(figure.Label)
		rest = strings.TrimSpace(strings.TrimPrefix(rest, label))
		if utf8.RuneCountInString(rest) <= 2 {
			return true
		}
	}
	return false
}

// topicPhrase cuts a topic down to something that can sit inside a sentence.
//
// A topic is written into headings and leads — "…을 순서대로 나눠 봅니다" — so a
// topic that is half the brief produces a slide that reads like the brief. Korean
// puts the head noun last, so the tail of a long phrase is the part that names
// the subject: "신규 채널 확장 계획을 임원에게" becomes "확장 계획".
func topicPhrase(name string) string {
	const limit = 20
	if latinPhrase(name) {
		return latinHeadPhrase(name, 48)
	}
	if spacelessScript(name) {
		return cjkTailPhrase(name, limit)
	}
	words := make([]string, 0, 8)
	for _, word := range strings.Fields(strings.TrimSpace(name)) {
		// A conjugated verb is never part of a subject. "보고합니다" is what the
		// author asked for, not what the slide is about.
		if verbLike(word) {
			continue
		}
		words = append(words, word)
	}
	// Everything from the audience onward is the request: "…을 실무진에게 하는
	// 자료" is a subject followed by an instruction, and only the subject belongs
	// in a heading.
	for index, word := range words {
		if index > 0 && addressMarker(word) {
			words = words[:index]
			break
		}
	}
	// And what a stripped verb left behind belongs to nobody.
	for len(words) > 1 && strandedAuxiliary(words[len(words)-1]) {
		words = words[:len(words)-1]
	}
	words = trimStrandedNouns(words)
	// A word that ends in a marker — "임원에게", "현장에서" — names who is being
	// addressed rather than the subject, at either end of what is left.
	for len(words) > 1 && endsWithMarker(words[len(words)-1]) {
		words = words[:len(words)-1]
	}
	for len(words) > 1 && endsWithMarker(words[0]) {
		words = words[1:]
	}
	// Whatever is left, keep its tail: Korean puts the head noun last.
	for start := 0; start < len(words); start++ {
		candidate := strings.Join(words[start:], " ")
		if utf8.RuneCountInString(candidate) <= limit {
			return cleanTopic(candidate)
		}
	}
	if len(words) == 0 {
		return ""
	}
	runes := []rune(words[len(words)-1])
	if len(runes) > limit {
		runes = runes[len(runes)-limit:]
	}
	return string(runes)
}

// verbLike reports whether a word is a conjugated verb rather than a noun.
func verbLike(word string) bool {
	trimmed := strings.Trim(word, " .,·!?")
	for _, ending := range []string{"습니다", "합니다", "됩니다", "입니다", "봅니다", "니다", "한다", "했다", "된다", "됐다",
		// Connective forms carry the sentence on rather than naming a subject.
		"하고", "하며", "되고", "되며", "시키고", "드리고", "이고"} {
		if strings.HasSuffix(trimmed, ending) {
			return true
		}
	}
	return false
}

// endsWithMarker reports whether a word ends in a case marker long enough to be
// unmistakable. One-syllable markers are skipped: 효과, 속도 and 자료 all end in
// a syllable that is also a particle.
func endsWithMarker(word string) bool {
	for _, marker := range []string{"에게", "에서", "에는", "에도", "으로는", "한테", "께서", "부터", "까지"} {
		if strings.HasSuffix(word, marker) && utf8.RuneCountInString(word) > utf8.RuneCountInString(marker) {
			return true
		}
	}
	return false
}

// cleanTopic strips the grammatical tail a Korean phrase carries when it is
// lifted out of a sentence, so "투자 타당성을" becomes "투자 타당성".
func cleanTopic(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " .,·-—()[]")
	// Longest first, so "으로" is removed as a unit rather than leaving a stray "으".
	//
	// Only particles that are rare as a word's own final syllable are stripped.
	// 과, 와, 이, 가, 의, 도, 로 and 만 are all common endings — 효과, 속도, 자료,
	// 평가 — and removing them turns a topic into a fragment. The conjunctions
	// among them are handled by splitting instead.
	for _, suffix := range []string{"에 대해서", "에 대해", "에 관해서", "에 관해", "에 대한", "에 관한",
		"으로", "에서", "에게", "부터", "까지", "이라는", "라는", "에 있어",
		"을", "를", "은", "는"} {
		if !strings.HasSuffix(value, suffix) {
			continue
		}
		stem := strings.TrimSpace(strings.TrimSuffix(value, suffix))
		if utf8.RuneCountInString(stem) >= 2 {
			value = stem
		}
		break
	}
	// A phrase can end up with a dangling vowel jamo after stripping a particle.
	value = strings.TrimSpace(strings.TrimSuffix(value, "으"))
	return strings.Trim(value, " .,·-—")
}

// frameFor decides how a topic wants to be argued.
// defaultFrames are the angles a subject is taken from when its own words do not
// say which one it wants, in the order a report reads best: where it stands,
// what it costs, what could go wrong.
var defaultFrames = []string{frameSituation, frameCase, frameRisk}

func frameFor(topic string) string {
	lowered := strings.ToLower(topic)
	for _, entry := range frameMarkers {
		for _, pattern := range entry.patterns {
			if strings.Contains(lowered, pattern) {
				return entry.frame
			}
		}
	}
	return frameSituation
}

// figureLabel takes the few words before a number as its label, which is where
// prompts put it: "전환 대상 42개 시스템".
func figureLabel(prompt, match string) string {
	index := strings.Index(prompt, match)
	if index <= 0 {
		return ""
	}
	before := strings.TrimSpace(prompt[:index])
	// A label is the words immediately before the number, and it never reaches
	// back past punctuation: "…도입 효과: 개발 속도 32%" labels the 32%, not the
	// sentence it sits in.
	if cut := strings.LastIndexAny(before, ",:·、，;；：/"); cut >= 0 {
		// LastIndexAny reports a byte offset, and "、" is three bytes: stepping
		// over it as though it were one left a stray byte at the front of the
		// label, which is how a figure came out labelled "\ufffd予算".
		_, size := utf8.DecodeRuneInString(before[cut:])
		before = strings.TrimSpace(before[cut+size:])
	}
	if spacelessScript(before) {
		// Nothing to split on, so the label is what sits immediately before the
		// number, back to the particle or punctuation that separates it from the
		// rest of the sentence: "…冗長化する計画。目標可用性99.95%" labels 99.95%
		// with 目標可用性, not with the sentence before it.
		runes := []rune(before)
		const reach = 8
		for index := len(runes) - 1; index >= 0 && len(runes)-index <= reach; index-- {
			if phraseBreaks[runes[index]] {
				return strings.TrimSpace(string(runes[index+1:]))
			}
		}
		if len(runes) > reach {
			runes = runes[len(runes)-reach:]
		}
		return strings.TrimSpace(string(runes))
	}
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return ""
	}
	take := min(len(fields), 2)
	label := strings.Join(fields[len(fields)-take:], " ")
	// Only punctuation is trimmed here. Stripping particles the way a topic does
	// would turn "개발 속도" into "개발 속".
	return strings.Trim(label, " .,·-—:()[]")
}

// headingName is a topic as a slide heading. The timeframe the prompt stated
// belongs on the cover, and repeating it in every heading is what pushed titles
// past their line and left a syllable stranded.
func headingName(name string) string {
	trimmed := strings.TrimSpace(name)
	if prefix := periodPattern.FindString(trimmed); prefix != "" && strings.HasPrefix(trimmed, prefix) {
		if rest := strings.TrimSpace(trimmed[len(prefix):]); utf8.RuneCountInString(rest) >= 4 {
			return rest
		}
	}
	return trimmed
}

// hasFigures reports whether the prompt supplied enough numbers to draw.
func (outline promptOutline) hasFigures() bool { return len(outline.Figures) >= 2 }

// deckTitle is what belongs on the cover.
//
// A client that has no title of its own commonly sends the first line of the
// prompt, instructions and all. A title is the subjects, joined — not the
// request that produced them.
func (outline promptOutline) deckTitle(given, prompt string, joiner string) string {
	given = strings.TrimSpace(given)
	// A title the caller composed by slicing the prompt is not a title.
	sliced := given != "" && strings.HasPrefix(strings.TrimSpace(prompt), given)
	if given != "" && !sliced {
		return given
	}
	// The subject as written comes first: it still has the words that say what
	// the deck is about, which the per-sentence trimming gives up.
	//
	// It has to be the subject whole, though. A brief naming three subjects makes
	// a phrase too long for a cover, and cutting it from the front — where Korean
	// keeps the words that say what this is — leaves "이중화와 재해복구 체계 구축,
	// 그리고 운영 조직 재편" for a deck about payment infrastructure. When the
	// whole thing will not fit, the first subject is the honest title.
	// A title read out of the brief starts the cover, so it is capitalised even
	// though the brief wrote it mid-sentence. A title the author typed is left
	// exactly as they typed it.
	if whole, shortened := titlePhrase(outline.Subject); whole != "" && !shortened {
		return capitalized(whole)
	}
	names := make([]string, 0, len(outline.Topics))
	for _, topic := range outline.Topics {
		if name := strings.TrimSpace(topic.Name); name != "" {
			names = append(names, name)
		}
	}
	candidate := strings.Join(names, joiner)
	if candidate == "" {
		candidate = outline.Subject
	}
	// A cover title is read at a glance. Past about thirty characters it wraps,
	// and a wrapped join of three subjects reliably leaves one syllable stranded
	// on the last line — so the join gives way to the leading subject.
	for len(names) > 1 && utf8.RuneCountInString(candidate) > 30 {
		names = names[:len(names)-1]
		candidate = strings.Join(names, joiner)
	}
	if utf8.RuneCountInString(candidate) > 60 {
		candidate = string([]rune(candidate)[:60])
	}
	if candidate == "" {
		return given
	}
	return capitalized(candidate)
}

// subjectPhrase is what the deck is about, in words a sentence can carry.
//
// The subject as extracted still has the request attached — "…을 실무진에게 하는
// 자료" — and every sentence the plan builds from it inherits that. The title
// already knows how to cut a subject down; the prose uses the same cut.
func (outline promptOutline) subjectPhrase() string {
	// As with the title: a phrase that had to give up its opening words is not
	// what the deck is about, and the first subject is.
	if phrase, shortened := titlePhrase(outline.Subject); phrase != "" && !shortened {
		// A title has a line to itself; a sentence built around the subject does
		// not, so prose takes the shorter form of the same phrase.
		if within := phraseWithin(phrase, 30); within != "" {
			return within
		}
		return phrase
	}
	if len(outline.Topics) > 0 && strings.TrimSpace(outline.Topics[0].Name) != "" {
		return outline.Topics[0].Name
	}
	return outline.Subject
}

// titlePhrase is the prompt's own subject, kept whole, for the cover.
//
// A topic written into a sentence is cut down to its head noun — "결제 시스템
// 이중화 계획" becomes "이중화 계획", which reads well mid-sentence. On the cover
// that is the wrong trade: the words dropped from the front are the ones that
// say what the deck is about. So a title keeps the leading noun phrase and cuts
// at the audience instead ("…을 실무진에게 …" → "결제 시스템 이중화 계획").
func titlePhrase(subject string) (string, bool) {
	const limit = 30
	if latinPhrase(subject) {
		// Latin script puts the head noun first, so an over-long phrase gives way
		// at the end rather than at the front.
		// The head is what the deck is about, so a phrase that gave up its tail
		// has not given up its subject: nothing to report.
		return latinHeadPhrase(subject, 48), false
	}
	if spacelessScript(subject) {
		// No spaces to trim words at, so the cut is made where a clause ends —
		// and, as with Latin, the opening is kept, so nothing is reported.
		return cjkHeadPhrase(subject, limit), false
	}
	words := strings.Fields(strings.TrimSpace(subject))
	// Everything from the audience onward is the request, not the subject.
	for index, word := range words {
		if index > 0 && addressMarker(word) {
			words = words[:index]
			break
		}
	}
	// Stripping the instruction can leave the auxiliary behind: "설명하는" with
	// 설명 removed is a bare "하는", which belongs to nobody.
	for len(words) > 0 && strandedAuxiliary(words[len(words)-1]) {
		words = words[:len(words)-1]
	}
	words = trimStrandedNouns(words)
	if len(words) == 0 {
		return "", false
	}
	// Korean puts the head noun last, so an over-long phrase gives way at the
	// front — but only as far as it must, and giving way is worth reporting: a
	// title that had to drop its opening words is not what the deck is about.
	for start := 0; start < len(words); start++ {
		candidate := cleanTopic(strings.Join(words[start:], " "))
		if candidate != "" && utf8.RuneCountInString(candidate) <= limit {
			return candidate, start > 0
		}
	}
	return "", true
}

// trimStrandedNouns drops what is left dangling when the words between two
// nouns are removed.
//
// "…계획을 실무진에게 설명하는 자료" loses its middle to the instruction pattern
// and leaves "계획을 자료" — an object particle with no verb and a document noun
// with nothing to attach to. Both ends are cleaned: a trailing document noun that
// follows an object, and a leading verb form whose subject went with the address.
func trimStrandedNouns(words []string) []string {
	documents := map[string]bool{"자료": true, "덱": true, "문서": true, "보고서": true, "발표자료": true, "장표": true}
	for len(words) > 1 && documents[strings.Trim(words[len(words)-1], " .,·")] {
		previous := words[len(words)-2]
		if !strings.HasSuffix(previous, "을") && !strings.HasSuffix(previous, "를") {
			break
		}
		words = words[:len(words)-1]
	}
	for len(words) > 1 && modifierForm(words[0]) {
		words = words[1:]
	}
	return words
}

// modifierForm reports whether a word is a verb modifying what follows — the
// half of "고객에게 전달할" that stays when the address is taken away.
func modifierForm(word string) bool {
	trimmed := strings.Trim(word, " .,·")
	if utf8.RuneCountInString(trimmed) < 2 {
		return false
	}
	for _, ending := range []string{"할", "한", "하는", "될", "된", "되는", "드릴", "드리는"} {
		if strings.HasSuffix(trimmed, ending) {
			return true
		}
	}
	return false
}

// addressMarker reports whether a word names who the deck is for.
func addressMarker(word string) bool {
	trimmed := strings.Trim(word, " .,·!?")
	for _, ending := range []string{"에게", "께", "한테", "대상으로", "용으로", "용", "위해", "위한"} {
		if strings.HasSuffix(trimmed, ending) && utf8.RuneCountInString(trimmed) > utf8.RuneCountInString(ending) {
			return true
		}
	}
	return false
}

// strandedAuxiliary reports whether a word is what a removed verb left behind.
func strandedAuxiliary(word string) bool {
	switch strings.Trim(word, " .,·!?") {
	case "하는", "한", "할", "하기", "하기로", "된", "되는", "드리는", "주는", "위한", "위해":
		return true
	}
	return false
}

// TitleFor is the deck title a prompt implies. It is exported so a deck is named
// when it is created rather than only when it is written, which is what the
// workspace lists and what the cover shows.
//
// A caller's own title wins. A placeholder — an empty string, or the default a
// client sends when the person typed nothing — does not.
func TitleFor(prompt, given, language string) string {
	if isAuthoredTitle(given, prompt) {
		return strings.TrimSpace(given)
	}
	joiner := ", "
	if strings.HasPrefix(strings.ToLower(language), "ko") || strings.TrimSpace(language) == "" {
		joiner = " · "
	}
	outline := outlinePrompt(prompt, given, localizedCopy(language))
	if title := outline.deckTitle("", prompt, joiner); strings.TrimSpace(title) != "" {
		return title
	}
	return strings.TrimSpace(given)
}

// placeholderTitles are the names a client sends when the author typed none.
var placeholderTitles = []string{"untitled presentation", "untitled", "새 프레젠테이션", "제목 없음", "무제"}

func isAuthoredTitle(given, prompt string) bool {
	trimmed := strings.TrimSpace(given)
	if trimmed == "" {
		return false
	}
	for _, placeholder := range placeholderTitles {
		if strings.EqualFold(trimmed, placeholder) {
			return false
		}
	}
	// A title that is the opening of the prompt was sliced from it, not written.
	return !strings.HasPrefix(strings.TrimSpace(prompt), trimmed)
}

// --- phrases in a script that reads the other way ----------------------------

// latinPhrase reports whether a phrase is written in Latin script.
//
// Everything above cuts a phrase down by keeping its tail, because Korean —
// like Japanese and Chinese — puts the head noun last. English does the
// opposite: "a plan to make the payment platform redundant across two regions"
// says what it is in its first words, and keeping the tail produced titles that
// read "two regions team". So a Latin phrase is cut from the other end.
func latinPhrase(value string) bool {
	letters, latin := 0, 0
	for _, symbol := range value {
		if !unicode.IsLetter(symbol) {
			continue
		}
		letters++
		if symbol < unicode.MaxASCII || unicode.Is(unicode.Latin, symbol) {
			latin++
		}
	}
	return letters > 0 && latin*4 >= letters*3
}

// latinPrepositions begin the modifier at the end of a noun phrase. When a
// phrase has to give way, it gives way at one of these rather than mid-thought.
var latinPrepositions = map[string]bool{
	"across": true, "for": true, "with": true, "within": true, "under": true, "over": true,
	"through": true, "throughout": true, "during": true, "among": true, "between": true,
	"into": true, "onto": true, "toward": true, "towards": true, "against": true, "about": true,
	"by": true, "from": true, "in": true, "on": true, "at": true, "to": true,
}

// latinArticles open a phrase without naming anything, and a title never starts
// or ends with one.
var latinArticles = map[string]bool{"a": true, "an": true, "the": true, "and": true, "or": true, "of": true}

// latinHeadPhrase keeps the head of a Latin phrase, within a limit. It gives
// way one word at a time from the end, because the words at the front are the
// ones that say what the phrase is about, and it never ends on a word that
// only introduces what was cut.
func latinHeadPhrase(name string, limit int) string {
	words := strings.Fields(strings.TrimSpace(name))
	for len(words) > 1 && latinArticles[strings.ToLower(strings.Trim(words[0], " .,"))] {
		words = words[1:]
	}
	gaveWay := false
	for len(words) > 0 {
		for len(words) > 1 {
			last := strings.ToLower(strings.Trim(words[len(words)-1], " .,"))
			// A phrase never ends on a word that only introduces what was cut, and
			// after it gives way, not on the participle that opened the clause it
			// lost: "…for new engineers joining the" is half a thought.
			if latinPrepositions[last] || latinArticles[last] ||
				(gaveWay && len(words) > 2 && strings.HasSuffix(last, "ing")) {
				words = words[:len(words)-1]
				continue
			}
			break
		}
		candidate := cleanTopic(strings.Join(words, " "))
		if utf8.RuneCountInString(candidate) <= limit {
			return candidate
		}
		words = words[:len(words)-1]
		gaveWay = true
	}
	return ""
}

// phraseWithin cuts a phrase to a limit from whichever end its script keeps its
// head noun on.
func phraseWithin(name string, limit int) string {
	if latinPhrase(name) {
		return latinHeadPhrase(name, limit)
	}
	if spacelessScript(name) {
		return cjkTailPhrase(name, limit)
	}
	words := strings.Fields(strings.TrimSpace(name))
	for start := 0; start < len(words); start++ {
		candidate := cleanTopic(strings.Join(words[start:], " "))
		if candidate != "" && utf8.RuneCountInString(candidate) <= limit {
			return candidate
		}
	}
	return ""
}

// capitalized starts a Latin phrase the way a title does. A heading lifted out
// of a brief arrives in the middle of a sentence — "migration in three phases"
// — and a slide that opens in lower case reads as an unfinished note.
func capitalized(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !latinPhrase(trimmed) {
		return trimmed
	}
	runes := []rune(trimmed)
	if unicode.IsUpper(runes[0]) {
		return trimmed
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// --- phrases in a script that has no spaces ----------------------------------

// spacelessScript reports whether a phrase is written without spaces between
// its words, as Japanese and Chinese are.
//
// Every rule above works on words, and strings.Fields on a Japanese sentence
// returns the sentence. So a phrase in one of those scripts was never trimmed
// at all: "予算4億" arrived as a slide title instead of as a figure, and a cut
// to length landed in the middle of a word — "二つの" became "つの".
func spacelessScript(value string) bool {
	letters, spaceless := 0, 0
	for _, symbol := range value {
		if !unicode.IsLetter(symbol) {
			continue
		}
		letters++
		// Korean is written with spaces, so Hangul is not counted here.
		if unicode.Is(unicode.Han, symbol) || unicode.Is(unicode.Hiragana, symbol) ||
			unicode.Is(unicode.Katakana, symbol) {
			spaceless++
		}
	}
	return letters > 0 && spaceless*2 >= letters
}

// phraseBreaks are where a Japanese or Chinese phrase can be cut without
// landing inside a word: the particles that join clauses, and punctuation.
var phraseBreaks = map[rune]bool{
	// Japanese case particles.
	'を': true, 'は': true, 'が': true, 'に': true, 'で': true, 'へ': true, 'と': true,
	'も': true, 'や': true, 'ら': true, 'り': true,
	// Chinese function words that end a modifier.
	'的': true, '和': true, '与': true, '及': true, '在': true, '对': true, '为': true,
	'把': true, '从': true, '到': true, '让': true, '向': true,
	// Punctuation, in both. Only the full-width forms: a full stop between two
	// digits is a decimal point, and cutting a phrase at it splits a number.
	'、': true, '。': true, '，': true, '；': true, '：': true, '・': true, '「': true, '」': true,
	'（': true, '）': true, '！': true, '？': true,
}

// cjkTailPhrase keeps the tail of a space-less phrase — where these scripts,
// like Korean, put the head noun — cutting where a clause ends rather than
// inside a word.
func cjkTailPhrase(name string, limit int) string {
	runes := []rune(strings.Trim(strings.TrimSpace(name), " 、。，・"))
	if len(runes) <= limit {
		return string(runes)
	}
	for start := len(runes) - limit; start < len(runes); start++ {
		if start == 0 || phraseBreaks[runes[start-1]] {
			return strings.Trim(string(runes[start:]), " 、。，・")
		}
	}
	return strings.Trim(string(runes[len(runes)-limit:]), " 、。，・")
}

// cjkHeadPhrase keeps the opening of a space-less phrase, for a cover title:
// the words a brief drops last are the ones that say what it is about.
func cjkHeadPhrase(name string, limit int) string {
	runes := []rune(strings.Trim(strings.TrimSpace(name), " 、。，・"))
	if len(runes) <= limit {
		return string(runes)
	}
	// A brief's first sentence is its subject, so the cut is made there when one
	// ends within reach. Otherwise it is made where a clause ends — a comma
	// before the sentence's own end would keep half a thought.
	for _, breaks := range []map[rune]bool{sentenceEnds, phraseBreaks} {
		for end := limit; end > 0; end-- {
			if breaks[runes[end]] || breaks[runes[end-1]] {
				return strings.Trim(string(runes[:end]), " 、。，・")
			}
		}
	}
	return strings.Trim(string(runes[:limit]), " 、。，・")
}

// sentenceEnds are where one thought finishes and the next begins.
var sentenceEnds = map[rune]bool{'。': true, '！': true, '？': true}
