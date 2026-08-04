package deck

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// A deck is written as source before it is drawn.
//
// The source is a small line-oriented language, close enough to Markdown that a
// person can read and edit it without instruction, and strict enough that a
// language model can emit it reliably. It exists because the interesting part of
// generating a deck is the structure — what each slide argues, in what form —
// and structure is much easier to review, diff and correct as text than as a
// tree of boxes.
//
//	# 클라우드 전환 로드맵          a slide, with its title
//	@cover                        which kind of slide this is
//	> 임원 보고 · 2026 하반기       the lead line under the title
//	- 첫 번째 요점                 a bullet
//	  - 딸린 근거                  a sub-bullet, indented two spaces
//	::kpi 핵심 지표                a component, drawn rather than written
//	- 전환 대상 | 42개
//	- 예상 절감 | 18%
//	::
//	!notes 예산 질문이 나오면…      speaker notes
//	// this line is a comment
//
// Compiling it against a template produces slides bound to that template's real
// layouts and slots, so the same source drawn into two different templates comes
// out as two properly designed decks.

// SourceSlide is one slide as written in source, before it is bound to a layout.
type SourceSlide struct {
	Title    string
	Lead     string
	Role     string
	LayoutID string
	Bullets  []pptx.Paragraph
	Blocks   []SourceBlock
	Images   []SourceImage
	Notes    string
	// Line is where the slide began, for error reporting.
	Line int
}

// SourceImage is an image a slide places, as written in source.
type SourceImage struct {
	// Reference is the image's name or id, as the author wrote it.
	Reference string
	Caption   string
	Line      int
}

// SourceBlock is a component as written in source.
type SourceBlock struct {
	Kind    string
	Caption string
	// Definition names the stored grid a ::grid component is drawn from.
	Definition string
	Items      []pptx.Item
	// Rows keeps each row's fields verbatim. A table has as many columns as its
	// author wrote, which label/value/detail cannot express.
	Rows [][]string
	Line int
}

// Source is a parsed deck.
type Source struct {
	Slides []SourceSlide
	// Warnings describe lines that were not understood. Parsing never fails: a
	// deck that is half-written should still draw what it has.
	Warnings []string
}

// roleAliases maps the words an author (or a model) actually writes to Ptium's
// layout roles.
var roleAliases = map[string]string{
	"cover": pptx.RoleTitle, "title": pptx.RoleTitle, "표지": pptx.RoleTitle,
	"section": pptx.RoleSection, "divider": pptx.RoleSection, "간지": pptx.RoleSection,
	"content": pptx.RoleContent, "body": pptx.RoleContent, "본문": pptx.RoleContent,
	"two": pptx.RoleTwoContent, "twocontent": pptx.RoleTwoContent, "split": pptx.RoleTwoContent,
	"comparison": pptx.RoleComparison, "compare": pptx.RoleComparison, "비교": pptx.RoleComparison,
	"quote": pptx.RoleQuote, "statement": pptx.RoleQuote, "인용": pptx.RoleQuote,
	"picture": pptx.RolePicture, "image": pptx.RolePicture,
	"table": pptx.RoleTable, "chart": pptx.RoleChart,
	"closing": pptx.RoleClosing, "end": pptx.RoleClosing, "마무리": pptx.RoleClosing,
	"blank": pptx.RoleBlank,
}

// canonicalRoleName is the word Format writes for a role. The alias table is for
// reading, and iterating it to find a name would make Format's output depend on
// map order — which would turn every re-read of a deck into a spurious diff.
var canonicalRoleName = map[string]string{
	pptx.RoleTitle: "cover", pptx.RoleSection: "section", pptx.RoleContent: "content",
	pptx.RoleTwoContent: "two", pptx.RoleComparison: "comparison", pptx.RoleQuote: "quote",
	pptx.RolePicture: "picture", pptx.RoleTable: "table", pptx.RoleChart: "chart",
	pptx.RoleClosing: "closing", pptx.RoleBlank: "blank",
}

