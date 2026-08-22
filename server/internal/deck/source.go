package deck

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

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
	// Groups are the second and further columns: each one is a heading and the
	// points written after it. The first column has no entry — it is the slide's
	// lead and the points before the next heading — so a slide with one lead has
	// no groups at all and behaves exactly as it always did.
	Groups []SourceGroup
	// Sources are where the slide's figures came from. A deck that states a
	// number and cannot say where it is from is the first thing anyone in a
	// company asks about, so the language carries the answer beside the claim.
	Sources []SourceCitation
	// Line is where the slide began, for error reporting.
	Line int
}

// SourceCitation is one source a slide cites, as written in source:
//
//   - 매출 1,240억 ^1
//     !source 1 | 2026 시장 조사 보고서 | p.42
//
// Marker is what the claim carries ("1"), Title names the source and Locator
// says where in it to look.
// SourceGroup is a column: a heading, and where in the slide's points its own
// points begin.
type SourceGroup struct {
	Heading string
	From    int
}

type SourceCitation struct {
	Marker  string
	Title   string
	Locator string
	Line    int
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
	// A component a model closed before writing its rows is kept open for the
	// lines that follow, because that is what it meant. The rows arrive as loose
	// prose otherwise, and the component draws nothing.
	var hungry *SourceBlock
	closeBlock := func() {
		hungry = nil
		if inBlock {
			if len(block.Items) > 0 {
				current.Blocks = append(current.Blocks, block)
			} else {
				empty := block
				current.Blocks = append(current.Blocks, empty)
				hungry = &current.Blocks[len(current.Blocks)-1]
			}
		}
		inBlock, block = false, SourceBlock{}
	}
	flush := func() {
		closeBlock()
		hungry = nil
		promoteTabularBullets(&current)
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
				current.Notes = tidyText(current.Notes + " " + trimmed)
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
			hungry = nil
			flush()
			current, started = SourceSlide{Title: unescapePayload(strings.TrimLeft(trimmed, "#")), Line: line}, true

		case strings.HasPrefix(trimmed, "@"):
			begin(line)
			// A model routinely writes the title without its "# ". A slide kind on
			// the line after a bare line means that line was the title, so it is
			// promoted rather than left as a lead.
			if current.Title == "" && current.Lead != "" && len(current.Bullets) == 0 && len(current.Blocks) == 0 {
				current.Title, current.Lead = current.Lead, ""
			}
			directive := strings.TrimSpace(trimmed[1:])
			name, value, hasValue := strings.Cut(directive, " ")
			if strings.EqualFold(strings.TrimSpace(name), "layout") {
				if !hasValue || strings.TrimSpace(value) == "" {
					warn(line, "@layout needs a layout id")
					continue
				}
				current.LayoutID = layoutReference(value)
				continue
			}
			if role, ok := roleAliases[strings.ToLower(directive)]; ok {
				current.Role = role
				continue
			}
			warn(line, "unknown slide kind %q", directive)

		case strings.HasPrefix(trimmed, ">"):
			hungry = nil
			begin(line)
			lead := unescapePayload(trimmed[1:])
			// A second lead after points have started is a second column: it heads
			// the points that follow it. Two-column slides are written this way by
			// anyone describing two sides of something — and by the model — and
			// gluing the two headings into one sentence, which is what happened
			// before, puts both of them over the left column and leaves the right
			// one bare.
			if current.Lead == "" {
				current.Lead = lead
			} else if len(current.Bullets) > 0 {
				current.Groups = append(current.Groups, SourceGroup{Heading: lead, From: len(current.Bullets)})
			} else {
				current.Lead += " " + lead
			}

		case strings.HasPrefix(trimmed, "!"):
			hungry = nil
			begin(line)
			name, value, _ := strings.Cut(strings.TrimPrefix(trimmed, "!"), " ")
			lowered := strings.ToLower(strings.TrimSpace(name))
			if lowered == "source" || lowered == "출처" {
				if citation, ok := parseCitation(value, line); ok {
					current.Sources = append(current.Sources, citation)
				} else {
					warn(line, "!source needs a title: !source 1 | 2026 시장 조사 보고서 | p.42")
				}
				continue
			}
			if !strings.HasPrefix(lowered, "note") {
				warn(line, "unknown directive %q", name)
				continue
			}
			current.Notes = tidyText(current.Notes + " " + strings.TrimSpace(value))
			inNotes = true

		case strings.HasPrefix(trimmed, "-"), strings.HasPrefix(trimmed, "*"):
			begin(line)
			item := strings.TrimSpace(trimmed[1:])
			if inBlock {
				block.Items = append(block.Items, parseSourceItem(item))
				block.Rows = append(block.Rows, itemFields(item))
				continue
			}
			if hungry != nil {
				// The rows of a component that was closed too early.
				hungry.Items = append(hungry.Items, parseSourceItem(item))
				hungry.Rows = append(hungry.Rows, itemFields(item))
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
			case hungry != nil:
				hungry.Items = append(hungry.Items, parseSourceItem(trimmed))
				hungry.Rows = append(hungry.Rows, itemFields(trimmed))
			case current.Lead == "" && len(current.Bullets) == 0 && countRowFields(trimmed) < 2:
				current.Lead = trimmed
			default:
				current.Bullets = append(current.Bullets, pptx.Paragraph{Text: tidyText(trimmed)})
			}
		}
	}
	flush()
	return result
}

