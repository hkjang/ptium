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
	// Asked says the brief named this section outright — "리스크 장을 넣어주세요"
	// — rather than the deck being about it. It gets a section like any other
	// topic and stays out of the deck's title.
	Asked bool
	// Chosen says the frame came from the topic's own words rather than from the
	// rotation that keeps three wordless topics from looking alike. A frame the
	// words chose is not swapped for variety: a section the brief called "채용
	// 일정" was coming back with the body of a metrics slide because an earlier
	// slide had used the sequence frame first, so the deck told its author to
	// write targets under a heading about a schedule.
	Chosen bool
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
// askedNicely is how a Korean request ends. It is optional everywhere it
// appears, and it appears after every verb that can be the last word of a
// brief, because taking the verb and leaving the auxiliary is what put the
// imperative on the cover: "…타당성 검토 결과를 줘" was the title of a deck,
// and the same three syllables headed every slide in it.
const askedNicely = `\s*(줘|주세요|주시고|주시죠|주라|줄래|주실래요|주십시오|다오)?`

var instructionPattern = regexp.MustCompile(
	// A count can be written in words as easily as in digits — "세 장으로" is how
	// anyone actually asks — and an instruction left in the subject travels into
	// a slide title: "재발 방지책을 세 장".
	`(?i)((\d{1,3}|한|두|세|네|다섯|여섯|일곱|여덟|아홉|열|몇)\s*(장|매|쪽|페이지|ページ|枚|页|張|张|slides?|pages?)(짜리|정도|이내|분량|くらい|ほど|程度|以内|前後)?(로|으로|의|で|の|的)?)|` +
		// "임원 보고용으로", "고객 발표용", "for the board": who it is for is a
		// setting, not part of the subject. The address itself goes with it —
		// stripping "에게 보고" out of "임원에게 보고" leaves a stray "임원" that then
		// travels into the deck's title.
		// "이사회에 보고" addresses the room with 에 rather than 에게, and leaving
		// it in ended a slide title with "…이사회에" and then wrote "이사회에에".
		`([가-힣]{2,10}(에게|께|한테)\s*(보고|발표|공유|제출|설명|안내|소개)?(하는|할|해|하기\s*위한|하기\s*위해)?` +
		askedNicely + `\s*(용|자료)?(으로|로|를|을)?)|` +
		`([가-힣]{1,10}에\s*(보고|발표|공유|제출|설명|안내|소개)(하는|할|해|한|하여|하고|하기\s*위한|하기\s*위해)?` +
		askedNicely + `\s*(용|자료)?(으로|로|를|을)?)|` +
		// "경영진 대상으로", "고객 대상": the room can be named without 에게, and
		// the half left behind became the end of the title — "…3개년 로드맵을
		// 경영진 대상". Only after a word that names a room, because 대상 on its
		// own is an ordinary noun: "분석 대상" is part of a subject.
		`((임원|경영진|고객|투자자|내부|사내|팀|팀장|부서|이사회|실무자|개발팀|영업팀)\s*대상(으로|의|,)?)|` +
		// "보고서", "제안서", "기획서": a document whose name begins with one of
		// these verbs is one word. It is matched here so that the verb alternative
		// below cannot take half of it — a deck came out titled "AI 챗봇 도입 검토
		// 서" — and then handed back whole, because what the document is called is
		// part of what the deck is about.
		`(보고서|제안서|기획서|계획서|검토서|설명서|요약서)|` +
		// The audience carries the space in front of the verb with it. Written as
		// "(임원)?\s*보고", the match starts at the space even when no audience is
		// there — one character to the left of "보고서", so the document noun
		// above never got its turn.
		// "보고해줘" is one word to the person writing it. Taking "보고해" and
		// leaving "줘" put the imperative on the cover and on every heading:
		// "사내 PostgreSQL 전환 타당성 검토 결과를 줘". The auxiliary that finishes
		// the request belongs to the request.
		// "보고 체계 개선 방안" is about a reporting process, and with every part
		// after 보고 optional the rule took the word out of it: the deck came
		// back called "체계 개선 방안". An instruction says who it is for, or how
		// it is being done, or that it is 용 — a bare noun says none of those.
		`((임원|경영진|고객|투자자|내부|사내|팀)\s*(보고|발표|공유|제출|설명)(할|하는|한|해|하여|하고|합니다|해서)?` +
		askedNicely + `\s*(용|자료)?(으로|로|를|을|에)?)|` +
		`((보고|발표|공유|제출|설명)((할|하는|한|해|하여|하고|합니다|해서)` + askedNicely +
		`\s*(용|자료)?(으로|로|를|을|에)?|용(으로|로)?|\s*자료(로|를)?))|` +
		`(자료(로|를)?\s*(만들|작성|정리|준비|구성)(어|아|여)?\s*(줘|주세요|주시고|주라|줄래|주실래요)?)|` +
		// The verbs a request for a section is written with. What was a request
		// has already been read by then; what is left is an instruction with no
		// section attached to it, and "…계획을 넣어줘" is not a subject.
		`(넣어\s*줘|넣어\s*주세요|넣어\s*주시고|포함해\s*줘|포함해\s*주세요|추가해\s*줘|추가해\s*주세요|` +
		// "…를 담아 주세요" asks for a section as plainly as "…를 넣어 주세요", and
		// without it the request stayed glued to the last section the brief
		// listed: a deck came back with a slide headed "예산을 담아 주세요".
		`담아\s*줘|담아\s*주세요|담아\s*주시고|담아주|담아서\s*주세요|` +
		`만들어\s*줘|만들어\s*주세요|만들어라|작성해\s*줘|작성해\s*주세요|정리해\s*줘|정리해\s*주세요|` +
		// "써 주세요" asks for a deck exactly as "작성해 주세요" does, and it was
		// the only one of the two the subject kept: "신규 채용 계획을 써 주세요"
		// was the title of the deck it produced.
		`써\s*줘|써\s*주세요|써\s*주시고|써\s*주십시오|써\s*주라|써\s*줄래|` +
		`요약해\s*줘|요약해\s*주세요|구성해\s*줘|준비해\s*줘|해\s*줘|부탁해|부탁드립니다|` +
		// The whole of an English request, taken in one piece so that no part of
		// it is left to become the subject. Stripping the verb alone left the
		// rest standing: "Write me a 8 slide deck about reducing cloud spend"
		// produced a deck titled "Me a deck about reducing cloud spend", and
		// "Make a short deck about hiring plans" kept its own verb. It is
		// anchored so it can only take the opening of a brief, and it must come
		// before the bare verbs below, which would otherwise match first and
		// take only their own word.
		// What follows the verb is not optional. With every part of it allowed
		// to be empty the rule matched any sentence that opened with one of
		// these words, and "Make or buy decision for our data platform" became
		// a deck about "or buy decision". A request says who it is for ("me")
		// or what it wants ("a 10 slide deck about"); a subject does neither.
		`^\s*(?:please\s+)?(?:write|make|create|generate|prepare|draft|build|put\s+together|give|send)\s+` +
		`(?:` +
		`me\s+(?:an?|the)?\s*(?:short|quick|brief|detailed|simple|rough)?\s*` +
		`(?:\d{1,3}[-\s]?(?:slide|page)s?\s+)?(?:deck|presentation|slides?|report|summary|briefing|overview)?\s*` +
		`(?:about|on|covering|regarding|explaining)?` +
		`|(?:an?|the)\s+(?:short|quick|brief|detailed|simple|rough)?\s*` +
		`(?:\d{1,3}[-\s]?(?:slide|page)s?\s+)?(?:deck|presentation|slides?|report|summary|briefing|overview)` +
		`\s*(?:about|on|covering|regarding|explaining)?` +
		`|(?:\d{1,3}[-\s]?(?:slide|page)s?\s+)(?:deck|presentation|slides?)\s*(?:about|on|covering)?` +
		`)\s*|` +
		// Japanese and Chinese are offered on the same screen as Korean and
		// English, and neither had any instruction reading at all: "新しい採用計画
		// を8枚でまとめてください" became, in full, the title of the deck it asked
		// for. Only the asking is stripped — the polite ending is required,
		// because 作成 and 整理 are ordinary words inside a subject.
		`((まとめ|作成し|作っ|書い|用意し|整理し|準備し|説明し)(て|で)\s*(ください|下さい|くれ|ほしい|欲しい|もらえますか|いただけますか)?)|` +
		`(お願いします|お願いいたします|お願い致します|してください|して下さい)|` +
		// 向け is "for": the room, as Japanese names it. Only a room that is
		// actually named, because "any word before 向け" reaches backwards
		// through the subject — it turned "データ基盤移行のリスクを経営層向けに…"
		// into a deck about "データ".
		`((経営層|役員|取締役|上司|お客様|顧客|社内|チーム|部門|部署|投資家|一般|新入社員|開発チーム|営業)` +
		`\s*(向けに|向けて|向け|宛て))|` +
		`(について|に関して|に関する)|` +
		// 请 opens a request only when a request verb follows it: "请示流程" is a
		// subject that begins with the same character, and stripping it left
		// "示流程". The same for 整理 and 准备, ordinary verbs in a subject unless
		// the sentence is asking for something — and Japanese words too, so a
		// loose rule here reached into Japanese briefs and made 整理整頓 into 整頓.
		`(^\s*(请|麻烦|劳烦)\s*(用|帮我|帮忙)?\s*(\d{1,3}\s*(页|张|頁)\s*)?` +
		`(做成|做一个|做一份|写一份|制作|整理|准备|总结|归纳)(一下)?)|` +
		`(^\s*帮我\s*(做成|做一个|做一份|写一份|制作|整理|准备|总结|归纳)(一下)?)|` +
		`(^\s*把)|` +
		`((做成|做一个|做一份|写一份)|(整理|准备|制作|总结|归纳)\s*一下)|` +
		`((的)?\s*(汇报|报告|報告|演示|PPT|ppt)\s*$)|` +
		// The bare verbs that remain are the ones that cannot be anything but a
		// request. "write", "create", "generate" and "prepare" open ordinary
		// sentences — "Generate reports faster with the new pipeline" is a
		// subject, and taking the first word left "s faster with the new
		// pipeline".
		`please\s+|make me\s+|summari[sz]e\s+|` +
		// "A board update on …", "An update for the team on …": in English the
		// purpose comes first and the subject follows it. Left in, the purpose
		// became the subject, and every slide in the deck was titled with the
		// whole brief.
		`^\s*(?:an?|the)?\s*(?:board|executive|management|quarterly|monthly|weekly|internal|short|brief)?\s*` +
		`(?:update|report|deck|presentation|briefing|overview|summary|slides?)\s+(?:on|about|for|covering)\s+|` +
		// "for the executive team", "for our board": in English the audience sits
		// at the end of the sentence, and half of it left behind — "…two regions
		// team" — is what a slide title used to read.
		`for\s+(?:the|our|your|an?)?\s*(?:[a-z]+\s+){0,2}` +
		`(?:board|teams?|execs?|executives?|leadership|management|committee|stakeholders?|` +
		`customers?|clients?|investors?|partners?|staff|department|audience))`)

