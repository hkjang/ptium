package generation

import (
	"regexp"
	"strconv"
	"strings"
)

// maximumIntentSlides bounds what counts as a deck length written in a prompt.
//
// It is not the deployment's limit, and setting it to the same number made a
// cliff at exactly that limit: "50장짜리 자료를 만들어 줘" gave fifty slides and
// "51장짜리" gave the default ten, with nothing said. A count one over what a
// site allows is still a request — it is just one the site cannot grant, and
// the cap already clamps it and says so.
//
// What the bound is really for is telling a deck length from a number that is
// about something else. Every pattern requires a slide word beside the number,
// so what is left to guard against is "이 100페이지 보고서를 요약해 줘" — a
// source document, not a deck. Two digits next to 장 or 페이지 is a deck length
// somebody meant; three is far more often the thing being summarised.
const maximumIntentSlides = 99

// Intent is what the prompt itself asks for, as opposed to what the form
// controls happen to be set to.
//
// A person who writes "3장만 만들어줘" has stated the deck length more precisely
// than a slider they never touched, and a product that ignores it looks broken.
// Everything here is optional: a zero value means the prompt said nothing.
type Intent struct {
	// SlideCount is the number of slides the prompt asks for, 0 if unstated.
	SlideCount int
	// Language is a BCP-47-ish short code the prompt asks to be written in.
	Language string
	// Audience is who the prompt says the deck is for.
	Audience string
}

// ParseIntent reads the instructions a prompt states about the deck itself.
func ParseIntent(prompt string) Intent {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Intent{}
	}
	return Intent{
		SlideCount: promptSlideCount(prompt),
		Language:   promptLanguage(prompt),
		Audience:   promptAudience(prompt),
	}
}

// ApplySlideCount resolves the deck length from, in order: what the caller
// explicitly asked for, what the prompt says, and the deployment default.
func (intent Intent) ApplySlideCount(explicit, fallback, maximum int) int {
	return clampCount(intent.SlideCountAsked(explicit, fallback), maximum)
}

// SlideCountAsked is the length the request asked for, before any cap this
// deployment puts on it.
//
// It exists so the caller can tell whether the answer is the number that was
// asked for. A deployment capped at five slides gave a request for ten a deck
// of five, and said nothing: the same cap on an imported file and on applied
// source both announce themselves, and only the front door was quiet.
func (intent Intent) SlideCountAsked(explicit, fallback int) int {
	switch {
	case explicit > 0:
		return explicit
	case intent.SlideCount > 0:
		return intent.SlideCount
	default:
		return fallback
	}
}

func clampCount(value, maximum int) int {
	if maximum < 1 {
		maximum = 50
	}
	if value < 1 {
		return 1
	}
	if value > maximum {
		return maximum
	}
	return value
}

// Slide-count phrasings, in the order they are tried. Each captures the number
// last, and the leading group exists only to require a non-digit boundary: RE2
// has no lookbehind, and without it "120장" would be read as "20".
//
// Three digits are captured on purpose. A number outside the supported range is
// then rejected rather than truncated into a plausible-looking one.
var slideCountPatterns = []*regexp.Regexp{
	// "5~7장", "5-7 slides": a range, of which the upper bound is honoured
	// because a deck may always come in shorter than its ceiling.
	regexp.MustCompile(`(?i)(?:^|[^\d])(\d{1,3})\s*[~\-–]\s*(\d{1,3})\s*(?:장|매|쪽|페이지|slides?|pages?)`),
	// "3장", "10 페이지", "7 slides", "3-slide deck"
	regexp.MustCompile(`(?i)(?:^|[^\d])(\d{1,3})\s*(?:장|매|쪽|페이지|[- ]?slides?|[- ]?pages?)`),
	// "슬라이드 3개", "슬라이드는 5장", "페이지 4개"
	regexp.MustCompile(`(?i)(?:슬라이드|장수|페이지)\s*(?:는|은|를|을|수는)?\s*(?:^|[^\d])?(\d{1,3})\s*(?:개|장|매|쪽)?`),
	// "slides: 4", "slide count 6", "deck of 8"
	regexp.MustCompile(`(?i)(?:slide\s*count|slides?|pages?|deck\s+of)\s*[:=]?\s*(\d{1,3})`),
}

