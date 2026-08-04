package generation

import (
	"regexp"
	"strings"
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
		// setting, not part of the subject.
		`((임원|경영진|고객|투자자|내부|사내|팀)?\s*(보고|발표|공유|제출|설명)\s*(용|자료)?(으로|로|에)?)|` +
		`(자료(로|를)?\s*(만들|작성|정리))|` +
		`(만들어\s*줘|만들어\s*주세요|만들어라|작성해\s*줘|작성해\s*주세요|정리해\s*줘|정리해\s*주세요|` +
		`요약해\s*줘|요약해\s*주세요|구성해\s*줘|준비해\s*줘|해\s*줘|부탁해|부탁드립니다|` +
		`please|make me|create|generate|write|prepare|summari[sz]e|for the (board|team|exec\w*))`)

// periodPattern finds a stated timeframe, which belongs on the cover.
var periodPattern = regexp.MustCompile(
	`(?i)(20\d{2}\s*년?\s*(상반기|하반기|[1-4]\s*분기|[1-9]|1[0-2])?\s*(월|분기)?)|((FY|fy)\s?20\d{2})|(20\d{2}\s*[-~]\s*20\d{2})`)

// figurePattern finds a number with a unit attached, the kind a prompt supplies
// as a fact: "18%", "42개", "120억", "3배".
// Longer units come first: "12개월" is a duration, not twelve of something.
var figurePattern = regexp.MustCompile(`(\d[\d,.]*)\s*(%|퍼센트|억원|만원|개월|시간|주일|억|만|천|배|개|건|명|일|주|년|원|달러|USD|usd)`)

// topicSplitter breaks a subject into the things it names. Korean conjunctions
// and list punctuation both appear constantly.
var topicSplitter = regexp.MustCompile(`\s*(?:,|·|/|、|;|:|：|및|그리고|와\s|과\s|\band\b|\bplus\b|\+)\s*`)

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
		name := cleanTopic(candidate)
		if name == "" || utf8.RuneCountInString(name) < 2 {
			continue
		}
		outline.Topics = append(outline.Topics, promptTopic{Name: name, Frame: frameFor(name)})
	}
	// A subject that does not split is still one topic.
	if len(outline.Topics) == 0 {
		outline.Topics = []promptTopic{{Name: subject, Frame: frameFor(subject)}}
	}
	return outline
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
	if cut := strings.LastIndexAny(before, ",:·、;：/"); cut >= 0 {
		before = strings.TrimSpace(before[cut+len(string(before[cut])):])
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
	// A cover line runs to about forty characters; beyond that it is a sentence.
	if utf8.RuneCountInString(candidate) > 44 && len(names) > 1 {
		candidate = names[0]
	}
	if utf8.RuneCountInString(candidate) > 60 {
		candidate = string([]rune(candidate)[:60])
	}
	if candidate == "" {
		return given
	}
	return candidate
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