// askedSection finds a slide the brief asks for by name.
//
// "마지막에 리스크 장을 꼭 넣어주세요" is somebody naming a section, and the deck
// came out with a slide actually titled "리스크 장을 꼭 넣어주세요" — twice, plus a
// "— 현황" of it. The English side was the same: "Include a slide on rollback"
// became a heading three times over. What the sentence names is the section;
// the rest of it is the asking.
//
// The asking has to be there. A slide word by itself says nothing: "랜딩 페이지
// 개편" and "보안 섹션 강화 방안" are subjects that happen to contain one, and
// reading those as requests took the subject out of the deck and left a section
// called "랜딩".
var askedSection = regexp.MustCompile(
	`([^\s,.。\n][^,.。\n]{0,18}?)\s*(?:슬라이드|섹션|페이지|장)\s*(?:도|은|는|을|를)?\s*` +
		`(?:꼭\s*|반드시\s*|하나\s*)?(?:넣어\s*주세요|넣어\s*주시고|넣어\s*줘|넣어라|넣어|넣기|넣고|` +
		`포함해\s*주세요|포함해\s*줘|포함시켜|포함하고|포함해|포함|` +
		`추가해\s*주세요|추가해\s*줘|추가하고|추가해|추가|` +
		`만들어\s*주세요|만들어\s*줘|만들어|구성해\s*주세요|부탁해|부탁드립니다|부탁)`)

// namedBeforeSlideWord pulls the names out of a request once the request has
// been found. Two sections asked for in one breath — "일정 슬라이드와 예산
// 슬라이드를 포함해 주세요" — are one sentence with one verb, so the whole list is
// one request and each name sits in front of a slide word inside it.
var namedBeforeSlideWord = regexp.MustCompile(`([^\s,.。\n][^,.。\n]{0,18}?)\s*(?:슬라이드|섹션|페이지|장)`)

// joinedOn is the conjunction the second name in such a list begins with.
var joinedOn = regexp.MustCompile(`^\s*(?:와|과|,|랑|하고|및|그리고)\s*`)

// askingLatin says the sentence is a request at all: English puts the asking
// first, and without it "the landing page redesign" is a subject.
var askingLatin = regexp.MustCompile(`(?i)\b(?:include|add)\b`)

// askedSectionLatin is the request itself, in either order it gets written.
var askedSectionLatin = regexp.MustCompile(
	`(?i)\b(?:include|add)\s+(?:an?|the)?\s*(?:slides?|sections?|pages?)\s+(?:on|about|for|covering)\s+` +
		`([a-z][a-z0-9'\- ]{1,28}?)(?:\s+(?:and|plus)\b|[,.]|$)|` +
		`\b(?:an?|the)\s+([a-z][a-z0-9'\-]{1,20})\s+(?:slides?|sections?|pages?)\b`)

// wherePlaced is the "put it at the end" half of such a request: an instruction
// about order, not a thing to have a slide about.
var wherePlaced = regexp.MustCompile(`(?:맨\s*)?(?:마지막|처음|끝|앞|뒤|중간)\s*(?:에|에다|으로|로)?\s*`)