// promoteTabularBullets turns a run of pipe-separated bullets into the component
// it plainly is. Asked for a table, a model often writes the rows as ordinary
// bullets — "- 단일 서버 의존 | 마이크로서비스 분산" — or as a markdown table,
// and drawn as prose those pipes are just noise on the slide. A run of two or
// more rows with the same column count is a table; nothing else is touched.
func promoteTabularBullets(slide *SourceSlide) {
	if len(slide.Bullets) < 2 {
		return
	}
	start, end, columns := -1, -1, 0
	for index := 0; index <= len(slide.Bullets); index++ {
		count := 0
		if index < len(slide.Bullets) {
			count = countRowFields(slide.Bullets[index].Text)
		}
		if count >= 2 && (start < 0 || count == columns) {
			if start < 0 {
				start, columns = index, count
			}
			continue
		}
		if start >= 0 && index-start >= 2 {
			end = index
			break
		}
		start, columns = -1, 0
		if count >= 2 {
			start, columns = index, count
		}
	}
	if end < 0 {
		return
	}

	rows := make([][]string, 0, end-start)
	items := make([]pptx.Item, 0, end-start)
	for _, bullet := range slide.Bullets[start:end] {
		if isTableRule(bullet.Text) {
			continue
		}
		rows = append(rows, itemFields(bullet.Text))
		items = append(items, parseSourceItem(bullet.Text))
	}
	if len(rows) < 2 {
		return
	}
	kind := "table"
	if columns == 2 {
		// Two columns of words are a comparison; two columns where the second is
		// a figure are indicators. Both read far better than a table of two.
		kind = "comparison"
		if allNumeric(rows, 1) {
			kind = "kpi"
		}
	}
	slide.Blocks = append(slide.Blocks, SourceBlock{Kind: kind, Items: items, Rows: rows, Line: slide.Line})
	slide.Bullets = append(slide.Bullets[:start:start], slide.Bullets[end:]...)
}

// countRowFields is how many columns a row really has: a markdown row's
// surrounding pipes are punctuation, not empty first and last columns.
func countRowFields(text string) int {
	return len(trimTableRow(splitItemFields(text)))
}

// isTableRule matches the "|---|---|" line markdown puts under a header row.
func isTableRule(text string) bool {
	fields := trimTableRow(splitItemFields(text))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" || strings.Trim(trimmed, "-:") != "" {
			return false
		}
	}
	return len(fields) > 0
}