// koreanNumerals covers the counted forms a person actually writes before 장.
var koreanNumerals = map[string]int{
	"한": 1, "하나": 1, "두": 2, "둘": 2, "세": 3, "셋": 3, "네": 4, "넷": 4,
	"다섯": 5, "여섯": 6, "일곱": 7, "여덟": 8, "아홉": 9, "열": 10,
	"열한": 11, "열두": 12, "열세": 13, "열네": 14, "열다섯": 15, "스무": 20, "스물": 20,
	"일": 1, "이": 2, "삼": 3, "사": 4, "오": 5, "육": 6, "칠": 7, "팔": 8, "구": 9, "십": 10,
}

var englishNumerals = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7,
	"eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "fifteen": 15, "twenty": 20,
}

var koreanNumeralPattern = regexp.MustCompile(`(하나|열다섯|열한|열두|열세|열네|스물|스무|한|두|세|네|넷|다섯|여섯|일곱|여덟|아홉|열)\s*(?:장|매|쪽|페이지)`)

var englishNumeralPattern = regexp.MustCompile(`(?i)\b(one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|fifteen|twenty)[- ](?:slides?|pages?)`)

// chapterPattern marks 장 used as "chapter", where a number is a heading and not
// a length: "제3장", "3장에서".
var chapterPattern = regexp.MustCompile(`제\s*\d{1,2}\s*장`)

func promptSlideCount(prompt string) int {
	cleaned := chapterPattern.ReplaceAllString(prompt, " ")
	for index, pattern := range slideCountPatterns {
		match := pattern.FindStringSubmatch(cleaned)
		if match == nil {
			continue
		}
		// A range keeps its upper bound; every other pattern has one number.
		value := match[len(match)-1]
		if index == 0 && len(match) > 2 {
			value = match[2]
		}
		count, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		if count >= 1 && count <= maximumIntentSlides {
			return count
		}
		// A number too large to be a deck length says nothing about the deck.
		return 0
	}
	if match := koreanNumeralPattern.FindStringSubmatch(cleaned); match != nil {
		if count, ok := koreanNumerals[match[1]]; ok {
			return count
		}
	}
	if match := englishNumeralPattern.FindStringSubmatch(cleaned); match != nil {
		if count, ok := englishNumerals[strings.ToLower(match[1])]; ok {
			return count
		}
	}
	return 0
}

var languageRequests = []struct {
	code    string
	pattern *regexp.Regexp
}{
	{"ko", regexp.MustCompile(`(?i)한국어로|국문으로|한글로|in Korean`)},
	{"en", regexp.MustCompile(`(?i)영어로|영문으로|in English|English version`)},
	{"ja", regexp.MustCompile(`(?i)일본어로|日本語で|in Japanese`)},
	{"zh", regexp.MustCompile(`(?i)중국어로|中文で|用中文|in Chinese`)},
}

func promptLanguage(prompt string) string {
	for _, request := range languageRequests {
		if request.pattern.MatchString(prompt) {
			return request.code
		}
	}
	return ""
}

// audienceRequests recognises who a deck is addressed to, which changes how the
// narrative is pitched even when the form field was left at its default.
var audienceRequests = []struct {
	audience string
	pattern  *regexp.Regexp
}{
	{"임원", regexp.MustCompile(`임원|경영진|C-?level|executives?|이사회|보드`)},
	{"고객", regexp.MustCompile(`고객|클라이언트|customers?|clients?`)},
	{"투자자", regexp.MustCompile(`투자자|투자 유치|VC|investors?`)},
	{"개발팀", regexp.MustCompile(`개발팀|엔지니어|engineers?|developers?`)},
	{"신입", regexp.MustCompile(`신입|온보딩|onboarding|new hires?`)},
}

// addressedTo is somebody saying outright who the deck is for: "팀에 공유할",
// "임원에게 보고합니다".
var addressedTo = regexp.MustCompile(
	`([가-힣A-Za-z]{1,10}?)\s*(?:에게|께|한테|에)\s*(?:보고|공유|발표|설명|제출|안내|소개|배포)`)

func promptAudience(prompt string) string {
	// What the brief says the deck is for beats what the brief is about. A
	// note on onboarding new developers "팀에 공유할 자료" is written for the
	// team; reading 신입 out of the subject addressed it to the new joiners.
	if said := addressedTo.FindStringSubmatch(prompt); said != nil {
		named := strings.TrimSpace(said[1])
		for _, request := range audienceRequests {
			if request.pattern.MatchString(named) {
				return request.audience
			}
		}
		if named != "" {
			return named
		}
	}
	for _, request := range audienceRequests {
		if request.pattern.MatchString(prompt) {
			return request.audience
		}
	}
	return ""
}