// strandedVerb is the verb a sentence ends on once what it was doing has been
// taken out of it.
//
// "물류센터 자동화 도입을 임원에게 보고합니다" loses "임원에게 보고" — that is the
// audience, not the subject — and what is left ends "…도입을 합니다", which then
// became the deck's title. The subject is the noun in front of the verb.
var strandedVerb = regexp.MustCompile(
	`\s*(?:을|를|이|가)?\s*(?:하고자\s*)?(?:합니다|입니다|했습니다|하였습니다|하겠습니다|됩니다|드립니다|한다|이다)\s*([.。,]|$)`)

// intending is somebody saying what they are about to do — "개선하려고 합니다",
// "새로 만들려고 하는데", "공유하려고" — which is the asking rather than the
// subject. Left in, a deck came out titled "협력사 정산 프로세스를 개선하려고".
var intending = regexp.MustCompile(
	`\s*(?:하|되)?(?:려고|으려고|고자)\s*(?:합니다|해요|하는데|한다|하며|하고|해서|하면서)?`)

// leftMidVerb is the stem an intention leaves standing when the verb was not
// written with 하 — "새로 만들려고 하는데" comes back as "새로 만들". The adverb in
// front of it belongs to the verb rather than to the subject.
var leftMidVerb = regexp.MustCompile(
	`\s*(?:을|를)?\s*(?:새로|다시|새롭게|추가로|따로)?\s*(?:만들|만드|시키)(\s|$)|` +
		`\s*(?:을|를)?\s*(?:하|되)\s*$`)

// pastTense is a verb that has already happened — "나빠졌습니다", "늘었습니다".
// Unlike 합니다 and 입니다, which attach to a noun the subject needs, these
// endings only ever follow a verb stem, so the whole word goes.
var pastTense = regexp.MustCompile(`\s*[가-힣]{0,5}(?:었|았|였|졌)습니다\s*([.。,]|$)`)

// comparedWith is the half of a sentence that says "than what". "재고 회전율이
// 작년보다" is not a subject; 재고 회전율 is.
var comparedWith = regexp.MustCompile(`\s*[가-힣]{1,8}보다\s*([.。,]|$)`)

// stillNeeded is what "8장 정도 필요합니다" leaves once the count is out of it.
var stillNeeded = regexp.MustCompile(`\s*(?:정도\s*)?필요(?:합니다|해요|하다|한)?\s*([.。,]|$)`)

// strayComma is the punctuation an instruction leaves behind when it is taken
// out from between two halves of a sentence.
var strayComma = regexp.MustCompile(`\s+([,;:])`)

// emptyClause is a separator with nothing between it and the next one.
var emptyClause = regexp.MustCompile(`([,;:])\s*[,;:]`)

// tidySeparators closes the gap an instruction left. Taking the audience out of
// "Budget request for the design team, 250M KRW" left "Budget request , 250M
// KRW", and that space before the comma was on the cover.
func tidySeparators(subject string) string {
	subject = strayComma.ReplaceAllString(subject, "${1}")
	for range 3 {
		tidied := emptyClause.ReplaceAllString(subject, "${1}")
		if tidied == subject {
			break
		}
		subject = tidied
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(subject), ",;:"))
}

// justAPeriod says the clause names a time and nothing else. A brief that ends
// "…, 2026" gave the deck a section titled "2026".
func justAPeriod(clause string) bool {
	trimmed := strings.TrimSpace(clause)
	if trimmed == "" {
		return false
	}
	if place := periodPattern.FindStringIndex(trimmed); place != nil {
		return place[0] == 0 && place[1] == len(trimmed)
	}
	return false
}

// slideCount is how long the deck should be, written the way anyone asks for
// it — in digits or in words.
var slideCount = regexp.MustCompile(
	`(?i)(\d{1,3}|한|두|세|네|다섯|여섯|일곱|여덟|아홉|열|몇)\s*(장|매|쪽|페이지|ページ|枚|页|張|张|slides?|pages?)` +
		`(\s*(짜리|정도|안팎|이내|이상|분량|내외|くらい|ほど|程度|以内|前後))?(\s*(로|으로|의|을|를|で|の|的))?`)

// justACount says the name is how many slides rather than what one is about.
// "세 장을 넣어줘" asks for three slides, not for a section called 세.
var countWord = regexp.MustCompile(`^(?:\d{1,3}|한|두|세|네|다섯|여섯|일곱|여덟|아홉|열|몇|여러)$`)

func justACount(name string) bool { return countWord.MatchString(strings.TrimSpace(name)) }

// leftoverAsking is the "include" an English request leaves behind once the
// slide it asked for has been taken out of the sentence.
var leftoverAsking = regexp.MustCompile(`(?i)\b(?:please\s+)?(?:include|add)\s*,`)

// askedFor separates a brief into what it is about and the sections it asks
// for by name. The request goes out of the subject entirely: leaving the name
// in made it part of the deck's title, and a report on warehouse automation
// came out called "물류센터 자동화 도입을 합니다. 리스크".
func askedFor(subject string) (string, []string) {
	var names []string
	take := func(matches [][]string) {
		for _, match := range matches {
			for _, group := range match[1:] {
				// "마지막에 리스크 장" says where to put it as well as what it is
				// about, and a slide titled "마지막에 리스크" is neither.
				group = wherePlaced.ReplaceAllString(group, " ")
				group = joinedOn.ReplaceAllString(group, "")
				name := cleanTopic(strings.TrimSpace(group))
				if utf8.RuneCountInString(name) >= 2 && !justACount(name) {
					names = append(names, name)
					break
				}
			}
		}
	}
	for _, span := range askedSection.FindAllString(subject, 4) {
		take(namedBeforeSlideWord.FindAllStringSubmatch(span, 4))
	}
	subject = askedSection.ReplaceAllString(subject, ", ")
	if askingLatin.MatchString(subject) {
		take(askedSectionLatin.FindAllStringSubmatch(subject, 4))
		subject = askedSectionLatin.ReplaceAllString(subject, ", ")
		subject = leftoverAsking.ReplaceAllString(subject, ", ")
	}
	subject = wherePlaced.ReplaceAllString(subject, " ")
	subject = strandedVerb.ReplaceAllString(subject, "${1}")
	return strings.TrimSpace(strings.Join(strings.Fields(subject), " ")), names
}

// documentNouns are the words for the thing being written. They are matched by
// the instruction pattern only so that nothing else takes part of them.
var documentNouns = map[string]bool{
	"보고서": true, "제안서": true, "기획서": true, "계획서": true,
	"검토서": true, "설명서": true, "요약서": true,
}

func keepDocumentNouns(match string) string {
	if documentNouns[strings.TrimSpace(match)] {
		return match
	}
	return " "
}

// audienceAddress is who a sentence is addressed to, at the front of it.
var audienceAddress = regexp.MustCompile(
	`(?:임원|경영진|이사회|고객|클라이언트|투자자|개발팀|엔지니어링팀|기획팀|영업팀|운영팀|` +
		`전사|사내|본부|직원|신입|팀원|팀)\s*(?:에게|께|한테|에)\s`)