// ParseSource reads deck source. It is deliberately forgiving: unknown
// directives become warnings rather than errors, because a half-written deck
// should still show what it has.
func ParseSource(source string) Source {
	var result Source
	// The slide under construction is held by value and flushed when the next one
	// starts: a pointer into result.Slides would dangle the moment that slice grew.
	var current SourceSlide
	var block SourceBlock
	started, inBlock, inNotes := false, false, false

	warn := func(line int, format string, args ...any) {
		result.Warnings = append(result.Warnings, fmt.Sprintf("line %d: %s", line, fmt.Sprintf(format, args...)))
	}
	closeBlock := func() {
		if inBlock && len(block.Items) > 0 {
			current.Blocks = append(current.Blocks, block)
		}
		inBlock, block = false, SourceBlock{}
	}
	flush := func() {
		closeBlock()
		if started && !current.empty() {
			result.Slides = append(result.Slides, current)
		}
		current, started = SourceSlide{}, false
	}
	begin := func(line int) {
		if !started {
			current, started = SourceSlide{Line: line}, true
		}
	}

	for index, raw := range strings.Split(source, "\n") {
		line := index + 1
		text := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(text)

		// Notes run until the next directive or blank line, so a note can be a
		// whole paragraph.
		if inNotes {
			if trimmed != "" && !isDirective(trimmed) {
				current.Notes = strings.TrimSpace(current.Notes + " " + trimmed)
				continue
			}
			inNotes = false
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		switch {
		case trimmed == "::":
			closeBlock()

		case strings.HasPrefix(trimmed, "::"):
			closeBlock()
			begin(line)
			kind, caption, _ := strings.Cut(strings.TrimSpace(trimmed[2:]), " ")
			// An image is not a component: it references something stored rather
			// than describing something to draw.
			if strings.EqualFold(strings.TrimSpace(kind), "image") || strings.TrimSpace(kind) == "이미지" {
				reference, label, _ := strings.Cut(caption, "|")
				reference = unescapePayload(reference)
				if reference == "" {
					warn(line, "::image needs the name or id of an uploaded image")
					continue
				}
				current.Images = append(current.Images, SourceImage{
					Reference: reference, Caption: strings.TrimSpace(unescapePayload(label)), Line: line})
				continue
			}
			resolved := pptx.BlockKind(kind)
			if resolved == "" {
				warn(line, "unknown component %q", strings.TrimSpace(kind))
				continue
			}
			block, inBlock = SourceBlock{Kind: resolved, Caption: strings.TrimSpace(caption), Line: line}, true
			if resolved == pptx.BlockGrid {
				// A grid names its definition first: "::grid raci 담당 체계".
				definition, rest, _ := strings.Cut(strings.TrimSpace(caption), " ")
				if definition == "" {
					definition = strings.ToLower(strings.TrimSpace(kind))
				}
				block.Definition, block.Caption = definition, strings.TrimSpace(rest)
			}

		case strings.HasPrefix(trimmed, "#"):
			flush()
			current, started = SourceSlide{Title: unescapePayload(strings.TrimLeft(trimmed, "#")), Line: line}, true

		case strings.HasPrefix(trimmed, "@"):
			begin(line)
			directive := strings.TrimSpace(trimmed[1:])
			name, value, hasValue := strings.Cut(directive, " ")
			if strings.EqualFold(strings.TrimSpace(name), "layout") {
				if !hasValue || strings.TrimSpace(value) == "" {
					warn(line, "@layout needs a layout id")
					continue
				}
				current.LayoutID = strings.TrimSpace(value)
				continue
			}
			if role, ok := roleAliases[strings.ToLower(directive)]; ok {
				current.Role = role
				continue
			}
			warn(line, "unknown slide kind %q", directive)

		case strings.HasPrefix(trimmed, ">"):
			begin(line)
			lead := unescapePayload(trimmed[1:])
			if current.Lead == "" {
				current.Lead = lead
			} else {
				current.Lead += " " + lead
			}

		case strings.HasPrefix(trimmed, "!"):
			begin(line)
			name, value, _ := strings.Cut(strings.TrimPrefix(trimmed, "!"), " ")
			if !strings.HasPrefix(strings.ToLower(name), "note") {
				warn(line, "unknown directive %q", name)
				continue
			}
			current.Notes = strings.TrimSpace(current.Notes + " " + strings.TrimSpace(value))
			inNotes = true

		case strings.HasPrefix(trimmed, "-"), strings.HasPrefix(trimmed, "*"):
			begin(line)
			item := strings.TrimSpace(trimmed[1:])
			if inBlock {
				block.Items = append(block.Items, parseSourceItem(item))
				block.Rows = append(block.Rows, itemFields(item))
				continue
			}
			current.Bullets = append(current.Bullets, pptx.Paragraph{Text: unescapePayload(item), Level: bulletLevel(text)})

		default:
			// A bare line is prose: the lead when there is none yet, a bullet
			// otherwise. Authors write this constantly and always mean one of the two.
			begin(line)
			switch {
			case inBlock:
				block.Items = append(block.Items, parseSourceItem(trimmed))
				block.Rows = append(block.Rows, itemFields(trimmed))
			case current.Lead == "" && len(current.Bullets) == 0:
				current.Lead = trimmed
			default:
				current.Bullets = append(current.Bullets, pptx.Paragraph{Text: trimmed})
			}
		}
	}
	flush()
	return result
}

// empty reports whether a slide has nothing on it, which is what a stray
// directive between slides produces.
func (s SourceSlide) empty() bool {
	return s.Title == "" && s.Lead == "" && len(s.Bullets) == 0 &&
		len(s.Blocks) == 0 && len(s.Images) == 0 && s.Notes == ""
}

// unescapePayload removes the one escape the language has: a backslash in front
// of text that would otherwise be read as markup. It is applied to what a
// directive carries, not to the directive itself.
func unescapePayload(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `\`) {
		return strings.TrimSpace(value[1:])
	}
	return value
}

// itemFields is a row's fields, trimmed and unescaped.
func itemFields(text string) []string {
	parts := splitItemFields(text)
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		fields = append(fields, strings.TrimSpace(part))
	}
	return fields
}

// splitItemFields splits a component row on its unescaped pipes, so a label may
// contain one.
func splitItemFields(text string) []string {
	var fields []string
	var current strings.Builder
	escaped := false
	for _, character := range text {
		switch {
		case escaped:
			if character != '|' && character != '\\' {
				current.WriteRune('\\')
			}
			current.WriteRune(character)
			escaped = false
		case character == '\\':
			escaped = true
		case character == '|':
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(character)
		}
	}
	if escaped {
		current.WriteRune('\\')
	}
	return append(fields, current.String())
}

func isDirective(trimmed string) bool {
	for _, prefix := range []string{"#", "@", ">", "-", "*", "::", "!", "//"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// bulletLevel reads nesting from leading whitespace, two spaces per level, which
// is what an author gets from pressing tab in any editor.
func bulletLevel(text string) int {
	indent := 0
	for _, character := range text {
		switch character {
		case ' ':
			indent++
		case '\t':
			indent += 2
		default:
			return min(indent/2, 4)
		}
	}
	return 0
}

// parseSourceItem reads "label | value | detail", any part of which may be
// omitted. A value that looks like a number is kept as one so components can
// draw it to scale.
func parseSourceItem(text string) pptx.Item {
	parts := splitItemFields(text)
	item := pptx.Item{Label: strings.TrimSpace(parts[0])}
	if len(parts) > 1 {
		raw := strings.TrimSpace(parts[1])
		item.Value = raw
		if number, ok := parseNumber(raw); ok {
			item.Number = &number
		}
	}
	if len(parts) > 2 {
		item.Detail = strings.TrimSpace(strings.Join(parts[2:], " | "))
	}
	return item
}

// parseNumber pulls a magnitude out of a written value: "18%", "42개",
// "1,200억", "-3.5pt" all carry one.
func parseNumber(value string) (float64, bool) {
	var digits strings.Builder
	seenDigit := false
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
			digits.WriteRune(character)
			seenDigit = true
		case character == '.' && seenDigit:
			digits.WriteRune(character)
		case (character == '-' || character == '+') && digits.Len() == 0:
			digits.WriteRune(character)
		case character == ',':
			// A thousands separator, not a decimal point.
		default:
			if seenDigit {
				// Stop at the first unit so "42개 / 18%" reads as 42.
				number, err := strconv.ParseFloat(digits.String(), 64)
				return number, err == nil
			}
		}
	}
	if !seenDigit {
		return 0, false
	}
	number, err := strconv.ParseFloat(digits.String(), 64)
	return number, err == nil
}