// allNumeric reports whether every row carries a figure in the given column.
func allNumeric(rows [][]string, column int) bool {
	for _, row := range rows {
		if column >= len(row) {
			return false
		}
		if _, ok := parseNumber(row[column]); !ok {
			return false
		}
	}
	return true
}

// layoutReference is the layout a model meant. It writes the name plainly more
// often than not, but it also writes "id=제목-및-내용" or a quoted name, and a
// reference the parser does not recognise silently moves the slide to another
// layout.
func layoutReference(value string) string {
	reference := strings.TrimSpace(value)
	for _, prefix := range []string{"id=", "id:", "name=", "name:", "layout=", "layout:"} {
		if len(reference) > len(prefix) && strings.EqualFold(reference[:len(prefix)], prefix) {
			reference = strings.TrimSpace(reference[len(prefix):])
		}
	}
	return strings.Trim(reference, `"'`)
}

// empty reports whether a slide has nothing on it, which is what a stray
// directive between slides produces.
func (s SourceSlide) empty() bool {
	return s.Title == "" && s.Lead == "" && len(s.Bullets) == 0 &&
		len(s.Blocks) == 0 && len(s.Images) == 0 && s.Notes == ""
}

// koreanUnitSpace matches a number separated from its Korean unit by a space,
// which a model produces constantly and a reader never writes.
//
// The unit has to end where the word ends. Without that, "2026 시장 조사" became
// "2026시장 조사" — the list holds 시 as a unit and 시장 begins with it. So what
// follows the unit must be anything but another Hangul syllable, unless that
// syllable begins a particle or a suffix, which is how a real unit continues:
// "4시간에서", "3단계로", "2장짜리".
var koreanUnitSpace = regexp.MustCompile(
	`([0-9%）\)\]])[ \t]+(개월|시간|주일|퍼센트|포인트|분기|단계|가지|주차|일차|페이지|배수|년|월|일|시|분|초|주|억|만|천|원|개|건|명|장|배|회|위|인|곳|층|쪽|권|편|톤|칸|줄|%)` +
		`([^가-힣]|[에으을를은는이가도만과와의로부까지간째당짜씩여반]|$)`)

// koreanTrailingSpace matches a unit separated from the particle or suffix that
// follows it, which is the same mistake one syllable later: "15% 씩", "3년 간".
var koreanTrailingSpace = regexp.MustCompile(`(%|년|월|일|개|건|명|억|만|원|배|시간)[ \t]+(씩|간|째|당|여|분의|이내|이상|이하)`)

// koreanForeignParticle matches a particle written apart from a word that is not
// itself Korean — "deliverables 를", "94% 의", "Q4 와" — which is where a model
// writing Korean leaves a space no Korean writer would. Two Korean words are
// left alone: a space between them usually belongs, and the ones that do not are
// a matter of judgement rather than of rule.
var koreanForeignParticle = regexp.MustCompile(
	`([0-9A-Za-z%）\)\]])[ \t]+(이라는|에게서|으로서|으로써|에서는|에서도|까지는|부터는|이라고|라는|에서|에게|으로|까지|부터|보다|처럼|이며|이고|와의|과의|을|를|은|는|이|가|의|에|로|와|과|도|만)([ \t\n,.;:!?)\]}]|$)`)

// tidyText fixes the typography a model gets wrong in Korean. It only removes a
// space that should not be there; nothing else about the wording is touched.
func tidyText(value string) string {
	return TidyKorean(strings.TrimSpace(value))
}

// TidyKorean closes the gaps a model leaves in Korean text. It is exported for
// the generation pipeline, which runs it over what the model wrote so that the
// deck's own text — the source the workspace shows — reads the way the slides do.
func TidyKorean(value string) string {
	for {
		tidied := koreanUnitSpace.ReplaceAllString(value, "$1$2$3")
		tidied = koreanTrailingSpace.ReplaceAllString(tidied, "$1$2")
		tidied = koreanForeignParticle.ReplaceAllString(tidied, "$1$2$3")
		if tidied == value {
			return value
		}
		value = tidied
	}
}