// tellingSomebody says the sentence is addressed to a room at all, whatever
// distance its verb sits at from the address.
var tellingSomebody = regexp.MustCompile(`(?:보고|발표|공유|설명|제출|안내|소개|배포)(?:할|하는|한|해|하여|하고|합니다|해서|용|서)`)

// periodPattern finds a stated timeframe, which belongs on the cover.
var periodPattern = regexp.MustCompile(
	`(?i)(20\d{2}\s*[년年]?\s*(상반기|하반기|上半期|下半期|[1-4]\s*분기|[1-9]|1[0-2])?\s*(월|月|분기|四半期)?)|((FY|fy)\s?20\d{2})|(20\d{2}\s*[-~]\s*20\d{2})`)

// figurePattern finds a number with a unit attached, the kind a prompt supplies
// as a fact: "18%", "42개", "120억", "3배", "400M KRW", "12 months".
// Longer units come first: "12개월" is a duration, not twelve of something, and
// "400M KRW" is an amount rather than the letter M.
var figurePattern = regexp.MustCompile(`(?i)(\d[\d,.]*)\s*(%|퍼센트|억원|만원|개월|시간|주일|억|만|천|배|개|건|명|일|주|년|원|달러|` +
	`[kmb]n?\s*(?:KRW|USD|EUR|JPY|GBP)|KRW|USD|EUR|JPY|GBP|` +
	// The same units as the Korean ones, in the characters Japanese and Chinese
	// write them with: without these a brief's figures became slide titles.
	`億円|亿元|万円|万元|千円|か月|ヶ月|カ月|个月|小時|小时|時間|パーセント|億|亿|万|千|倍|割|円|元|人|名|件|個|个|日|天|週|周|月|年|` +
	// Counters a Korean brief uses for the things a company counts. Without them
	// "서울 1,200곳" is not a number at all: it stayed in the text, the topic
	// splitter made a slide subject of it, and a deck about four regions came out
	// as four slides titled after their own measurements. 장 is deliberately not
	// here — "10장으로 정리해줘" is how long the deck should be, not a figure.
	`곳|대|팀|회|차|점|종|층|석|위|단계|포인트|퍼센트포인트|분기|분|초|` +
	`[kmgt]b|km|kg|톤|평|㎡|m2|` +
	`months?|weeks?|quarters?|years?|days?|hours?|minutes?|` +
	`people|users?|customers?|accounts?|systems?|teams?|sites?|stores?|cases?|` +
	`percentage points?|pp|x)\b?`)

// maximumBriefFigures bounds what one brief's numbers can fill: a deck states
// the figures it argues from, and a brief with forty of them is a spreadsheet.
const maximumBriefFigures = 8

// topicSplitter breaks a subject into the things it names. Korean conjunctions
// and list punctuation both appear constantly.
// A topic never spans a full stop: two sentences are two thoughts, and joining
// them produces a "topic" that reads like a paragraph of the brief.
var topicSplitter = regexp.MustCompile(`\s*(?:[。！？]+|[.!?]+\s|[.!?]+$|,|·|/|、|，|;|；|:|：|및|그리고|\band\b|\bplus\b|\+)\s*`)

// joiningParticle is 와/과 used as "and". It is written attached to the word in
// front of it, which is also how a hundred ordinary words end — 효과, 결과,
// 성과, 경과 — and splitting on it wherever it appeared cut those in half: a
// deck about "협업 툴 도입 효과 측정 결과" had a section called "협업 툴 도입 효".
// The nouns that end this way have one syllable in front of the 과; a noun
// being joined to another has two or more.
var joiningParticle = regexp.MustCompile(`([가-힣]{2,}|[A-Za-z0-9]{2,})(?:와|과)\s`)

// outlinePrompt reads a prompt into the structure of a deck.
func outlinePrompt(prompt, title string, phrases languageCopy) promptOutline {
	prompt = strings.TrimSpace(prompt)
	outline := promptOutline{}
	if period := periodPattern.FindString(prompt); strings.TrimSpace(period) != "" {
		outline.Period = strings.Join(strings.Fields(period), " ")
	}
	// Every figure the brief states, then the first few that are figures. The
	// cap used to be applied to the matches, and a year matches: a brief that
	// says "2023년 820억, 2024년 910억, 2025년 1,040억" spent four of its eight on
	// years that are thrown away one line below, so the percentages after them
	// were never read at all — not into a chart, not into a kpi row, and not
	// into the check that asks which numbers the brief never gave.
	for _, match := range figurePattern.FindAllStringSubmatch(prompt, -1) {
		if len(outline.Figures) >= maximumBriefFigures {
			break
		}
		value := strings.TrimSpace(match[1] + match[2])
		// A year is a timeframe, not a figure, and the period already carries it.
		if outline.Period != "" && strings.Contains(outline.Period, match[1]) {
			continue
		}
		if match[2] == "년" && len(match[1]) == 4 && strings.HasPrefix(match[1], "20") {
			continue
		}
		// A quarter is a timeframe too. Read as a figure, "4분기 전망" became a
		// number the deck should draw rather than a section it should argue,
		// and a brief asking for "3분기 마감 결과와 4분기 전망" produced a deck
		// with no fourth quarter anywhere in it.
		if match[2] == "분기" && len(match[1]) == 1 && match[1][0] >= '1' && match[1][0] <= '4' {
			continue
		}
		label := figureLabel(prompt, match[0])
		outline.Figures = append(outline.Figures, promptFigure{Label: label, Value: value})
	}

	// How many slides is read before what they are about. "세 장을 넣어줘" is a
	// length written with the same verb a request is, and left in it made a
	// section called "클라우드 전환 계획을 세" out of the whole brief.
	prompt = slideCount.ReplaceAllString(prompt, " ")
	// The request for a section is read first: it is written with the same
	// verbs an instruction is — "추가해줘", "만들어 주세요", "부탁해" — and once
	// those have been stripped as instructions, "Q&A 슬라이드 추가해줘" is no
	// longer a request, just a subject reading "Q&A 슬라이드 추가".
	subject, asked := askedFor(prompt)
	subject = instructionPattern.ReplaceAllStringFunc(subject, keepDocumentNouns)
	// The room can be named at the front of a sentence whose verb comes at the
	// end: "개발팀에 배포 절차를 설명하는 자료" is addressed to the development
	// team, and the deck was titled "개발팀에 배포 절차".
	if tellingSomebody.MatchString(prompt) {
		subject = audienceAddress.ReplaceAllString(subject, " ")
	}
	subject = intending.ReplaceAllString(subject, " ")
	subject = leftMidVerb.ReplaceAllString(strings.TrimSpace(subject), "${1}")
	subject = stillNeeded.ReplaceAllString(subject, "${1}")
	subject = pastTense.ReplaceAllString(subject, "${1}")
	subject = comparedWith.ReplaceAllString(subject, "${1}")
	subject = strandedVerb.ReplaceAllString(subject, "${1}")
	subject = tidySeparators(subject)
	subject = strings.TrimSpace(strings.Join(strings.Fields(subject), " "))
	subject = cleanTopic(subject)
	if subject == "" {
		subject = strings.TrimSpace(title)
	}
	if subject == "" {
		subject = phrases.DefaultTopic
	}
	outline.Subject = subject

	for _, candidate := range splitTopics(subject) {
		// "목표 가용성 99.95%" is a figure the deck should show, not a subject it
		// should argue. Treating one as a topic gave it its own slides — and a
		// twelve-slide deck came out with the same step diagram three times.
		if figureClause(candidate, outline.Figures) || measurementOnly(candidate) || justAPeriod(candidate) ||
			statedFact(candidate, outline.Figures) {
			continue
		}
		name := topicPhrase(cleanTopic(candidate))
		// Shortening can leave the measurement and drop the words: "2026년 채용
		// 계획" became "2026년", and a section was titled after a year. The clause
		// it was cut from is the better heading; if that is a measurement too,
		// the brief did not name a subject here at all.
		if measurementOnly(name) {
			name = topicPhrase(cleanTopic(candidate + " "))
			if fuller := cleanTopic(candidate); !measurementOnly(fuller) {
				name = fuller
			}
		}
		if name == "" || utf8.RuneCountInString(name) < 2 || measurementOnly(name) {
			continue
		}
		frame, chosen := frameChosenFor(name)
		if !chosen {
			// Nothing in the words says how to argue this one. Three subjects that
			// all default to the same frame produce three slides with the same lead
			// and the same shape, so each takes a different angle instead.
			frame = defaultFrames[len(outline.Topics)%len(defaultFrames)]
		}
		outline.Topics = append(outline.Topics, promptTopic{Name: name, Frame: frame, Chosen: chosen})
	}
	// A subject that does not split is still one topic.
	if len(outline.Topics) == 0 {
		frame, chosen := frameChosenFor(subject)
		outline.Topics = []promptTopic{{Name: topicPhrase(subject), Frame: frame, Chosen: chosen}}
	}
	// A section somebody asked for by name is a section the deck has, whatever
	// else the brief is about.
	for _, name := range asked {
		if alreadyATopic(name, outline.Topics) {
			continue
		}
		frame, chosen := frameChosenFor(name)
		if !chosen {
			frame = defaultFrames[len(outline.Topics)%len(defaultFrames)]
		}
		outline.Topics = append(outline.Topics, promptTopic{Name: name, Frame: frame, Asked: true, Chosen: chosen})
	}
	return outline
}

