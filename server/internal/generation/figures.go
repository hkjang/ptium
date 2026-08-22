package generation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// figuresNotInBrief lists the figures a written deck states that the brief
// never gave it.
//
// Asked for a deck about a 12억 원 investment, the model wrote "가용성 99.99%
// 확보 목표" — a number nobody in the room can source, on the slide that asks
// for the money. The rule against inventing one was written under "Rules for
// components", so prose was never covered by it.
//
// Unlike an invented source, an invented figure cannot be deleted: it is inside
// a sentence, and cutting it would leave the sentence saying something else. So
// the deck keeps it and says which numbers it introduced, and the author decides.
func figuresNotInBrief(source, brief string) []string {
	haystack := digitsOnly(brief)
	seen := map[string]bool{}
	var missing []string
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "::") {
			continue
		}
		if strings.HasPrefix(trimmed, "!source") || strings.HasPrefix(trimmed, "!출처") {
			// A locator is a page or a table number, not a claim.
			continue
		}
		for _, figure := range pptx.StatedFigures(trimmed) {
			figure = strings.TrimSpace(figure)
			number := digitsOnly(leadingNumber.FindString(figure))
			if number == "" || seen[figure] {
				continue
			}
			if strings.Contains(haystack, number) {
				continue
			}
			seen[figure] = true
			missing = append(missing, figure)
		}
	}
	return missing
}

var leadingNumber = regexp.MustCompile(`\d[\d,.]*`)

// digitsOnly makes "1,200" and "1200" the same number, which is the only
// difference between how a brief writes a figure and how a deck does.
func digitsOnly(value string) string {
	value = strings.ReplaceAll(value, ",", "")
	return strings.TrimSuffix(strings.TrimSpace(value), ".")
}

// inventedFigureNote is what the deck says about the numbers it introduced.
func inventedFigureNote(figures []string, language string) string {
	shown := figures
	if len(shown) > 4 {
		shown = shown[:4]
	}
	list := strings.Join(shown, ", ")
	if len(figures) > len(shown) {
		list += fmt.Sprintf(" 외 %d", len(figures)-len(shown))
	}
	switch {
	case strings.HasPrefix(strings.ToLower(language), "ja"):
		return fmt.Sprintf("ブリーフにない数字が使われています: %s。出典を確認できない数字は差し替えてください。", list)
	case strings.HasPrefix(strings.ToLower(language), "zh"):
		return fmt.Sprintf("以下数字简报中没有: %s。无法核实的数字请替换。", list)
	case strings.HasPrefix(strings.ToLower(language), "ko"), strings.TrimSpace(language) == "":
		return fmt.Sprintf("브리프에 없는 숫자가 들어 있습니다: %s. 근거를 댈 수 없는 숫자는 고쳐 주세요.", list)
	}
	list = strings.Join(shown, ", ")
	if len(figures) > len(shown) {
		list += fmt.Sprintf(" and %d more", len(figures)-len(shown))
	}
	return fmt.Sprintf("These figures are not in the brief: %s. Replace any number you cannot source.", list)
}

// deckSourceOf is the deck as it stands after compiling and repair, falling back
// to what the model returned when compiling produced no source.
func deckSourceOf(result Deck, written string) string {
	if strings.TrimSpace(result.Source) != "" {
		return result.Source
	}
	return written
}