// unescapePayload removes the one escape the language has: a backslash in front
// of text that would otherwise be read as markup. It is applied to what a
// directive carries, not to the directive itself.
func unescapePayload(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `\`) {
		value = strings.TrimSpace(value[1:])
	}
	return tidyText(value)
}

// itemFields is a row's fields, trimmed and unescaped.
func itemFields(text string) []string {
	parts := trimTableRow(splitItemFields(text))
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		fields = append(fields, tidyText(part))
	}
	return fields
}

// trimTableRow drops the empty fields a markdown-style row leaves at its ends.
// A model writes "| 기존 방식 | 신규 방식 |" as often as "기존 방식 | 신규 방식",
// and splitting the first produces a blank first and last column, which collapses
// a two-column row into one nameless entry.
func trimTableRow(fields []string) []string {
	for len(fields) > 1 && strings.TrimSpace(fields[0]) == "" {
		fields = fields[1:]
	}
	for len(fields) > 1 && strings.TrimSpace(fields[len(fields)-1]) == "" {
		fields = fields[:len(fields)-1]
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
	parts := trimTableRow(splitItemFields(text))
	item := pptx.Item{Label: tidyText(parts[0])}
	if len(parts) > 1 {
		raw := tidyText(parts[1])
		item.Value = raw
		if number, ok := parseNumber(raw); ok {
			item.Number = &number
		}
	}
	if len(parts) > 2 {
		item.Detail = tidyText(strings.Join(parts[2:], " | "))
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

// TitleFromSource is what a deck written in this language calls itself: the
// heading of its first slide.
func TitleFromSource(source string) string {
	parsed := ParseSource(source)
	if len(parsed.Slides) == 0 {
		return ""
	}
	return strings.TrimSpace(parsed.Slides[0].Title)
}

// parseCitation reads "1 | 제목 | p.42". The marker is optional: a slide with
// one source can simply name it.
func parseCitation(value string, line int) (SourceCitation, bool) {
	fields := splitItemFields(value)
	for index := range fields {
		fields[index] = strings.TrimSpace(unescapePayload(fields[index]))
	}
	citation := SourceCitation{Line: line}
	switch {
	case len(fields) == 0:
		return SourceCitation{}, false
	case len(fields) == 1:
		citation.Title = fields[0]
	default:
		// A leading field that is only a marker — "1", "a", "*" — is the marker;
		// otherwise the first field is the title and the rest is where to look.
		if isCitationMarker(fields[0]) {
			citation.Marker = fields[0]
			citation.Title = fields[1]
			if len(fields) > 2 {
				citation.Locator = strings.Join(fields[2:], " ")
			}
		} else {
			citation.Title = fields[0]
			citation.Locator = strings.Join(fields[1:], ", ")
		}
	}
	if strings.TrimSpace(citation.Title) == "" {
		return SourceCitation{}, false
	}
	return citation, true
}

// isCitationMarker reports whether a field is the short mark a claim carries
// rather than the source's name.
//
// Outside the Latin alphabet a mark is one character. "가" is a mark; "통계청"
// is the national statistics office, and reading a three-syllable Korean
// publisher as a footnote marker loses the one word the audience asked for.
// Korean, Japanese and Chinese institutions are named in two and three
// characters — that is the normal case there, not the exception.
func isCitationMarker(value string) bool {
	trimmed := strings.TrimSpace(value)
	length := utf8.RuneCountInString(trimmed)
	if trimmed == "" || length > 3 {
		return false
	}
	for _, symbol := range trimmed {
		if !unicode.IsDigit(symbol) && !unicode.IsLetter(symbol) && symbol != '*' && symbol != '†' {
			return false
		}
		if symbol > unicode.MaxASCII && length > 1 {
			return false
		}
	}
	return true
}