// alreadyATopic says whether the deck is going to argue this subject anyway.
func alreadyATopic(name string, topics []promptTopic) bool {
	wanted := strings.ToLower(strings.Join(strings.Fields(name), ""))
	if wanted == "" {
		return true
	}
	for _, topic := range topics {
		held := strings.ToLower(strings.Join(strings.Fields(topic.Name), ""))
		if strings.Contains(held, wanted) || strings.Contains(wanted, held) {
			return true
		}
	}
	return false
}

// splitTopics divides a brief into the things it is about.
//
// A thousands separator is a comma, and the brief's own list separator is a
// comma. Splitting on both turned "매출 1,240억 원" into "매출 1" and "240억 원",
// and a board deck came out with a slide titled "240억 원". The separators
// inside numbers are hidden before the split and put back after it.
func splitTopics(subject string) []string {
	const shield = "\uE000"
	guarded := insideNumber.ReplaceAllString(subject, "${1}"+shield+"${2}")
	guarded = shieldBrackets(guarded, shield)
	guarded = joiningParticle.ReplaceAllString(guarded, "${1},")
	clauses := topicSplitter.Split(guarded, -1)
	for index, clause := range clauses {
		clauses[index] = strings.ReplaceAll(clause, shield, ",")
	}
	return clauses
}

// shieldBrackets hides the separators inside a parenthesis, because what is in
// one is an aside rather than another subject.
//
// "물류센터 자동화(AMR 20대) 도입 승인" was split at the space inside the bracket
// and became two topics: "물류센터 자동화(AMR" and "20대) 도입 승인". The deck then
// had slides titled after both halves, one of them carrying a bracket it never
// opened.
func shieldBrackets(subject, shield string) string {
	var out strings.Builder
	depth := 0
	for _, character := range subject {
		switch character {
		case '(', '（', '[', '｢', '「':
			depth++
		case ')', '）', ']', '｣', '」':
			if depth > 0 {
				depth--
			}
		}
		if depth > 0 && (character == ',' || character == '，' || character == '·' || character == '/') {
			out.WriteString(shield)
			continue
		}
		out.WriteRune(character)
	}
	return out.String()
}

// insideNumber is a separator between two digits: part of the number, not a
// break between clauses.
var insideNumber = regexp.MustCompile(`(\d)[,，](\d)`)

// statedFact reports whether a clause is the brief telling the writer something
// rather than naming a section.
//
// A good brief states its facts — "현재 오라클 라이선스가 연 4억이고, 전환 대상은
// 리포팅 DB 12개, 목표는 2026년 3분기 완료야" is exactly what somebody who wants a
// useful deck writes. Every one of those clauses became a slide heading, cut
// where the reader stopped: "현재 오라클 라이선스가 연", "전환 대상은 리포팅 DB",
// "목표는 2026년 3분기 완료야". The figures had already been read out of the same
// clauses, so the brief was being used twice — once as data, once as a heading
// that says nothing.
//
// What separates the two is grammar, not content. A section name is a noun
// phrase; a statement has a subject and a predicate. Both are required here:
// a marked subject on its own would take "위험은 무엇인가" with it, and a figure
// on its own would take "클라우드 비용 최적화 3개년 로드맵".
func statedFact(clause string, figures []promptFigure) bool {
	trimmed := strings.TrimSpace(clause)
	if trimmed == "" || !hasMarkedSubject(trimmed) {
		return false
	}
	if endsInPredicate(trimmed) {
		return true
	}
	// The clause is where one of the brief's numbers came from, so the number is
	// already kept — as a figure, which is what the deck will draw.
	for _, figure := range figures {
		value := strings.TrimSpace(figure.Value)
		label := strings.TrimSpace(figure.Label)
		if value != "" && strings.Contains(trimmed, value) &&
			(label == "" || strings.Contains(trimmed, label)) {
			return true
		}
	}
	return false
}

// hasMarkedSubject reports whether some word other than the last carries a
// subject or topic marker. The last word is skipped because a phrase can end in
// one of those syllables as part of itself — 방안, 결과가 아닌 것들.
func hasMarkedSubject(clause string) bool {
	words := strings.Fields(clause)
	if len(words) < 2 {
		return false
	}
	for _, word := range words[:len(words)-1] {
		for _, marker := range []string{"은", "는", "이", "가"} {
			if strings.HasSuffix(word, marker) && utf8.RuneCountInString(word) >= 3 {
				return true
			}
		}
	}
	return false
}

