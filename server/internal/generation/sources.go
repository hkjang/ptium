package generation

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// keepAttributedSources removes citations the brief does not support.
//
// The model is told to cite only what the brief attributes and to invent
// nothing. Asked for a deck from a brief that names no source at all, it wrote
// three anyway — "내부 시장 보고서", "고객 경험 분석 데이터", "보안 감사 보고서" —
// dated this month. Those are printed at the foot of the slide and listed in the
// speaker notes, where they read as evidence.
//
// A citation nobody can produce is worse than no citation: it is the one thing
// on the slide that will not survive being asked about. So a source whose own
// words are not in the brief is dropped, and the deck says how many went.
func keepAttributedSources(source, brief string) (string, int, int) {
	if !strings.Contains(source, "!source") {
		return source, 0, 0
	}
	haystack := normalizeForMatch(brief)
	lines := strings.Split(source, "\n")
	kept := make([]string, 0, len(lines))
	dropped, vague := 0, 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "!source") && !strings.HasPrefix(trimmed, "!출처") {
			kept = append(kept, line)
			continue
		}
		if !attributed(trimmed, haystack) {
			dropped++
			continue
		}
		if shortened, trimmedOff := withSupportedLocator(line, haystack); trimmedOff {
			vague++
			kept = append(kept, shortened)
			continue
		}
		kept = append(kept, line)
	}
	if dropped == 0 && vague == 0 {
		return source, 0, 0
	}
	return strings.Join(kept, "\n"), dropped, vague
}

// withSupportedLocator removes a locator the brief does not carry.
//
// The name of a source is the harder thing to invent, so a model that has been
// told to cite the brief mostly gets it right — and then makes up where in it.
// For a brief that says "내부 결제 로그 기준 지난 12개월" it wrote
// "!source 내부 결제 로그 | 2026-03": a real system, a month nobody named. That
// is worse than the invented source it replaced, because the name checks out and
// only the part nobody verifies is fiction.
//
// The citation survives without it. "내부 결제 로그" printed at the foot of the
// slide is true; "2026-03" is a claim about a page of it.
func withSupportedLocator(line, haystack string) (string, bool) {
	directive, value, found := strings.Cut(strings.TrimSpace(line), " ")
	if !found {
		return line, false
	}
	title, locator, hasLocator := strings.Cut(value, "|")
	if !hasLocator || strings.TrimSpace(locator) == "" {
		return line, false
	}
	if locatorSupported(locator, haystack) {
		return line, false
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	return indent + directive + " " + strings.TrimSpace(title), true
}

// cellRange is a locator written the way a spreadsheet writes one, "A1:C9" or
// "Sheet1!A1:C9". Ptium writes these itself when a deck is built from an
// attached sheet: they are coordinates into the attachment, not a claim about
// what it says, and the brief has no reason to contain them.
var cellRange = regexp.MustCompile(`^([^!]+!)?[a-z]{1,3}\d{1,7}(:[a-z]{1,3}\d{1,7})?$`)

// locatorSupported reports whether every claim in a locator is in the brief. A
// page, a table or a month is a claim; the word "page" is not, and neither is a
// cell reference.
func locatorSupported(locator, haystack string) bool {
	if cellRange.MatchString(strings.TrimSpace(normalizeForMatch(locator))) {
		return true
	}
	for _, token := range strings.FieldsFunc(normalizeForMatch(locator), func(symbol rune) bool {
		return unicode.IsSpace(symbol) || unicode.IsPunct(symbol)
	}) {
		if genericLocatorWords[token] {
			continue
		}
		if !hasDigit(token) && utf8.RuneCountInString(token) < 2 {
			continue
		}
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func hasDigit(value string) bool {
	return strings.ContainsAny(value, "0123456789")
}

// genericLocatorWords name the part of a document rather than which part.
var genericLocatorWords = map[string]bool{
	"페이지": true, "쪽": true, "부록": true, "기준": true, "자료": true, "표": true, "그림": true,
	"장": true, "절": true, "항": true, "page": true, "table": true, "figure": true, "fig": true,
	"appendix": true, "section": true, "exhibit": true, "chart": true, "sheet": true, "slide": true,
	"no": true, "p": true, "pp": true,
}

// attributed reports whether the brief carries the words of this citation.
//
// The comparison is by word rather than by phrase: a model that writes "통계청
// 2026 소비 동향" for a brief that says "통계청 2026 소비 동향(표 3)" is citing the
// brief, and one that writes "내부 시장 보고서" for a brief that mentions neither
// is not. Words every citation contains — 보고서, 자료, 데이터 — carry no weight,
// or every invented source would pass on the strength of the word "report".
func attributed(line, haystack string) bool {
	_, value, _ := strings.Cut(line, " ")
	title, _, _ := strings.Cut(value, "|")
	words := distinctiveWords(title)
	if len(words) == 0 {
		// Nothing but generic words: "출처: 내부 자료" cites nothing at all.
		return false
	}
	for _, word := range words {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

// genericSourceWords are the words that appear in the name of every source and
// so distinguish none of them.
var genericSourceWords = map[string]bool{
	"보고서": true, "자료": true, "데이터": true, "분석": true, "결과": true, "내부": true,
	"기준": true, "조사": true, "리포트": true, "report": true, "data": true, "internal": true,
	"analysis": true, "survey": true, "study": true, "results": true,
}

func distinctiveWords(value string) []string {
	var words []string
	for _, field := range strings.FieldsFunc(normalizeForMatch(value), func(symbol rune) bool {
		return unicode.IsSpace(symbol) || unicode.IsPunct(symbol)
	}) {
		if utf8.RuneCountInString(field) < 2 || genericSourceWords[field] {
			continue
		}
		words = append(words, field)
	}
	return words
}

func normalizeForMatch(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

// vagueLocatorNote is what the deck says about the locators it trimmed.
func vagueLocatorNote(count int, language string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(language), "ja"):
		return fmt.Sprintf("出典の該当箇所%d件はブリーフに書かれていないため削除し、出典名だけを残しました。", count)
	case strings.HasPrefix(strings.ToLower(language), "zh"):
		return fmt.Sprintf("%d 处来源的具体位置简报中没有，已删除，仅保留来源名称。", count)
	case strings.HasPrefix(strings.ToLower(language), "ko"), strings.TrimSpace(language) == "":
		return fmt.Sprintf("출처의 위치 표기 %d건은 브리프에 없어 지우고 출처 이름만 남겼습니다. 표나 페이지를 아신다면 직접 적어 주세요.", count)
	}
	return fmt.Sprintf("%d source locator(s) the brief does not give were removed; the source names remain. "+
		"Add the page or table yourself if you know it.", count)
}

// invented is what the deck says about the citations it dropped.
func inventedSourceNote(count int, language string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(language), "ja"):
		return fmt.Sprintf("ブリーフに書かれていない出典を%d件、AIが作成したため削除しました。必要な出典はスライドに直接追記してください。", count)
	case strings.HasPrefix(strings.ToLower(language), "zh"):
		return fmt.Sprintf("AI 生成了 %d 条简报中没有的来源，已删除。如需注明来源，请直接在幻灯片中补充。", count)
	case strings.HasPrefix(strings.ToLower(language), "ko"), strings.TrimSpace(language) == "":
		return fmt.Sprintf("브리프에 없는 출처 %d건을 AI가 지어내 지웠습니다. 필요한 출처는 슬라이드에 직접 적어 주세요.", count)
	}
	return fmt.Sprintf("%d source(s) the brief does not mention were invented by the model and removed. "+
		"Add the sources you need to the slides themselves.", count)
}
