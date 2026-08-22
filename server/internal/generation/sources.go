package generation

import (
	"fmt"
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
func keepAttributedSources(source, brief string) (string, int) {
	if !strings.Contains(source, "!source") {
		return source, 0
	}
	haystack := normalizeForMatch(brief)
	lines := strings.Split(source, "\n")
	kept := make([]string, 0, len(lines))
	dropped := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "!source") && !strings.HasPrefix(trimmed, "!출처") {
			kept = append(kept, line)
			continue
		}
		if attributed(trimmed, haystack) {
			kept = append(kept, line)
			continue
		}
		dropped++
	}
	if dropped == 0 {
		return source, 0
	}
	return strings.Join(kept, "\n"), dropped
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