// endsInPredicate reports whether the clause finishes the way a sentence does.
func endsInPredicate(clause string) bool {
	words := strings.Fields(clause)
	if len(words) == 0 {
		return false
	}
	last := strings.Trim(words[len(words)-1], " .,·!?")
	if verbLike(last) {
		return true
	}
	for _, ending := range []string{"이고", "이며", "이다", "이야", "야", "인데", "라서", "이었고", "였고", "이랑"} {
		if strings.HasSuffix(last, ending) && utf8.RuneCountInString(last) > utf8.RuneCountInString(ending) {
			return true
		}
	}
	return false
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

// measurementOnly reports whether a clause is a measurement with barely a word
// attached, whichever way the brief wrote it.
//
// figureClause needs the figure to have been read into the outline, and the
// outline keeps only the first few. Past that cap "2026년" and "영업 6명" came
// through as subjects, and each was given a section of its own: four slides
// titled after a year.
func measurementOnly(clause string) bool {
	if latinPhrase(clause) {
		return latinMeasurementOnly(clause)
	}
	if !figurePattern.MatchString(clause) {
		// A short subject is still a subject: "목차" is two letters and no number.
		return false
	}
	words := strings.TrimSpace(figurePattern.ReplaceAllString(strings.TrimSpace(clause), " "))
	words = strings.TrimSpace(strings.Join(strings.Fields(words), ""))
	// Which side the number is on says what it is doing. "영업 6명" and "매출
	// 1,040억" name a thing and then measure it — that is a datum, and it
	// belongs on a slide rather than being one. "4분기 전망" and "2026년 채용
	// 계획" are the other way round: the number says when, and the noun after it
	// is the subject. Treating both alike threw the second kind away, and a deck
	// briefed as "3분기 마감 결과와 4분기 전망" came out with no fourth quarter
	// anywhere in it.
	if leads := figurePattern.FindStringIndex(strings.TrimSpace(clause)); leads != nil && leads[0] == 0 {
		return utf8.RuneCountInString(words) < 2
	}
	return utf8.RuneCountInString(words) < 3
}

// latinMeasurementOnly reads the same question in a script where the units are
// written as words and the number carries a currency mark rather than a counter:
// figurePattern's list of Korean units cannot see "$1.8M".
//
// A figure line is a label and a number, in that order and nothing after it —
// "Investment $1.8M", "Direct 46%". A subject that mentions a number has words
// on both sides of it: "First half 2026 results", "Target 240 contracts".
func latinMeasurementOnly(clause string) bool {
	fields := strings.Fields(strings.TrimSpace(clause))
	first := -1
	for index, field := range fields {
		if strings.ContainsAny(field, "0123456789") {
			first = index
			break
		}
	}
	if first < 0 {
		return false
	}
	before, after := 0, 0
	for index, field := range fields {
		if strings.ContainsAny(field, "0123456789") {
			continue
		}
		word := strings.ToLower(strings.Trim(field, " .,:;()"))
		if word == "" || latinPrepositions[word] || latinArticles[word] {
			continue
		}
		if index < first {
			before++
			continue
		}
		after++
	}
	return after == 0 && before <= 1
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
			// A connective verb ends its clause rather than vanishing from it.
			// Dropping "줄이고" out of "배치 지연 6시간을 30분으로 줄이고 저장 비용 40%
			// 절감" glued the two halves into one heading that reads as neither.
			if len(words) > 0 {
				break
			}
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
	// Whatever is left, keep its tail: Korean puts the head noun last. The cut
	// has to fall between clauses, though. "고객 이탈률 개선을 위한 리텐션 전략"
	// is one rune over the limit, and taking a word off the front left four
	// slides in a row titled "개선을 위한 리텐션 전략" — improving what? A word
	// carrying an object particle, or a modifier waiting for its noun, is the
	// middle of a clause and not a place to start one.
	for start := 0; start < len(words); start++ {
		candidate := strings.Join(words[start:], " ")
		if utf8.RuneCountInString(candidate) > limit {
			continue
		}
		if start > 0 && midClause(words[start]) {
			continue
		}
		return cleanTopic(candidate)
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
// midClause says a word carries on from the one before it rather than starting
// something: an object or subject particle waiting for its verb, or a modifier
// waiting for its noun.
func midClause(word string) bool {
	trimmed := strings.Trim(word, " .,·")
	if utf8.RuneCountInString(trimmed) < 2 {
		return false
	}
	for _, ending := range []string{"을", "를", "이", "가", "은", "는", "와", "과", "의", "에"} {
		if strings.HasSuffix(trimmed, ending) {
			return true
		}
	}
	return modifierForm(trimmed) || trimmed == "위한" || trimmed == "위해" || trimmed == "및" || trimmed == "그리고"
}

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
		"을", "를", "은", "는",
		// The same tail in Japanese. Only the markers that are unmistakable at
		// the end of a phrase: の, に and で are as often part of a word as they
		// are a particle.
		"について", "に関して", "を", "は", "が", "へ"} {
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
	frame, _ := frameChosenFor(topic)
	return frame
}

// frameChosenFor is frameFor with the difference that matters kept: whether the
// words said how to argue this topic, or nothing at all and the situation frame
// is standing in.
func frameChosenFor(topic string) (string, bool) {
	lowered := strings.ToLower(topic)
	// The word that decides is the one that ends last. Korean puts the head noun
	// last — "도입 효과" is a slide about 효과, not about 도입 — and taking the
	// first table entry that matched anywhere made it a roadmap. Where two words
	// end together the longer one is the more specific: "기대효과" over "효과".
	frame, ends, length := frameSituation, -1, 0
	for _, entry := range frameMarkers {
		for _, pattern := range entry.patterns {
			index := strings.LastIndex(lowered, pattern)
			if index < 0 {
				continue
			}
			finish := index + len(pattern)
			if finish > ends || (finish == ends && len(pattern) > length) {
				frame, ends, length = entry.frame, finish, len(pattern)
			}
		}
	}
	return frame, ends >= 0
}

// figureLabel takes the few words before a number as its label, which is where
// prompts put it: "전환 대상 42개 시스템".
func figureLabel(prompt, match string) string {
	if label := figureLabelName(rawFigureLabel(prompt, match)); label != "" {
		return label
	}
	// Korean puts the term after the number as readily as before it — "3년 내
	// 회수", "18개월 손익분기" — and a figure with no label is dropped from the
	// chart entirely, so the brief said something the deck never shows.
	return figureLabelName(trailingFigureLabel(prompt, match))
}

// trailingFigureLabel reads the words after a number, for when nothing before
// it names what it counts.
func trailingFigureLabel(prompt, match string) string {
	index := strings.Index(prompt, match)
	if index < 0 {
		return ""
	}
	after := strings.TrimSpace(prompt[index+len(match):])
	if cut := strings.IndexAny(after, ",:·、，;；：/.。!?！？"); cut >= 0 {
		after = strings.TrimSpace(after[:cut])
	}
	if after == "" {
		return ""
	}
	if spacelessScript(after) {
		runes := []rune(after)
		const reach = 8
		if len(runes) > reach {
			runes = runes[:reach]
		}
		return strings.TrimSpace(string(runes))
	}
	fields := strings.Fields(after)
	// "3년 내 회수" is the payback, not the 내: a positional word on its own says
	// where rather than what.
	if len(fields) > 1 && positionalWords[fields[0]] {
		fields = fields[1:]
	}
	return strings.Join(fields[:min(len(fields), 2)], " ")
}

// positionalWords say where something sits rather than what it is.
var positionalWords = map[string]bool{
	"내": true, "안": true, "후": true, "뒤": true, "이내": true, "이후": true, "만에": true,
	"within": true, "after": true, "over": true, "per": true,
}

// figureLabelName repairs a label the same way a heading is repaired. It is cut
// out of the brief's sentence exactly as a heading is, and goes wrong the same
// way: "물류센터 자동화(AMR 20대)" labelled its figure "물류센터 자동화(AMR", with a
// bracket it never closes, and "배치 지연 6시간을 30분으로" labelled 30분 with
// "지연 6시간을" — a measurement labelling another measurement.
//
// Only what is certainly not part of the label goes. A label is two words at
// most already, and taking more would leave the number with nothing to say what
// it counts.
func figureLabelName(label string) string {
	repaired := withoutBrokenBrackets(strings.TrimSpace(label))
	if fields := strings.Fields(repaired); len(fields) > 1 {
		last := fields[len(fields)-1]
		if figurePattern.MatchString(last) {
			if shorter := strings.TrimSpace(strings.Join(fields[:len(fields)-1], " ")); shorter != "" {
				repaired = shorter
			}
		}
	}
	if repaired = strings.Trim(repaired, " .,·-—:()[]"); repaired == "" {
		return strings.TrimSpace(label)
	}
	return repaired
}

// wordStart backs a cut out of the middle of a word.
//
// A script without spaces still has runs that are one word — katakana, Latin,
// digits — and counting back a fixed number of characters lands inside one:
// "平均オンボーディング期間" was labelled "ボーディング期間". The cut moves back to
// where the run began, or forward past it if that reaches too far.
func wordStart(runes []rune, start int) int {
	if start <= 0 || start >= len(runes) {
		return start
	}
	if !sameRun(runes[start-1], runes[start]) {
		return start
	}
	const reachBack = 8
	for back := start; back > 0 && start-back < reachBack; back-- {
		if !sameRun(runes[back-1], runes[back]) {
			return back
		}
	}
	// The run is longer than the label may be. Start after it instead of inside.
	for forward := start; forward < len(runes); forward++ {
		if !sameRun(runes[forward-1], runes[forward]) {
			return forward
		}
	}
	return start
}

// sameRun reports whether two characters belong to the same unbroken word.
func sameRun(before, after rune) bool {
	switch {
	case unicode.Is(unicode.Katakana, before) || before == 'ー':
		return unicode.Is(unicode.Katakana, after) || after == 'ー'
	case unicode.IsDigit(before):
		return unicode.IsDigit(after) || after == '.' || after == ','
	case unicode.Is(unicode.Latin, before):
		return unicode.Is(unicode.Latin, after)
	}
	return false
}

func rawFigureLabel(prompt, match string) string {
	index := strings.Index(prompt, match)
	if index <= 0 {
		return ""
	}
	before := strings.TrimSpace(prompt[:index])
	// A label is the words immediately before the number, and it never reaches
	// back past punctuation: "…도입 효과: 개발 속도 32%" labels the 32%, not the
	// sentence it sits in.
	// A sentence end is the firmest barrier of them all: "…도입 승인 요청. 투자
	// 18억" labelled the 18억 with "요청. 투자", carrying a word out of the
	// sentence before it.
	if cut := strings.LastIndexAny(before, ",:·、，;；：/.。!?！？"); cut >= 0 {
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
			runes = runes[wordStart(runes, len(runes)-reach):]
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
		// What a date was attached to comes with a particle — "2026年の採用計画"
		// leaves "の採用計画" — and a heading does not open on one.
		rest := strings.TrimLeft(strings.TrimSpace(trimmed[len(prefix):]), " のノ의・·、,")
		if utf8.RuneCountInString(rest) >= 4 {
			trimmed = rest
		}
	}
	return withoutTrailingFigure(withoutAside(withoutBrokenBrackets(trimmed)))
}

// withoutTrailingFigure drops a measurement from the end of a heading.
//
// A brief that says "채널 비중은 직판 46%, 대리점 33%" gives its first clause as a
// subject, and the subject came out as the title of three slides — "채널 비중은
// 직판 46% — 기대 효과". A title says what the slide is about; the number belongs
// on it, drawn, and not in the heading twice.
// withoutAside drops a parenthesis from a heading. What is inside one is detail
// — "자동화(AMR 20대) 도입 승인" — and a heading is the subject: the detail belongs
// on the slide, in a point or a figure, where there is room to read it.
func withoutAside(name string) string {
	trimmed := strings.TrimSpace(name)
	for _, pair := range []struct{ open, close string }{{"(", ")"}, {"（", "）"}} {
		for {
			open := strings.Index(trimmed, pair.open)
			if open < 0 {
				break
			}
			close := strings.Index(trimmed[open:], pair.close)
			if close < 0 {
				break
			}
			candidate := strings.TrimSpace(trimmed[:open]) + " " + strings.TrimSpace(trimmed[open+close+len(pair.close):])
			candidate = strings.TrimSpace(strings.Join(strings.Fields(candidate), " "))
			if utf8.RuneCountInString(candidate) < 3 {
				return trimmed
			}
			trimmed = candidate
		}
	}
	return trimmed
}

// withoutBrokenBrackets drops a bracket a heading does not close.
//
// A subject is cut out of the brief's own sentence, and the cut can fall inside
// a parenthesis: "물류센터 자동화(AMR 20대) 도입 승인" gave a slide titled
// "20대) 도입 승인", and another titled "20대) 도입 승인 — 기대 효과". A heading
// with an orphan bracket in it reads as a mistake before it reads as anything.
func withoutBrokenBrackets(name string) string {
	trimmed := strings.TrimSpace(name)
	for _, pair := range []struct{ open, close string }{{"(", ")"}, {"（", "）"}, {"[", "]"}, {"{", "}"}} {
		opens, closes := strings.Count(trimmed, pair.open), strings.Count(trimmed, pair.close)
		if opens == closes {
			continue
		}
		if closes > opens {
			// The words before the stray closing bracket belong to a phrase this
			// heading does not have: what is left after it is the subject.
			if at := strings.Index(trimmed, pair.close); at >= 0 {
				if rest := strings.TrimSpace(trimmed[at+len(pair.close):]); utf8.RuneCountInString(rest) >= 3 {
					trimmed = rest
					continue
				}
			}
		}
		// An opening bracket with nothing to close it: the heading ends where the
		// bracket began.
		if at := strings.LastIndex(trimmed, pair.open); at > 0 {
			if rest := strings.TrimSpace(trimmed[:at]); utf8.RuneCountInString(rest) >= 3 {
				trimmed = rest
			}
		}
	}
	return trimmed
}

func withoutTrailingFigure(name string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(name), " ,·")
	for {
		shorter := withoutOneTrailingFigure(trimmed)
		shorter = withoutTrailingParticle(shorter)
		if shorter == trimmed {
			return trimmed
		}
		trimmed = shorter
	}
}

func withoutOneTrailingFigure(name string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(name), " ,·")
	body := strings.TrimRight(trimmed, headingParticles)
	// The last match, not the first: "…2026년 3분기" ends in a measurement even
	// though the first one it contains sits in the middle.
	all := figurePattern.FindAllStringIndex(body, -1)
	if len(all) == 0 {
		return trimmed
	}
	found := all[len(all)-1]
	if found[1] != len(body) {
		return trimmed
	}
	rest := strings.TrimRight(strings.TrimSpace(body[:found[0]]), " ,·")
	// What is left has to still be a subject: "18억" alone is a figure, and a
	// heading of one word that the brief only wrote as a label for a number is
	// not better than the words it came with.
	if utf8.RuneCountInString(rest) < 3 {
		return trimmed
	}
	return rest
}

// headingParticles are the 조사 that can sit behind a measurement in a heading —
// "3분기에", "6시간을" — and are part of the sentence the figure was cut from
// rather than part of the figure.
const headingParticles = "은는이가의을를에서으로"

// withoutTrailingParticle drops a case marker a heading was cut off in the
// middle of: "유지보수 서비스를" is half a sentence, "유지보수 서비스" is a subject.
//
// Only the markers that a Korean noun does not end in are trimmed. 이, 가, 의
// and 에 are left alone, because "전문가" and "회의" end in them and are words.
func withoutTrailingParticle(name string) string {
	trimmed := strings.TrimSpace(name)
	if latinPhrase(trimmed) {
		// "Payback in 3 years" gives up its measurement and is left introducing
		// something that is no longer there.
		fields := strings.Fields(trimmed)
		for len(fields) > 1 {
			last := strings.ToLower(strings.Trim(fields[len(fields)-1], " .,"))
			if !latinPrepositions[last] && !latinArticles[last] {
				break
			}
			fields = fields[:len(fields)-1]
		}
		return strings.Join(fields, " ")
	}
	if spacelessScript(trimmed) {
		// 売上は is what a sentence says about 売上, and a heading names the thing.
		for _, particle := range []string{"は", "が", "を", "には", "では", "としては"} {
			shortened := strings.TrimSuffix(trimmed, particle)
			if shortened != trimmed && utf8.RuneCountInString(shortened) >= 2 {
				return shortened
			}
		}
		return trimmed
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return trimmed
	}
	last := fields[len(fields)-1]
	for _, particle := range []string{"으로", "에서", "을", "를", "은", "는"} {
		if !strings.HasSuffix(last, particle) {
			continue
		}
		shortened := strings.TrimSuffix(last, particle)
		if utf8.RuneCountInString(shortened) < 2 {
			return trimmed
		}
		fields[len(fields)-1] = shortened
		return strings.Join(fields, " ")
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
	// Stripping the instruction can take the words in front of a full stop and
	// leave the stop behind: "…사업 계획을 임원에게 보고. 매출 목표 1조" becomes
	// "…사업 계획을 . 매출 목표 1조". What follows that stop was a different
	// sentence, and a title is one thought.
	subject = beforeStrandedStop(subject)
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
		candidate := withoutSubjectParticle(cleanTopic(strings.Join(words[start:], " ")))
		if candidate != "" && utf8.RuneCountInString(candidate) <= limit {
			return candidate, start > 0
		}
	}
	return "", true
}

// withoutSubjectParticle drops the 이/가 a sentence marks its subject with.
//
// It is only ever dropped from a phrase of several words. On its own the same
// syllable is the end of an ordinary noun — 높이, 길이, 손잡이 — and a heading
// reading "높" would be worse than one reading "높이".
func withoutSubjectParticle(name string) string {
	words := strings.Fields(strings.TrimSpace(name))
	if len(words) < 2 {
		return name
	}
	last := words[len(words)-1]
	for _, particle := range []string{"이", "가"} {
		shortened := strings.TrimSuffix(last, particle)
		if shortened != last && utf8.RuneCountInString(shortened) >= 2 {
			words[len(words)-1] = shortened
			return strings.Join(words, " ")
		}
	}
	return name
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
	for _, ending := range []string{"에게", "께", "한테", "대상으로", "위해", "위한"} {
		if strings.HasSuffix(trimmed, ending) && utf8.RuneCountInString(trimmed) > utf8.RuneCountInString(ending) {
			return true
		}
	}
	// "보고용", "고객용": 용 says what something is for. It is also the last
	// syllable of a great many ordinary words — 비용, 내용, 채용, 사용, 활용,
	// 운용, 적용 — and taking it for a purpose cut every heading short at the
	// first of them: "클라우드 비용 최적화 방안" was titled "클라우드", and
	// "신규 채용 계획" was titled "신규". It is a purpose only when the word in
	// front of it names one.
	for _, ending := range []string{"용으로", "용의", "용"} {
		if strings.HasSuffix(trimmed, ending) {
			return purposeWords[strings.TrimSuffix(trimmed, ending)]
		}
	}
	return false
}

// purposeWords are the things a deck is made for.
var purposeWords = map[string]bool{
	"보고": true, "발표": true, "공유": true, "제출": true, "설명": true, "안내": true,
	"소개": true, "교육": true, "홍보": true, "배포": true, "인쇄": true, "참고": true,
	"검토": true, "승인": true, "회의": true, "업무": true,
	"내부": true, "외부": true, "사내": true, "대외": true,
	"고객": true, "임원": true, "경영진": true, "투자자": true, "직원": true, "개인": true,
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
	// The cut falls between clauses. Shortening "고객 이탈률 개선을 위한 리텐션
	// 전략" a word at a time gave four slides in a row the heading "개선을 위한
	// 리텐션 전략" — improving what?
	for _, betweenClauses := range []bool{true, false} {
		for start := 0; start < len(words); start++ {
			if betweenClauses && start > 0 && midClause(words[start]) {
				continue
			}
			candidate := cleanTopic(strings.Join(words[start:], " "))
			if candidate != "" && utf8.RuneCountInString(candidate) <= limit {
				return candidate
			}
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

// strandedStop is a full stop standing on its own, which is what a stripped
// instruction leaves behind.
var strandedStop = regexp.MustCompile(`\s+[.。!?]+(\s|$)`)

// beforeStrandedStop keeps the first thought of a subject whose sentence lost
// its verb to the instruction pattern.
//
// A title is one thought whether the stop was stranded or written that way:
// "재고 회전율이. 원인 분석과 개선안" reads as two headings run together, which is
// what it is.
func beforeStrandedStop(subject string) string {
	if match := strandedStop.FindStringIndex(subject); match != nil {
		if head := strings.TrimSpace(subject[:match[0]]); utf8.RuneCountInString(head) >= 4 {
			return head
		}
	}
	if match := sentenceEnd.FindStringIndex(subject); match != nil {
		if head := strings.TrimSpace(subject[:match[0]]); utf8.RuneCountInString(head) >= 4 {
			return head
		}
	}
	return subject
}

// sentenceEnd is a full stop that ends a sentence rather than sitting inside a
// number or an abbreviation: it has to be followed by a space or nothing.
var sentenceEnd = regexp.MustCompile(`[.。!?！？](?:\s|$)`)
