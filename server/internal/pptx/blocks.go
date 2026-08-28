package pptx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Block kinds. A slide body is either prose or one of these components, which
// is what separates a designed deck from a wall of bullets.
const (
	BlockBullets    = "bullets"
	BlockKPI        = "kpi"
	BlockHero       = "hero"
	BlockSteps      = "steps"
	BlockTimeline   = "timeline"
	BlockComparison = "comparison"
	BlockColumns    = "columnChart"
	BlockBars       = "barChart"
	BlockLine       = "lineChart"
	BlockShare      = "shareBar"
	BlockMeter      = "meter"
	BlockTable      = "table"
	BlockQuote      = "quote"
	BlockCallout    = "callout"
	// BlockGrid is a component an organisation defined: a labelled grid whose
	// cell values are drawn from a stored definition.
	BlockGrid = "grid"
)

// BlockKinds lists every component the renderer supports, for prompts and
// validation.
func BlockKinds() []string {
	return []string{BlockBullets, BlockKPI, BlockHero, BlockSteps, BlockTimeline,
		BlockComparison, BlockColumns, BlockBars, BlockLine, BlockShare,
		BlockMeter, BlockTable, BlockQuote, BlockCallout, BlockGrid}
}

// Item is one entry of a component: a statistic, a step, a milestone, a bar.
type Item struct {
	Label   string   `json:"label,omitempty"`
	Value   string   `json:"value,omitempty"`
	Number  *float64 `json:"number,omitempty"`
	Delta   string   `json:"delta,omitempty"`
	Trend   string   `json:"trend,omitempty"`
	Detail  string   `json:"detail,omitempty"`
	Bullets []string `json:"bullets,omitempty"`
}

// number returns the numeric weight of an item, falling back to a value string
// that happens to be numeric.
func (i Item) number() float64 {
	if i.Number != nil {
		return *i.Number
	}
	cleaned := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return -1
	}, i.Value)
	parsed, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// Display returns the label to draw for an item's value.
func (i Item) Display(unit string) string {
	if strings.TrimSpace(i.Value) != "" {
		return i.Value
	}
	if i.Number == nil {
		return ""
	}
	return formatNumber(*i.Number) + unit
}

// Series is one line of a multi-series chart.
type Series struct {
	Name   string    `json:"name,omitempty"`
	Points []float64 `json:"points"`
}

// Block is a visual component on a slide.
type Block struct {
	Kind    string     `json:"kind"`
	Heading string     `json:"heading,omitempty"`
	Items   []Item     `json:"items,omitempty"`
	Series  []Series   `json:"series,omitempty"`
	Labels  []string   `json:"labels,omitempty"`
	Columns []string   `json:"columns,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
	Unit    string     `json:"unit,omitempty"`
	// Grid is the definition a grid component is drawn from. It travels with the
	// slide so a deck renders the same way after the definition changes.
	Grid *GridSpec `json:"grid,omitempty"`
	// Span names further regions the component covers, beside the one it is
	// placed in. A comparison matrix or a chart in a two-column layout reads far
	// better across the whole body than squeezed into half of it, and the regions
	// it covers are drawn as one.
	Span []string `json:"span,omitempty"`
	// Text is the single statement a quote or callout carries.
	Text      string `json:"text,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Emphasis  int    `json:"emphasis,omitempty"` // 1-based item to highlight
	Attribute string `json:"attribute,omitempty"`
}

// maximumBlockItems caps a component so it stays readable at slide scale.
const maximumBlockItems = 8

// DrawableItems is how many entries a component of this kind puts on a slide.
//
// Five steps across a slide is already a tight row and a sixth would be
// unreadable, so the drawing takes the first five. That is a reasonable design
// and a bad silence: the author wrote six and the sixth is on no slide. The
// number is published here so the measurement can say what was left out.
// drawableEntries is DrawableItems for one block. A comparison written as a
// matrix — an attribute per row, the sides across — is drawn row by row and is
// not capped at three; only the card shape is.
func drawableEntries(block Block) int {
	if block.Kind == BlockComparison && len(comparisonMatrix(block)) > 0 {
		return 0
	}
	return DrawableItems(block.Kind)
}

// A kind with no fixed cap answers zero: what it draws depends on the room it
// is given and on the shape of what it holds, and a number here would be a
// guess reported as a fact.
func DrawableItems(kind string) int {
	switch kind {
	case BlockComparison:
		return 3
	case BlockMeter, BlockSteps:
		return 5
	case BlockTimeline:
		return 6
	}
	return 0
}

// BlockMinimumLines is how much vertical room a component needs, measured in
// lines of the slot's own body text. A one-line caption slot can still hold a
// statement; a chart cannot.
func BlockMinimumLines(kind string) int {
	switch kind {
	case BlockQuote, BlockCallout, BlockHero:
		return 2
	case BlockColumns, BlockBars, BlockLine, BlockTimeline, BlockGrid:
		return 4
	}
	return 3
}

// BlockRoom is BlockMinimumLines for a component that has something to say
// above itself. A quote introduced by a line needs the line's room too, and a
// region that cannot give it is better filled with prose than with a statement
// drawn past its own frame.
func BlockRoom(block Block) int {
	lines := BlockMinimumLines(block.Kind)
	if statementLabel(block) != "" && (block.Kind == BlockQuote || block.Kind == BlockCallout) {
		lines++
	}
	return lines
}

// RenderBlock lays a component out inside a frame. It returns an empty
// component when the block carries nothing to draw, so callers can fall back to
// prose without a special case.
func RenderBlock(design Design, frame Frame, block Block) Component {
	if frame.Width <= 0 || frame.Height <= 0 {
		return Component{}
	}
	body := frame
	component := Component{Name: componentName(block.Kind), Frame: frame}
	statementBlock := block.Kind == BlockQuote || block.Kind == BlockCallout
	if statementBlock {
		// A pull quote is not given a bold heading over it — it is the statement.
		// But the words are the author's: a slide whose layout has no subtitle
		// region keeps its lead in the component's heading, and clearing it here
		// took that line off the slide without telling anybody. It is drawn the
		// way a label is drawn, above the statement it introduces.
		if label := statementLabel(block); label != "" {
			height := lineHeightFor(design.Small) * min(cellLines(label, design.Small, body.Width), 2)
			component.Primitives = append(component.Primitives, text(
				Frame{X: body.X, Y: body.Y, Width: body.Width, Height: height},
				line(label), textOptions{Size: design.Small, Color: design.InkMuted, Bold: true, Font: design.Minor, Wrap: true}))
			body.Y += height + design.Unit/2
			body.Height -= height + design.Unit/2
		}
		block.Heading = ""
	}
	if heading := strings.TrimSpace(block.Heading); heading != "" {
		// A heading that wraps needs the room it wraps into. Reserving one line for
		// it and drawing two puts the second line on top of the component.
		height := lineHeightFor(design.Heading) * min(cellLines(heading, design.Heading, body.Width), 3)
		component.Primitives = append(component.Primitives, text(
			Frame{X: body.X, Y: body.Y, Width: body.Width, Height: height},
			line(heading), textOptions{Size: design.Heading, Color: design.InkPrimary, Bold: true, Font: design.Major, Wrap: true}))
		body.Y += height + design.Unit
		body.Height -= height + design.Unit
	}
	if caption := strings.TrimSpace(block.Caption); caption != "" && !statementBlock {
		// A caption labels what follows, so it sits above it. Pinned to the bottom
		// of a frame that its content does not fill, it reads as a stray line.
		height := lineHeightFor(design.Small) * min(cellLines(caption, design.Small, body.Width), 2)
		component.Primitives = append(component.Primitives, text(
			Frame{X: body.X, Y: body.Y, Width: body.Width, Height: height},
			line(caption), textOptions{Size: design.Small, Color: design.InkMuted, Font: design.Minor, Wrap: true}))
		body.Y += height + design.Unit/2
		body.Height -= height + design.Unit/2
	}
	if body.Height <= design.Unit {
		return Component{}
	}

	// Everything after this point is the component's own drawing; the heading and
	// caption above it are shapes either way.
	component.BodyFrom = len(component.Primitives)
	var primitives []Primitive
	switch block.Kind {
	case BlockKPI:
		primitives = design.layoutKPI(body, block)
	case BlockHero:
		primitives = design.layoutHero(body, block)
	case BlockSteps:
		primitives = design.layoutSteps(body, block)
	case BlockTimeline:
		primitives = design.layoutTimeline(body, block)
	case BlockComparison:
		primitives = design.layoutComparison(body, block)
	case BlockColumns:
		primitives = design.layoutColumns(body, block)
		component.Chart = design.chartPart(body, block)
	case BlockBars:
		primitives = design.layoutBars(body, block)
		component.Chart = design.chartPart(body, block)
	case BlockLine:
		primitives = design.layoutLine(body, block)
		component.Chart = design.chartPart(body, block)
	case BlockShare:
		primitives = design.layoutShare(body, block)
	case BlockMeter:
		primitives = design.layoutMeter(body, block)
	case BlockTable:
		var drawnRows int
		primitives, drawnRows = design.layoutTable(body, block)
		component.RowsDrawn = drawnRows
		component.Table = design.tablePart(body, block, drawnRows)
	case BlockQuote:
		primitives = design.layoutQuote(body, block)
	case BlockCallout:
		primitives = design.layoutCallout(body, block)
	case BlockGrid:
		primitives = design.layoutGrid(body, block)
	default:
		return Component{}
	}
	if len(primitives) == 0 {
		return Component{}
	}
	component.Primitives = append(component.Primitives, primitives...)
	return component
}

// statementLabel is what a quote or callout is introduced by: the lead the
// slide had nowhere else to keep, and the word the author labelled it with.
func statementLabel(block Block) string {
	heading := strings.TrimSpace(block.Heading)
	caption := strings.TrimSpace(block.Caption)
	switch {
	case heading == "":
		return caption
	case caption == "" || strings.EqualFold(heading, caption):
		return heading
	default:
		return caption + " · " + heading
	}
}

func componentName(kind string) string {
	names := map[string]string{
		BlockKPI: "KPI row", BlockHero: "Hero figure", BlockSteps: "Process steps",
		BlockTimeline: "Timeline", BlockComparison: "Comparison", BlockColumns: "Column chart",
		BlockBars: "Bar chart", BlockLine: "Line chart", BlockShare: "Share bar",
		BlockMeter: "Meter", BlockTable: "Table", BlockQuote: "Pull quote", BlockCallout: "Callout",
		BlockGrid: "Grid",
	}
	if name, ok := names[kind]; ok {
		return name
	}
	return "Component"
}

// statement returns the single sentence a quote or callout is built around.
func (b Block) statement() string {
	for _, candidate := range []string{b.Text, b.Caption, b.Heading} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	for _, item := range b.items() {
		if trimmed := strings.TrimSpace(item.Label + " " + item.Value); strings.TrimSpace(trimmed) != "" {
			return strings.TrimSpace(trimmed)
		}
	}
	return ""
}

// items returns the block's entries, capped and with empty ones dropped.
func (b Block) items() []Item {
	result := make([]Item, 0, len(b.Items))
	for _, item := range b.Items {
		if strings.TrimSpace(item.Label) == "" && strings.TrimSpace(item.Value) == "" &&
			item.Number == nil && len(item.Bullets) == 0 {
			continue
		}
		result = append(result, item)
		if len(result) == maximumBlockItems {
			break
		}
	}
	return result
}

// --- figures ----------------------------------------------------------------

// layoutKPI draws a row of stat tiles. A handful of headline numbers is a KPI
// row, never a grouped bar chart.
func (d Design) layoutKPI(frame Frame, block Block) []Primitive {
	items := block.items()
	if len(items) == 0 {
		return nil
	}
	// Five figures on one line are five slivers, so a long row folds. Two rows of
	// three is how a person would set six figures — and dropping the ones that
	// did not fit, which is what a single row did, throws away numbers the author
	// went and found.
	columns, rows := len(items), 1
	if len(items) > 4 {
		columns, rows = (len(items)+1)/2, 2
	}
	valueSize := d.Display
	if columns > 2 {
		valueSize = d.Title
	}
	// A label as long as "목표 가용성" wraps inside a narrow card, and a card that
	// reserved one line for it drew the second line straight through the number.
	// The tile is sized for the label it actually has.
	cardWidth := (frame.Width - d.Unit*2*(columns-1)) / max(columns, 1)
	labelLines := 1
	for _, item := range items {
		labelLines = max(labelLines, min(cellLines(item.Label, d.Small, cardWidth-d.Unit*4), 2))
	}
	// A figure wider than its card was drawn straight across the card beside it:
	// "1억 5천만 원" ran under the next tile's background and the room read
	// "1억 5천". The number gives way to the card it is in, down to the size of
	// the label but no further — smaller than that and it stops being the thing
	// the slide is about.
	// The floor is the label's own size: a figure smaller than the words around
	// it stops being the thing the slide is about.
	if inner := cardWidth - d.Unit*4; inner > 0 {
		values := make([]string, 0, len(items))
		for _, item := range items {
			values = append(values, item.Display(block.Unit))
		}
		valueSize = fitToWidth(valueSize, inner, d.Small, values...)
	}
	tileHeightFor := func(size int) int {
		height := d.Unit*4 + lineHeightFor(d.Small)*labelLines + d.Unit/2 + lineHeightFor(size)
		if hasDetail(items) {
			height += d.Unit/2 + lineHeightFor(d.Small)
		}
		return height
	}
	// A tile is as tall as what it holds; stretching it to the placeholder
	// leaves a bank of empty boxes down the slide.
	gap := d.Unit * 2
	available := frame.Height
	if rows > 1 {
		available = (frame.Height - gap) / rows
	}
	tileHeight := tileHeightFor(valueSize)
	// If the room will not take the taller card, the number gives way rather than
	// the label: a wrapped label with a smaller figure still reads, and a figure
	// with a label through it does not.
	if tileHeight > available {
		if labelLines > 1 && valueSize > d.Heading {
			valueSize = d.Heading
			tileHeight = tileHeightFor(valueSize)
		}
		if tileHeight > available {
			// The tile is cut to the room, so what is in it is cut to the tile:
			// clamping the box alone drew the label, the figure and the note
			// straight out of the bottom of the region.
			tileHeight = available
			for valueSize > d.Small && tileHeightFor(valueSize) > tileHeight {
				valueSize -= 100
			}
			if labelLines > 1 && tileHeightFor(valueSize) > tileHeight {
				labelLines = 1
			}
		}
	}
	// A note under the figure is the first thing to go when the room will not
	// hold all three: the label says what it is and the figure is the point.
	showDetail := tileHeightFor(valueSize) <= tileHeight
	var primitives []Primitive
	for row := 0; row < rows; row++ {
		first := row * columns
		if first >= len(items) {
			break
		}
		band := Frame{X: frame.X, Y: frame.Y + row*(tileHeight+gap), Width: frame.Width, Height: tileHeight}
		tiles := band.Columns(columns, gap)
		for offset, tile := range tiles {
			index := first + offset
			if index >= len(items) {
				break
			}
			item := items[index]
			inner := tile.Inset(d.Unit * 2)
			primitives = append(primitives, rounded(tile, d.SurfaceRaised, d.Unit))
			primitives = append(primitives, filled(Frame{X: tile.X, Y: tile.Y, Width: d.Unit / 2, Height: tile.Height}, d.Series(index)))
			cursor := inner.Y
			labelHeight := lineHeightFor(d.Small) * labelLines
			primitives = append(primitives, text(
				Frame{X: inner.X, Y: cursor, Width: inner.Width, Height: labelHeight},
				line(item.Label), textOptions{Size: d.Small, Color: d.InkMuted, Font: d.Minor, Wrap: true}))
			cursor += labelHeight + d.Unit/2

			// The figure is drawn in what is left of the tile. A tile cut down to
			// a short region kept laying out from the top at full size, and the
			// number was drawn below the tile it belongs to.
			valueHeight := min(lineHeightFor(valueSize), max(inner.Bottom()-cursor, 0))
			if valueHeight <= 0 {
				continue
			}
			// A figure gives way to its card down to the size of its label and no
			// further; what will still not fit is cut rather than painted across
			// the tile beside it. A brief that answers "인력" with a sentence —
			// "12 staff redeployed" — is the case: it is not a figure, and the
			// measurement says what was cut.
			shown := min(valueSize, sizeForLine(valueHeight))
			primitives = append(primitives, text(
				Frame{X: inner.X, Y: cursor, Width: inner.Width, Height: valueHeight},
				line(cutToWidth(PlainText(item.Display(block.Unit)), shown, inner.Width)),
				textOptions{Size: shown, Color: d.InkPrimary, Bold: true, Font: d.Major}))
			cursor += valueHeight

			if detail := strings.TrimSpace(item.Delta + " " + item.Detail); showDetail && strings.TrimSpace(detail) != "" {
				color := d.InkMuted
				switch strings.ToLower(item.Trend) {
				case "up", "good", "positive":
					color = d.Positive
				case "down", "bad", "negative":
					color = d.Negative
				}
				primitives = append(primitives, text(
					Frame{X: inner.X, Y: cursor + d.Unit/2, Width: inner.Width, Height: lineHeightFor(d.Small)},
					line(strings.TrimSpace(detail)), textOptions{Size: d.Small, Color: color, Font: d.Minor, Wrap: true}))
			}
		}
	}
	return primitives
}

func hasDetail(items []Item) bool {
	for _, item := range items {
		if strings.TrimSpace(item.Delta+item.Detail) != "" {
			return true
		}
	}
	return false
}

// fitToHeight is the largest size at or below the one asked for at which the
// text wraps into the room it has, never smaller than floor.
//
// A component's own text was bounded by a box and not by the words in it: a
// detail that wrapped to more lines than the box holds was drawn below the
// component, outside the region the template gave it and over whatever was
// under it.
func fitToHeight(size, width, height, floor int, lines ...string) int {
	if width <= 0 || height <= 0 || size <= floor {
		return size
	}
	fits := func(at int) bool {
		used := 0
		for _, line := range lines {
			used += cellLines(line, at, width) * lineHeightFor(at)
		}
		return used <= height
	}
	for size > floor && !fits(size) {
		size -= 100
	}
	if size < floor {
		size = floor
	}
	return size
}

// roomForRunes is how many characters of a line fit in a height, at a size.
// It is the count truncate needs, not a width: what does not fit is a line at
// the bottom rather than a word at the end.
func roomForRunes(value string, size, width, height int) int {
	line := lineHeightFor(size)
	if line <= 0 || width <= 0 {
		return utf8.RuneCountInString(value)
	}
	rows := max(height/line, 1)
	perRow := max(int(float64(width)/(float64(size)/100*EMUPerPoint)/0.6), 1)
	return max(rows*perRow, 4)
}

// fitToWidth is the largest size at or below the one asked for at which every
// line fits the width it is given, never smaller than floor.
//
// A component draws its own text and nothing was checking that the text fits
// sideways: a figure wider than its box is painted over whatever is beside it,
// and the room reads half a number. Measuring found it in the hero, the meter,
// the timeline and the share bar as well as the card row it was first seen in.
func fitToWidth(size, width, floor int, lines ...string) int {
	if width <= 0 || size <= floor {
		return size
	}
	widest := func(at int) int {
		found := 0
		for _, line := range lines {
			found = max(found, textWidth(line, at))
		}
		return found
	}
	for widest(size) > width && size > floor {
		// Scaling by the ratio lands a hair over on the last step, since a glyph
		// is not exactly proportional to its point size; the step down keeps it
		// moving until it fits.
		size = max(min(size*width/widest(size), size-25), floor)
	}
	return size
}

// layoutHero draws the single number a slide leads with, in the body face
// rather than a display face, with its label beneath.
func (d Design) layoutHero(frame Frame, block Block) []Primitive {
	items := block.items()
	if len(items) == 0 {
		return nil
	}
	item := items[0]
	value := item.Display(block.Unit)
	if value == "" {
		return nil
	}
	size := d.Display * 2
	if size > 9600 {
		size = 9600
	}
	size = fitToWidth(size, frame.Width, d.Heading, value)
	valueHeight := lineHeightFor(size)
	if valueHeight > frame.Height*2/3 {
		size = d.Display
		valueHeight = lineHeightFor(size)
	}
	primitives := []Primitive{
		text(Frame{X: frame.X, Y: frame.Y, Width: frame.Width, Height: valueHeight},
			line(value), textOptions{Size: size, Color: d.Accent, Bold: true, Font: d.Minor}),
	}
	cursor := frame.Y + valueHeight + d.Unit
	if label := strings.TrimSpace(item.Label); label != "" {
		primitives = append(primitives, text(
			Frame{X: frame.X, Y: cursor, Width: frame.Width, Height: lineHeightFor(d.Heading) * 2},
			line(label), textOptions{Size: d.Heading, Color: d.InkPrimary, Font: d.Minor, Wrap: true}))
		cursor += lineHeightFor(d.Heading) + d.Unit
	}
	if detail := strings.TrimSpace(item.Detail); detail != "" {
		primitives = append(primitives, text(
			Frame{X: frame.X, Y: cursor, Width: frame.Width, Height: lineHeightFor(d.Body) * 2},
			line(detail), textOptions{Size: d.Body, Color: d.InkMuted, Font: d.Minor, Wrap: true}))
	}
	return primitives
}

// layoutMeter draws a single ratio against a limit: the fill carries the value
// and the unfilled track is a lighter step of the same ramp.
func (d Design) layoutMeter(frame Frame, block Block) []Primitive {
	items := block.items()
	if len(items) == 0 {
		return nil
	}
	if len(items) > 5 {
		items = items[:5]
	}
	var primitives []Primitive
	rowHeight := lineHeightFor(d.Body) + lineHeightFor(d.Small) + d.Unit
	gap := d.Unit
	if total := rowHeight*len(items) + gap*(len(items)-1); total < frame.Height {
		gap = (frame.Height - rowHeight*len(items)) / max(len(items)-1, 1)
		if gap > d.Unit*3 {
			gap = d.Unit * 3
		}
	}
	cursor := frame.Y
	trackHeight := d.Unit
	for _, item := range items {
		ratio := item.number() / 100
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		labelHeight := lineHeightFor(d.Body)
		primitives = append(primitives,
			text(Frame{X: frame.X, Y: cursor, Width: frame.Width * 3 / 4, Height: labelHeight},
				line(item.Label), textOptions{Size: d.Body, Color: d.InkPrimary, Font: d.Minor}),
			text(Frame{X: frame.X + frame.Width*3/4, Y: cursor, Width: frame.Width / 4, Height: labelHeight},
				line(item.Display(block.Unit)), textOptions{Size: d.Body, Color: d.InkSecondary, Bold: true, Align: "r", Font: d.Minor}))
		trackY := cursor + labelHeight + d.Unit/2
		primitives = append(primitives, rounded(Frame{X: frame.X, Y: trackY, Width: frame.Width, Height: trackHeight}, d.Track(), trackHeight/2))
		if width := int(float64(frame.Width) * ratio); width > trackHeight {
			primitives = append(primitives, rounded(Frame{X: frame.X, Y: trackY, Width: width, Height: trackHeight}, d.Accent, trackHeight/2))
		}
		cursor = trackY + trackHeight + gap
	}
	return primitives
}

// --- narrative components ---------------------------------------------------

// layoutSteps draws a numbered process. Steps read left to right with a
// connector, so the order is part of the picture.
func (d Design) layoutSteps(frame Frame, block Block) []Primitive {
	items := block.items()
	if len(items) < 2 {
		return nil
	}
	if len(items) > 5 {
		items = items[:5]
	}
	var primitives []Primitive
	badge := lineHeightFor(d.Heading) + d.Unit
	titleHeight := lineHeightFor(d.Body) * 2
	// The component is drawn into the room the template gave it. A short body
	// region — the editorial layouts have one — held the badge and two lines of
	// title and nothing else, and the rest was drawn below the region, over
	// whatever was under it.
	if badge+d.Unit*2+titleHeight > frame.Height {
		titleHeight = lineHeightFor(d.Body)
	}
	if badge+d.Unit*2+titleHeight > frame.Height {
		badge = max(frame.Height-d.Unit*2-titleHeight, d.Unit)
	}
	// And if the region is shorter than that, the title takes what is left of it
	// rather than the room it wanted, at the size that room can hold.
	titleHeight = min(titleHeight, max(frame.Height-badge-d.Unit*2, 0))
	titleSize := min(d.Body, sizeForLine(titleHeight))
	columns := frame.Columns(len(items), d.Unit*2)
	// One connector behind the badges ties the steps together.
	connectorY := frame.Y + badge/2
	primitives = append(primitives, hairline(Frame{
		X: columns[0].X + badge/2, Y: connectorY,
		Width: columns[len(columns)-1].X + badge/2 - (columns[0].X + badge/2), Height: 9525 * 2}, d.Line))
	for index, column := range columns {
		item := items[index]
		fill := d.Accent
		ink := d.OnAccent
		if block.Emphasis > 0 && block.Emphasis != index+1 {
			fill, ink = d.Track(), d.InkSecondary
		}
		// The number is drawn inside the badge, so it is sized by the badge: a
		// badge squeezed into a short region held a numeral taller than itself.
		numberSize := min(d.Body, sizeForLine(badge))
		primitives = append(primitives,
			Primitive{Kind: shapeEllipse, Frame: Frame{X: column.X, Y: frame.Y, Width: badge, Height: badge}, Fill: fill},
			text(Frame{X: column.X, Y: frame.Y, Width: badge, Height: badge},
				line(strconv.Itoa(index+1)), textOptions{Size: numberSize, Color: ink, Bold: true, Align: "ctr", Anchor: "ctr", Font: d.Minor}))
		cursor := frame.Y + badge + d.Unit*2
		primitives = append(primitives, text(
			Frame{X: column.X, Y: cursor, Width: column.Width, Height: titleHeight},
			line(item.Label), textOptions{Size: titleSize, Color: d.InkPrimary, Bold: true, Font: d.Minor, Wrap: true}))
		cursor += titleHeight
		if detail := strings.TrimSpace(item.Detail + item.Value); detail != "" {
			said := strings.TrimSpace(item.Detail + " " + item.Value)
			room := frame.Bottom() - cursor
			if room < lineHeightFor(d.Micro) {
				// There is no room for a word of it. A component that cannot show
				// its details shows its steps, rather than spilling out of the
				// region to say them.
				continue
			}
			// The box was bounded by the frame and the words were not: a detail
			// that wraps to more lines than the room holds is drawn below the
			// component, over whatever is under it. It is set smaller until it
			// fits, and cut where it cannot.
			size := fitToHeight(d.Small, column.Width, room, d.Micro, said)
			if lines := cellLines(said, size, column.Width); lines*lineHeightFor(size) > room {
				said = truncate(said, roomForRunes(said, size, column.Width, room))
			}
			primitives = append(primitives, text(
				Frame{X: column.X, Y: cursor, Width: column.Width, Height: room},
				line(said),
				textOptions{Size: size, Color: d.InkMuted, Font: d.Minor, Wrap: true}))
		}
	}
	return primitives
}

// layoutTimeline draws milestones along a horizontal axis, alternating labels
// above and below so dates never collide.
func (d Design) layoutTimeline(frame Frame, block Block) []Primitive {
	items := block.items()
	if len(items) < 2 {
		return nil
	}
	if len(items) > 6 {
		items = items[:6]
	}
	var primitives []Primitive
	axisY := frame.Y + frame.Height/2
	primitives = append(primitives, hairline(Frame{X: frame.X, Y: axisY, Width: frame.Width, Height: 9525 * 2}, d.Line))
	step := frame.Width / max(len(items)-1, 1)
	if len(items) == 1 {
		step = 0
	}
	radius := d.Unit
	// The ring around a dot is part of the mark, so the ends are inset by the
	// full outer radius or the first and last markers would clip the frame.
	const ringWidth = 9525 * 2
	edge := radius + ringWidth
	labelWidth := step * 4 / 5
	if labelWidth <= 0 {
		labelWidth = frame.Width
	}
	for index, item := range items {
		centerX := frame.X + index*step
		if index == 0 {
			centerX = frame.X + edge
		}
		if index == len(items)-1 {
			centerX = frame.Right() - edge
		}
		fill := d.Accent
		if block.Emphasis > 0 && block.Emphasis != index+1 {
			fill = d.DeEmphasis
		}
		primitives = append(primitives, dot(Point{X: centerX, Y: axisY + 9525}, radius, fill, d.Surface, ringWidth)...)
		above := index%2 == 0
		lines := make([]Paragraph, 0, 2)
		if label := strings.TrimSpace(item.Label); label != "" {
			lines = append(lines, Paragraph{Text: label})
		}
		if detail := strings.TrimSpace(item.Detail); detail != "" {
			lines = append(lines, Paragraph{Text: detail})
		}
		// The date sits against the axis and the description beyond it, so a
		// milestone always reads outward from its own dot.
		dateHeight := lineHeightFor(d.Small)
		labelHeight := dateHeight * len(lines)
		align, labelFrame := "ctr", Frame{Width: labelWidth, Height: labelHeight}
		labelFrame.X = centerX - labelWidth/2
		switch index {
		case 0:
			align, labelFrame.X = "l", centerX-radius
		case len(items) - 1:
			align, labelFrame.X = "r", centerX+radius-labelWidth
		}
		dateFrame := Frame{X: labelFrame.X, Width: labelWidth, Height: dateHeight}
		if above {
			dateFrame.Y = axisY - d.Unit*2 - dateHeight
			labelFrame.Y = dateFrame.Y - labelHeight
		} else {
			dateFrame.Y = axisY + d.Unit*2
			labelFrame.Y = dateFrame.Bottom()
		}
		if value := strings.TrimSpace(item.Display(block.Unit)); value != "" {
			primitives = append(primitives, text(dateFrame, line(value), textOptions{
				Size: d.Small, Color: d.Accent, Bold: true, Align: align, Anchor: "ctr", Font: d.Minor}))
		}
		if len(lines) > 0 {
			anchor := "b"
			if !above {
				anchor = "t"
			}
			primitives = append(primitives, text(labelFrame, lines, textOptions{
				Size: d.Small, Color: d.InkSecondary, Align: align, Anchor: anchor, Font: d.Minor}))
		}
	}
	return primitives
}

// layoutComparison draws two or three cards side by side, each with a heading
// and its own points.
func (d Design) layoutComparison(frame Frame, block Block) []Primitive {
	// Two shapes arrive under one name. "A versus B", each with a headline and its
	// supporting points, is a set of cards. "For each attribute, the old way and
	// the new way" is a matrix, and drawing that as cards puts one card per
	// attribute with the two sides stacked inside it — which compares nothing.
	if rows := comparisonMatrix(block); len(rows) > 0 {
		return d.layoutComparisonMatrix(frame, block, rows)
	}
	items := block.items()
	if len(items) < 2 {
		return nil
	}
	if len(items) > 3 {
		items = items[:3]
	}
	// Cards are sized to what they hold. Stretching two short cards down a tall
	// region leaves two empty boxes, which reads as unfinished rather than airy.
	needed := 0
	columnWidth := frame.Width / max(len(items), 1)
	for _, item := range items {
		height := d.Unit*5 + lineHeightFor(d.Body)*2
		if value := strings.TrimSpace(item.Display(block.Unit)); value != "" {
			size, lines := d.comparisonValueType(value, columnWidth-d.Unit*4)
			height += lineHeightFor(size)*lines + d.Unit/2
		}
		points := len(item.Bullets)
		if points == 0 && strings.TrimSpace(item.Detail) != "" {
			points = 1
		}
		height += lineHeightFor(d.Small) * 2 * min(points, 4)
		needed = max(needed, height)
	}
	if needed > 0 && needed < frame.Height {
		frame.Height = needed
	}

	var primitives []Primitive
	for index, card := range frame.Columns(len(items), d.Unit*2) {
		item := items[index]
		primitives = append(primitives, rounded(card, d.SurfaceRaised, d.Unit))
		accent := d.Series(index)
		primitives = append(primitives, filled(Frame{X: card.X, Y: card.Y, Width: card.Width, Height: d.Unit / 2}, accent))
		inner := card.Inset(d.Unit * 2)
		cursor := inner.Y + d.Unit/2
		headingHeight := lineHeightFor(d.Body) * 2
		primitives = append(primitives, text(
			Frame{X: inner.X, Y: cursor, Width: inner.Width, Height: headingHeight},
			line(item.Label), textOptions{Size: d.Body, Color: d.InkPrimary, Bold: true, Font: d.Minor, Wrap: true}))
		cursor += headingHeight
		if value := strings.TrimSpace(item.Display(block.Unit)); value != "" {
			size, lines := d.comparisonValueType(value, inner.Width)
			height := lineHeightFor(size) * lines
			primitives = append(primitives, text(
				Frame{X: inner.X, Y: cursor, Width: inner.Width, Height: height},
				line(value), textOptions{Size: size, Color: accent, Bold: true, Font: d.Major, Wrap: true}))
			cursor += height + d.Unit/2
		}
		points := item.Bullets
		if len(points) == 0 && strings.TrimSpace(item.Detail) != "" {
			points = []string{item.Detail}
		}
		if len(points) > 4 {
			points = points[:4]
		}
		for _, point := range points {
			height := lineHeightFor(d.Small) * 2
			if cursor+height > inner.Bottom() {
				break
			}
			primitives = append(primitives,
				Primitive{Kind: shapeEllipse, Frame: Frame{X: inner.X, Y: cursor + lineHeightFor(d.Small)/3,
					Width: d.Unit / 2, Height: d.Unit / 2}, Fill: accent},
				text(Frame{X: inner.X + d.Unit, Y: cursor, Width: inner.Width - d.Unit, Height: height},
					line(point), textOptions{Size: d.Small, Color: d.InkSecondary, Font: d.Minor, Wrap: true}))
			cursor += height
		}
	}
	return primitives
}

// layoutQuote draws a pull quote: one statement, an accent rule and an
// attribution.
func (d Design) layoutQuote(frame Frame, block Block) []Primitive {
	statement := block.statement()
	if statement == "" {
		return nil
	}
	rule := Frame{X: frame.X, Y: frame.Y, Width: d.Unit / 2, Height: frame.Height}
	body := Frame{X: frame.X + d.Unit*3, Y: frame.Y, Width: frame.Width - d.Unit*3, Height: frame.Height}
	size := d.Title
	if len([]rune(statement)) > 90 {
		size = d.Heading
	}
	primitives := []Primitive{
		filled(rule, d.Accent),
		text(body, line(statement), textOptions{Size: size, Color: d.InkPrimary, Anchor: "ctr", Font: d.Major, Wrap: true}),
	}
	if attribution := strings.TrimSpace(block.Attribute); attribution != "" {
		primitives = append(primitives, text(
			Frame{X: body.X, Y: body.Bottom() - lineHeightFor(d.Small), Width: body.Width, Height: lineHeightFor(d.Small)},
			line("— "+attribution), textOptions{Size: d.Small, Color: d.InkMuted, Font: d.Minor}))
	}
	return primitives
}

// layoutCallout draws one statement on a raised surface with an accent edge.
func (d Design) layoutCallout(frame Frame, block Block) []Primitive {
	statement := block.statement()
	if statement == "" {
		return nil
	}
	height := frame.Height
	if wanted := lineHeightFor(d.Heading)*3 + d.Unit*4; wanted < height {
		height = wanted
	}
	box := Frame{X: frame.X, Y: frame.Y, Width: frame.Width, Height: height}
	return []Primitive{
		rounded(box, d.SurfaceRaised, d.Unit),
		filled(Frame{X: box.X, Y: box.Y, Width: d.Unit / 2, Height: box.Height}, d.Accent),
		text(box.Inset(d.Unit*2), line(statement),
			textOptions{Size: d.Heading, Color: d.InkPrimary, Anchor: "ctr", Font: d.Minor, Wrap: true}),
	}
}

// layoutTable draws a header row and body rows separated by hairlines. Past
// about seven classes a table is the right form, so this has no series cap.
// tableRhythm is how tall a table's header and rows are.
//
// Rows share the region, but only up to a point: three rows spread over half a
// slide read as three stranded lines rather than as a table, so a short table
// keeps its rows close and leaves the space below it empty — which is what a
// person setting the same table would do.
//
// The rule under the last row needs its own hairline of room. Dividing the frame
// without reserving it made the drawing loop's own guard drop that row.
func (d Design) tableRhythm(frame Frame, rows int) (headerHeight, rowHeight int) {
	const hairlineHeight = 9525
	headerHeight = lineHeightFor(d.Small) + d.Unit
	if rows <= 0 {
		return headerHeight, lineHeightFor(d.Body) + d.Unit
	}
	rowHeight = (frame.Height - headerHeight - hairlineHeight) / rows
	minimum := lineHeightFor(d.Body) + d.Unit
	maximum := lineHeightFor(d.Body)*5/2 + d.Unit
	return headerHeight, min(max(rowHeight, minimum), maximum)
}

// tablePart describes the same table as a table, for the exported file. The
// numbers here follow layoutTable so that what PowerPoint holds and what the
// preview draws are the same table.
func (d Design) tablePart(frame Frame, block Block, drawnRows int) *TablePart {
	if len(block.Columns) == 0 || len(block.Rows) == 0 {
		return nil
	}
	columns := block.Columns
	if len(columns) > tableColumnCap {
		columns = columns[:tableColumnCap]
	}
	rows := block.Rows
	if len(rows) > tableRowCap {
		rows = rows[:tableRowCap]
	}
	aligns := make([]string, len(columns))
	for index := range aligns {
		aligns[index] = "r"
		if index == 0 {
			aligns[index] = "l"
		}
	}
	if drawnRows > 0 && drawnRows < len(rows) {
		// What PowerPoint holds is what the preview draws: a row the design had
		// no room for is not quietly present in the file either.
		rows = rows[:drawnRows]
	}
	trimmed := make([][]string, 0, len(rows))
	for _, row := range rows {
		if len(row) > len(columns) {
			row = row[:len(columns)]
		}
		trimmed = append(trimmed, row)
	}
	headerHeight, rowHeight := d.tableRhythm(frame, len(trimmed))
	bodySize, drawn := d.tableCells(frame, columns, trimmed, rowHeight)
	return &TablePart{
		Frame: frame, Columns: columns, Rows: markedCells(trimmed, drawn), Aligns: aligns,
		HeaderHeight: headerHeight, RowHeight: rowHeight,
		HeaderSize: d.Small, BodySize: bodySize, Font: d.Minor,
		HeaderInk: d.InkMuted, LabelInk: d.InkPrimary, ValueInk: d.InkSecondary,
		Rule: d.Line, Hairline: mixColor(d.Surface, d.Line, 0.6),
	}
}

// tableBodySize is the size the whole table's cells are written at: the largest
// at which every cell wraps into the row it was given.
//
// A row is a fixed height — that rhythm is what makes a table read as a table —
// and a cell whose words wrap to one line more than the row holds was drawn
// past it, over the row underneath. A real deck's table of platforms and what
// each is used for did exactly that, and the measurement called it what it was:
// two lines of the table overlap.
//
// One size for the table rather than one per row: rows of different sizes are
// not a table any more.
func (d Design) tableBodySize(frame Frame, columns []string, rows [][]string, rowHeight int) int {
	size, _ := d.tableCells(frame, columns, rows, rowHeight)
	return size
}

// cutToWidth shortens a line until it is drawn inside the width it has. It is
// for text that is not wrapped: a heading, where the only other outcome is
// paint across whatever is beside it.
func cutToWidth(said string, size, width int) string {
	if width <= 0 || textWidth(said, size) <= width {
		return said
	}
	runes := []rune(said)
	low, high := 0, len(runes)
	for low < high {
		middle := (low + high + 1) / 2
		if textWidth(truncate(said, middle), size) <= width {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return truncate(said, max(low, 1))
}

// cutToRows shortens a line until it wraps into the room it has, measuring
// rather than estimating: a rule of thumb about how many characters fit in a
// width is out by a third on Korean, where every glyph is as wide as it is
// tall, and a cell cut by that rule still drew a line past its row.
func cutToRows(said string, size, width, room int) string {
	if width <= 0 || room <= 0 || cellLines(said, size, width)*lineHeightFor(size) <= room {
		return said
	}
	runes := []rune(said)
	low, high := 0, len(runes)
	for low < high {
		middle := (low + high + 1) / 2
		if candidate := truncate(said, middle); cellLines(candidate, size, width)*lineHeightFor(size) <= room {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return truncate(said, max(low, 1))
}

// tableCells is the size the table is written at and the cells as they will be
// drawn: set smaller until they fit their row, and cut where they cannot.
//
// A real deck's table had a whole paragraph in one cell — a week's work listed
// in a single box — and no size small enough to hold it exists. The rest of the
// product answers that by cutting the line and marking it, which the reader can
// see; drawing it over the row underneath is the one answer that hides it.
func (d Design) tableCells(frame Frame, columns []string, rows [][]string, rowHeight int) (int, [][]string) {
	size := d.Body
	room := rowHeight - d.Unit/2
	cells := frame.Columns(len(columns), d.Unit)
	for _, row := range rows {
		for index, value := range row {
			if index >= len(cells) || strings.TrimSpace(value) == "" {
				continue
			}
			if fitted := fitToHeight(size, cells[index].Width, room, d.Micro, PlainText(value)); fitted < size {
				size = fitted
			}
		}
	}
	drawn := make([][]string, 0, len(rows))
	for _, row := range rows {
		cut := append([]string{}, row...)
		for index, value := range cut {
			if index >= len(cells) || strings.TrimSpace(value) == "" {
				continue
			}
			cut[index] = cutToRows(PlainText(value), size, cells[index].Width, room)
		}
		drawn = append(drawn, cut)
	}
	return size, drawn
}

// markedCells puts the author's marks back on the cells the table drew.
//
// The preview draws words rather than markup — it has no runs to draw them
// with — so tableCells hands back plain text. The exported file can carry more
// than words: a cell written **1억 2천** is bold in PowerPoint and a cell
// written [근거](https://…) is a link someone can follow. So a cell the design
// drew whole is written from what the author typed, and a cell it had to cut
// keeps only its words: half a link is not a link.
func markedCells(source, drawn [][]string) [][]string {
	marked := make([][]string, len(drawn))
	for row := range drawn {
		marked[row] = append([]string{}, drawn[row]...)
		if row >= len(source) {
			continue
		}
		for index, value := range marked[row] {
			if index < len(source[row]) && PlainText(source[row][index]) == value {
				marked[row][index] = source[row][index]
			}
		}
	}
	return marked
}

// A table is a shape a reader takes in at a glance. Past these it is a
// spreadsheet on a wall, and the measurement reports what did not fit rather
// than letting it disappear.
const (
	tableColumnCap = 5
	tableRowCap    = 8
)

// layoutTable draws the table and says how many of its rows it drew: a row the
// design has no room for is on no slide, and the author has to be told which.
func (d Design) layoutTable(frame Frame, block Block) ([]Primitive, int) {
	if len(block.Columns) == 0 || len(block.Rows) == 0 {
		return nil, 0
	}
	columns := block.Columns
	if len(columns) > tableColumnCap {
		columns = columns[:tableColumnCap]
	}
	rows := block.Rows
	if len(rows) > tableRowCap {
		rows = rows[:tableRowCap]
	}
	var primitives []Primitive
	cells := frame.Columns(len(columns), d.Unit)
	headerHeight, rowHeight := d.tableRhythm(frame, len(rows))
	bodySize, drawnRows := d.tableCells(frame, columns, rows, rowHeight)
	// The drawing carries the marks too: the preview draws a cell's link in the
	// link colour and underlined, and the printed page turns it into one a
	// reader can follow.
	rows = markedCells(rows, drawnRows)
	const hairlineHeight = 9525
	headerSize := d.Small
	heads := make([]string, len(cells))
	for index, cell := range cells {
		heads[index] = PlainText(columns[index])
		if fitted := fitToWidth(headerSize, cell.Width, d.Micro, heads[index]); fitted < headerSize {
			headerSize = fitted
		}
	}
	for index, cell := range cells {
		// A heading is one line and is not wrapped, so a long one is painted
		// across the table and off the slide. A document's table whose header
		// cell held a paragraph drew two metres past the region it was given.
		// A heading drawn whole keeps what the author marked; one that had to be
		// cut keeps only its words.
		if cut := cutToWidth(heads[index], headerSize, cell.Width); cut == heads[index] {
			heads[index] = columns[index]
		} else {
			heads[index] = cut
		}
		align := "l"
		if index > 0 {
			align = "r"
		}
		primitives = append(primitives, text(
			Frame{X: cell.X, Y: frame.Y, Width: cell.Width, Height: headerHeight},
			line(heads[index]), textOptions{Size: headerSize, Color: d.InkMuted, Bold: true, Align: align, Anchor: "b", Font: d.Minor}))
	}
	cursor := frame.Y + headerHeight
	primitives = append(primitives, hairline(Frame{X: frame.X, Y: cursor, Width: frame.Width, Height: 9525}, d.Line))
	drawn := 0
	for _, row := range rows {
		if cursor+rowHeight+hairlineHeight > frame.Bottom() {
			break
		}
		drawn++
		for index, cell := range cells {
			if index >= len(row) {
				break
			}
			align := "l"
			color := d.InkSecondary
			if index == 0 {
				color = d.InkPrimary
			} else {
				align = "r"
			}
			primitives = append(primitives, text(
				Frame{X: cell.X, Y: cursor + d.Unit/2, Width: cell.Width, Height: rowHeight - d.Unit/2},
				line(row[index]), textOptions{Size: bodySize, Color: color, Align: align, Anchor: "ctr", Font: d.Minor, Wrap: true}))
		}
		cursor += rowHeight
		primitives = append(primitives, hairline(Frame{X: frame.X, Y: cursor, Width: frame.Width, Height: hairlineHeight}, mixColor(d.Surface, d.Line, 0.6)))
	}
	return primitives, drawn
}

// --- charts -----------------------------------------------------------------

// barThickness keeps a mark thin: it never fills its band, and the leftover is
// air. The cap scales with the plot so a three-bar chart does not turn into
// three slabs.
func (d Design) barThickness(band, plot int) int {
	thickness := band * 55 / 100
	if cap := plot / 10; thickness > cap && cap > 0 {
		thickness = cap
	}
	if thickness < d.Unit {
		thickness = min(d.Unit, band)
	}
	return thickness
}

// layoutColumns draws a column chart: one hue for magnitude, values on the
// caps, a hairline baseline and no gridlines.
func (d Design) layoutColumns(frame Frame, block Block) []Primitive {
	items := block.items()
	if len(items) == 0 {
		return nil
	}
	maximum := 0.0
	for _, item := range items {
		maximum = math.Max(maximum, math.Abs(item.number()))
	}
	if maximum == 0 {
		return nil
	}
	var primitives []Primitive
	labelHeight := lineHeightFor(d.Small) * 2
	valueHeight := lineHeightFor(d.Small)
	// The band under the baseline holds the category labels, so the plot is
	// sized to leave room for them rather than letting them fall off the frame.
	baselineY := frame.Bottom() - labelHeight - d.Unit/2
	plotHeight := baselineY - frame.Y - valueHeight - d.Unit
	if plotHeight <= 0 {
		return nil
	}
	bands := frame.Columns(len(items), 0)
	// A 2px surface gap does the separating between neighbours; no borders.
	thickness := d.barThickness(bands[0].Width, frame.Width)
	for index, band := range bands {
		item := items[index]
		height := int(float64(plotHeight) * math.Abs(item.number()) / maximum)
		if height < d.Unit/2 {
			height = d.Unit / 2
		}
		fill := d.Accent
		if block.Emphasis > 0 {
			// Emphasis: the one series that matters keeps the hue, the rest recede.
			fill = d.DeEmphasis
			if block.Emphasis == index+1 {
				fill = d.Accent
			}
		}
		barFrame := Frame{X: band.X + (band.Width-thickness)/2, Y: baselineY - height, Width: thickness, Height: height}
		primitives = append(primitives, bar(barFrame, fill, d.Unit/2, sideTop))
		if len(items) <= 6 || block.Emphasis == index+1 {
			primitives = append(primitives, text(
				Frame{X: band.X, Y: barFrame.Y - valueHeight, Width: band.Width, Height: valueHeight},
				line(item.Display(block.Unit)),
				textOptions{Size: d.Small, Color: d.InkPrimary, Bold: true, Align: "ctr", Anchor: "b", Font: d.Minor}))
		}
		primitives = append(primitives, text(
			Frame{X: band.X, Y: baselineY + d.Unit/2, Width: band.Width, Height: labelHeight},
			line(item.Label), textOptions{Size: d.Small, Color: d.InkMuted, Align: "ctr", Font: d.Minor, Wrap: true}))
	}
	primitives = append(primitives, hairline(Frame{X: frame.X, Y: baselineY, Width: frame.Width, Height: 9525}, d.Line))
	return primitives
}

// layoutBars draws horizontal bars, which is the right form when the category
// names are long.
func (d Design) layoutBars(frame Frame, block Block) []Primitive {
	items := block.items()
	if len(items) == 0 {
		return nil
	}
	maximum := 0.0
	for _, item := range items {
		maximum = math.Max(maximum, math.Abs(item.number()))
	}
	if maximum == 0 {
		return nil
	}
	labelWidth := frame.Width * 30 / 100
	valueWidth := frame.Width * 12 / 100
	plotWidth := frame.Width - labelWidth - valueWidth - d.Unit*2
	if plotWidth <= 0 {
		return nil
	}
	var primitives []Primitive
	bands := frame.Rows(len(items), 0)
	thickness := d.barThickness(bands[0].Height, frame.Height)
	for index, band := range bands {
		item := items[index]
		width := int(float64(plotWidth) * math.Abs(item.number()) / maximum)
		if width < d.Unit/2 {
			width = d.Unit / 2
		}
		fill := d.Accent
		if block.Emphasis > 0 {
			fill = d.DeEmphasis
			if block.Emphasis == index+1 {
				fill = d.Accent
			}
		}
		centerY := band.Y + band.Height/2
		primitives = append(primitives,
			text(Frame{X: band.X, Y: band.Y, Width: labelWidth, Height: band.Height},
				line(item.Label), textOptions{Size: d.Small, Color: d.InkSecondary, Anchor: "ctr", Font: d.Minor, Wrap: true}),
			bar(Frame{X: band.X + labelWidth + d.Unit, Y: centerY - thickness/2, Width: width, Height: thickness}, fill, d.Unit/2, sideRight),
			text(Frame{X: band.X + labelWidth + d.Unit*2 + plotWidth, Y: band.Y, Width: valueWidth, Height: band.Height},
				line(item.Display(block.Unit)),
				textOptions{Size: d.Small, Color: d.InkPrimary, Bold: true, Align: "r", Anchor: "ctr", Font: d.Minor}))
	}
	primitives = append(primitives, hairline(Frame{X: frame.X + labelWidth + d.Unit, Y: frame.Y,
		Width: 9525, Height: frame.Height}, d.Line))
	return primitives
}

// layoutLine draws a trend. Series are capped at the template's validated
// categorical order, a legend appears from two series, and only the endpoint is
// direct-labelled.
func (d Design) layoutLine(frame Frame, block Block) []Primitive {
	series := make([]Series, 0, len(block.Series))
	for _, candidate := range block.Series {
		if len(candidate.Points) >= 2 {
			series = append(series, candidate)
		}
	}
	if len(series) == 0 {
		return nil
	}
	if len(series) > d.SeriesCap() {
		series = series[:d.SeriesCap()]
	}
	minimum, maximum := series[0].Points[0], series[0].Points[0]
	longest := 0
	for _, candidate := range series {
		longest = max(longest, len(candidate.Points))
		for _, point := range candidate.Points {
			minimum = math.Min(minimum, point)
			maximum = math.Max(maximum, point)
		}
	}
	if maximum == minimum {
		maximum = minimum + 1
	}
	labelHeight := lineHeightFor(d.Small) * 2
	legendHeight := 0
	if len(series) > 1 {
		legendHeight = lineHeightFor(d.Small) + d.Unit
	}
	valueWidth := frame.Width * 14 / 100
	plot := Frame{X: frame.X, Y: frame.Y + legendHeight,
		Width: frame.Width - valueWidth, Height: frame.Height - labelHeight - legendHeight - d.Unit/2}
	if plot.Width <= 0 || plot.Height <= 0 {
		return nil
	}
	var primitives []Primitive
	if legendHeight > 0 {
		// The legend packs to its labels rather than to equal columns, so two
		// series do not sit half a slide apart.
		cursor := frame.X
		for index, candidate := range series {
			label := strings.TrimSpace(candidate.Name)
			if label == "" {
				label = fmt.Sprintf("계열 %d", index+1)
			}
			width := textWidth(label, d.Small) + d.Unit*4
			primitives = append(primitives,
				filled(Frame{X: cursor, Y: frame.Y + lineHeightFor(d.Small)/2, Width: d.Unit * 2, Height: 9525 * 2}, d.Series(index)),
				text(Frame{X: cursor + d.Unit*3, Y: frame.Y, Width: width, Height: lineHeightFor(d.Small)},
					line(label), textOptions{Size: d.Small, Color: d.InkSecondary, Font: d.Minor}))
			cursor += width + d.Unit*2
			if cursor > frame.Right() {
				break
			}
		}
	}
	primitives = append(primitives, hairline(Frame{X: plot.X, Y: plot.Bottom(), Width: plot.Width, Height: 9525}, d.Line))
	step := plot.Width / max(longest-1, 1)
	for index, candidate := range series {
		color := d.Series(index)
		points := make([]Point, 0, len(candidate.Points))
		for pointIndex, value := range candidate.Points {
			ratio := (value - minimum) / (maximum - minimum)
			points = append(points, Point{
				X: plot.X + pointIndex*step,
				Y: plot.Bottom() - int(ratio*float64(plot.Height)),
			})
		}
		primitives = append(primitives, polyline(points, color, 9525*2))
		last := points[len(points)-1]
		primitives = append(primitives, dot(last, d.Unit*3/4, color, d.Surface, 9525*2)...)
		// The end label lives in the gutter reserved for it, clamped so a tall
		// final value never rides off the top or bottom of the frame.
		labelFrame := Frame{X: last.X + d.Unit, Y: last.Y - lineHeightFor(d.Small)/2,
			Width: frame.Right() - (last.X + d.Unit), Height: lineHeightFor(d.Small)}
		if labelFrame.Y < frame.Y {
			labelFrame.Y = frame.Y
		}
		if labelFrame.Bottom() > frame.Bottom() {
			labelFrame.Y = frame.Bottom() - labelFrame.Height
		}
		primitives = append(primitives, text(labelFrame,
			line(formatNumber(candidate.Points[len(candidate.Points)-1])+block.Unit),
			textOptions{Size: d.Small, Color: d.InkPrimary, Bold: true, Anchor: "ctr", Font: d.Minor}))
	}
	labels := block.Labels
	if len(labels) > 0 {
		for index := 0; index < longest && index < len(labels); index++ {
			// Only the ends and the middle are labelled; a tick per point is noise.
			if index != 0 && index != longest-1 && index != longest/2 {
				continue
			}
			// Ends anchor to the plot edge so no tick escapes the chart area.
			labelFrame := Frame{X: plot.X + index*step - step/2, Y: plot.Bottom() + d.Unit/2, Width: step, Height: labelHeight}
			align := "ctr"
			switch index {
			case 0:
				align, labelFrame.X = "l", plot.X
			case longest - 1:
				align, labelFrame.X = "r", plot.Right()-step
			}
			primitives = append(primitives, text(labelFrame,
				line(labels[index]), textOptions{Size: d.Small, Color: d.InkMuted, Align: align, Font: d.Minor}))
		}
	}
	return primitives
}

// comparisonValueType picks the size a comparison card's headline is set at and
// how many lines it needs. A figure is set as a figure; a phrase is set as text,
// because display type at phrase length either overflows or shrinks to nothing.
// cellLines is how many lines a cell's text needs at a size, in a width given in
// EMU rather than em units.
func cellLines(value string, size, width int) int {
	if width <= 0 || size <= 0 {
		return 1
	}
	return wrappedLines(value, float64(width)/(float64(size)/100*EMUPerPoint))
}

// IsComparisonMatrix reports whether a comparison block is the attribute-matrix
// shape rather than the side-by-side card shape.
func IsComparisonMatrix(block Block) bool {
	return block.Kind == BlockComparison && len(comparisonMatrix(block)) > 0
}

// comparisonMatrix returns the rows to draw as an attribute matrix, or nil when
// the block is the card kind. Three fields a row means attribute plus two sides;
// more than three rows of two fields is a list of attributes as well, because
// nobody compares four alternatives side by side on one slide.
func comparisonMatrix(block Block) [][]string {
	source := block.Rows
	if len(source) == 0 {
		// A deck stored before rows were kept, or one built by an API caller that
		// fills items only: label, value and detail are the three columns.
		for _, item := range block.items() {
			row := []string{item.Label, item.Display(block.Unit), item.Detail}
			if strings.TrimSpace(item.Detail) == "" && len(item.Bullets) > 0 {
				row[2] = item.Bullets[0]
			}
			source = append(source, row)
		}
	}
	rows := make([][]string, 0, len(source))
	widest := 0
	for _, row := range source {
		trimmed := make([]string, 0, len(row))
		for _, cell := range row {
			trimmed = append(trimmed, strings.TrimSpace(cell))
		}
		for len(trimmed) > 0 && trimmed[len(trimmed)-1] == "" {
			trimmed = trimmed[:len(trimmed)-1]
		}
		if len(trimmed) == 0 {
			continue
		}
		widest = max(widest, len(trimmed))
		rows = append(rows, trimmed)
	}
	if len(rows) < 2 {
		return nil
	}
	// A first row that names the sides is a header, whatever follows it. Drawn as
	// cards, "현재 | 목표" becomes two cards headed 현재 and 목표 with the real
	// comparison crammed into the second one — which is what a model writing a
	// two-column table actually produced.
	if widest >= 3 || len(rows) > 3 || namesSides(rows[0]) {
		return rows
	}
	return nil
}

// genericColumnNames are the words a header row uses for its first column. A
// model reaches for one of these whenever it writes a comparison table.
var genericColumnNames = map[string]bool{
	"항목": true, "구분": true, "분류": true, "영역": true, "기준": true, "비교": true, "측면": true,
	"item": true, "category": true, "aspect": true, "criteria": true, "dimension": true, "area": true,
}

// comparisonSideNames are the words a comparison's columns are named after. A
// header of "현재 | 목표" is as short as the data under it, so length alone cannot
// tell them apart — but these words are never data.
var comparisonSideNames = map[string]bool{
	"현재": true, "현행": true, "기존": true, "종전": true, "이전": true, "과거": true, "전": true,
	"목표": true, "신규": true, "개선": true, "미래": true, "향후": true, "계획": true, "후": true,
	"as-is": true, "asis": true, "to-be": true, "tobe": true, "before": true, "after": true,
	"current": true, "target": true, "old": true, "new": true, "today": true, "future": true,
}

// namesSides reports whether every cell of a row is the name of a side rather
// than a value.
func namesSides(row []string) bool {
	named := 0
	for _, cell := range row {
		trimmed := strings.ToLower(strings.TrimSpace(cell))
		if trimmed == "" {
			continue
		}
		if !comparisonSideNames[trimmed] && !genericColumnNames[trimmed] {
			return false
		}
		named++
	}
	return named >= 2
}

// tabularHeader reports whether the first row names the columns rather than
// holding data. Two independent signals agree on this in practice: a generic
// first cell, and cells markedly shorter than the rows beneath them.
func tabularHeader(rows [][]string) bool {
	if len(rows) < 2 {
		return false
	}
	if genericColumnNames[strings.ToLower(strings.TrimSpace(rows[0][0]))] || namesSides(rows[0]) {
		return true
	}
	// Below three rows there is not enough underneath a header to measure it
	// against, so only the names above count.
	if len(rows) < 3 {
		return false
	}
	header, body, cells := 0, 0, 0
	for _, cell := range rows[0] {
		length := utf8.RuneCountInString(cell)
		if length > 14 {
			return false
		}
		header += length
	}
	for _, row := range rows[1:] {
		for _, cell := range row {
			body += utf8.RuneCountInString(cell)
			cells++
		}
	}
	if cells == 0 || len(rows[0]) == 0 {
		return false
	}
	return float64(header)/float64(len(rows[0])) < 0.6*float64(body)/float64(cells)
}

// layoutComparisonMatrix draws an attribute-wise comparison: the attribute on the
// left, each side in its own tinted column, one row per attribute. Every row is
// drawn or the type shrinks until they all fit — a comparison that quietly loses
// its last row is worse than a small one.
func (d Design) layoutComparisonMatrix(frame Frame, block Block, rows [][]string) []Primitive {
	columns := 0
	for _, row := range rows {
		columns = max(columns, len(row))
	}
	if columns > 4 {
		columns = 4
	}
	var header []string
	if tabularHeader(rows) {
		header, rows = rows[0], rows[1:]
	}
	if len(rows) == 0 || columns < 2 {
		return nil
	}

	// Two columns under a header of two names are two sides — "현재 | 목표" — with
	// no attribute naming each row. Three or more, or two without a header, put
	// the attribute in the first column and the sides after it.
	labelled := columns >= 3 || len(header) == 0
	gap := d.Unit
	sides := columns
	attributeWidth := 0
	if labelled {
		sides = columns - 1
		attributeWidth = frame.Width * 26 / 100
		if columns == 2 {
			attributeWidth = frame.Width * 34 / 100
		}
	}
	sideWidth := (frame.Width - attributeWidth - gap*max(sides-1, 0)) / max(sides, 1)
	if labelled {
		sideWidth = (frame.Width - attributeWidth - gap*sides) / max(sides, 1)
	}
	columnX := func(index int) int {
		if labelled {
			if index == 0 {
				return frame.X
			}
			return frame.X + attributeWidth + gap*index + sideWidth*(index-1)
		}
		return frame.X + (sideWidth+gap)*index
	}
	columnWidth := func(index int) int {
		if labelled && index == 0 {
			return attributeWidth
		}
		return sideWidth
	}
	// The accent that marks a side, by column: with an attribute column the first
	// side is series one, without one the first column is.
	accentFor := func(index int) string {
		if labelled {
			return d.Series(index - 1)
		}
		return d.Series(index)
	}
	firstSide := 0
	if labelled {
		firstSide = 1
	}

	headerHeight := 0
	if len(header) > 0 {
		// The rule under a column title needs its own room; drawn inside the title's
		// band it strikes through it.
		headerHeight = lineHeightFor(d.Small) + d.Unit
		for index := firstSide; index < columns && index < len(header); index++ {
			headerHeight = max(headerHeight,
				lineHeightFor(d.Small)*min(cellLines(header[index], d.Small, columnWidth(index)-d.Unit), 2)+d.Unit)
		}
	}
	// Pick the largest body size whose rows all fit. Wrapping is measured, not
	// assumed, so a long cell costs the height it really takes.
	size := d.Body
	var heights []int
	for {
		heights = heights[:0]
		total := headerHeight
		for _, row := range rows {
			lines := 1
			for index := 0; index < columns && index < len(row); index++ {
				lines = max(lines, cellLines(row[index], size, columnWidth(index)-d.Unit))
			}
			height := lineHeightFor(size)*lines + d.Unit
			heights = append(heights, height)
			total += height
		}
		if total <= frame.Height || size <= d.Small {
			// A short table hugging the top of a tall region reads as unfinished.
			// Spending the spare room on the rows themselves keeps it balanced,
			// within reason: a two-row table is not a two-band poster.
			if spare := frame.Height - total; spare > 0 && len(heights) > 0 {
				share := spare / len(heights)
				for index := range heights {
					if limit := heights[index] * 12 / 10; share > limit-heights[index] {
						heights[index] = limit
						continue
					}
					heights[index] += share
				}
			}
			break
		}
		size = d.Small
	}

	var primitives []Primitive
	cursor := frame.Y
	if len(header) > 0 {
		for index := firstSide; index < columns && index < len(header); index++ {
			band := Frame{X: columnX(index), Y: cursor, Width: columnWidth(index), Height: headerHeight - d.Unit/2}
			primitives = append(primitives,
				filled(Frame{X: band.X, Y: cursor + headerHeight - d.Unit/3, Width: band.Width, Height: d.Unit / 3}, accentFor(index)),
				text(band, line(header[index]), textOptions{Size: d.Small, Color: d.InkPrimary, Bold: true, Anchor: "b", Font: d.Minor, Wrap: true}))
		}
		if labelled && strings.TrimSpace(header[0]) != "" && !genericColumnNames[strings.ToLower(strings.TrimSpace(header[0]))] {
			primitives = append(primitives, text(
				Frame{X: columnX(0), Y: cursor, Width: columnWidth(0), Height: headerHeight - d.Unit/2},
				line(header[0]), textOptions{Size: d.Small, Color: d.InkMuted, Bold: true, Anchor: "b", Font: d.Minor, Wrap: true}))
		}
		cursor += headerHeight
	}
	for rowIndex, row := range rows {
		height := heights[rowIndex]
		if cursor+height > frame.Bottom()+d.Unit/2 {
			break
		}
		if rowIndex%2 == 1 {
			primitives = append(primitives, rounded(
				Frame{X: frame.X, Y: cursor, Width: frame.Width, Height: height}, d.SurfaceRaised, d.Unit/2))
		}
		for index := 0; index < columns && index < len(row); index++ {
			cell := Frame{X: columnX(index) + d.Unit/2, Y: cursor + d.Unit/2,
				Width: columnWidth(index) - d.Unit, Height: height - d.Unit}
			options := textOptions{Size: size, Color: d.InkSecondary, Anchor: "ctr", Font: d.Minor, Wrap: true}
			if index == 0 {
				// The first column carries the row: the attribute that names it, or,
				// with no attribute column, the side being moved away from.
				options.Color, options.Bold = d.InkPrimary, true
			}
			primitives = append(primitives, text(cell, line(row[index]), options))
		}
		cursor += height
	}
	return primitives
}

// comparisonValueType is the size a card's headline is set at, and the lines it
// takes at that size. Three lines is the most a card should carry, so a headline
// that would need a fourth is set smaller rather than being given three lines and
// drawn in four — which is how the fourth line ended up over the card's edge.
func (d Design) comparisonValueType(value string, width int) (size, lines int) {
	const maximumLines = 3
	size = d.Title
	if utf8.RuneCountInString(value) > 12 {
		size = d.Heading
	}
	if utf8.RuneCountInString(value) > 26 {
		size = d.Body
	}
	for {
		lines = cellLines(value, size, width)
		if lines <= maximumLines || size <= d.Small {
			break
		}
		if next := size * 85 / 100; next < d.Small {
			size = d.Small
		} else {
			size = next
		}
	}
	return size, min(max(lines, 1), maximumLines)
}

// layoutShare draws a 100% stacked bar for part-to-whole, with a legend. It
// replaces a pie: segments stay comparable and long names stay readable.
func (d Design) layoutShare(frame Frame, block Block) []Primitive {
	items := block.items()
	if len(items) < 2 {
		return nil
	}
	if len(items) > d.SeriesCap() {
		items = items[:d.SeriesCap()]
	}
	total := 0.0
	for _, item := range items {
		total += math.Abs(item.number())
	}
	if total == 0 {
		return nil
	}
	barHeight := d.Unit * 5
	if barHeight > frame.Height/2 {
		barHeight = frame.Height / 2
	}
	const surfaceGap = 9525 * 2
	var primitives []Primitive
	cursor := frame.X
	for index, item := range items {
		width := int(float64(frame.Width) * math.Abs(item.number()) / total)
		if index == len(items)-1 {
			width = frame.Right() - cursor
		}
		if width < surfaceGap*2 {
			width = surfaceGap * 2
		}
		segment := Frame{X: cursor, Y: frame.Y, Width: width - surfaceGap, Height: barHeight}
		primitives = append(primitives, filled(segment, d.Series(index)))
		// A label only goes inside a segment when it fits with padding.
		label := item.Display(block.Unit)
		if measureEm(label)*float64(d.Small)/100*float64(EMUPerPoint) < float64(segment.Width)-float64(d.Unit*2) {
			primitives = append(primitives, text(segment, line(label), textOptions{
				Size: d.Small, Color: readableInk(d.Series(index), d.Surface, d.InkPrimary),
				Bold: true, Align: "ctr", Anchor: "ctr", Font: d.Minor}))
		}
		cursor += width
	}
	legendY := frame.Y + barHeight + d.Unit*2
	legendRows := (len(items) + 2) / 3
	legendColumns := frame.Columns(min(len(items), 3), d.Unit*2)
	for index, item := range items {
		column := legendColumns[index%len(legendColumns)]
		row := index / len(legendColumns)
		y := legendY + row*(lineHeightFor(d.Small)+d.Unit)
		if y+lineHeightFor(d.Small) > frame.Bottom() {
			break
		}
		primitives = append(primitives,
			Primitive{Kind: shapeEllipse, Frame: Frame{X: column.X, Y: y + lineHeightFor(d.Small)/4,
				Width: d.Unit, Height: d.Unit}, Fill: d.Series(index)},
			text(Frame{X: column.X + d.Unit*2, Y: y, Width: column.Width - d.Unit*2, Height: lineHeightFor(d.Small)},
				line(item.Label+"  "+item.Display(block.Unit)),
				textOptions{Size: d.Small, Color: d.InkSecondary, Font: d.Minor}))
	}
	_ = legendRows
	return primitives
}

// --- helpers ----------------------------------------------------------------

// sizeForLine is the largest size whose line fits a height: the inverse of
// lineHeightFor, for the places that are given room and have to choose a size.
func sizeForLine(height int) int {
	if height <= 0 {
		return 100
	}
	return max(int(float64(height)/1.26/float64(EMUPerPoint)*100), 100)
}

func lineHeightFor(size int) int {
	return int(float64(size) / 100 * float64(EMUPerPoint) * 1.26)
}

// formatNumber renders a value the way a slide should read it: compact above a
// thousand, thousands-separated below, and never with trailing zeros.
func formatNumber(value float64) string {
	absolute := math.Abs(value)
	switch {
	case absolute >= 1_000_000_000:
		return trimZero(value/1_000_000_000, 1) + "B"
	case absolute >= 1_000_000:
		return trimZero(value/1_000_000, 1) + "M"
	case absolute >= 10_000:
		return trimZero(value/1_000, 1) + "K"
	case absolute >= 1_000:
		return withThousands(value)
	case absolute >= 100:
		return trimZero(value, 0)
	}
	return trimZero(value, 1)
}

func trimZero(value float64, decimals int) string {
	formatted := strconv.FormatFloat(value, 'f', decimals, 64)
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(formatted, "0")
		formatted = strings.TrimRight(formatted, ".")
	}
	return formatted
}

func withThousands(value float64) string {
	whole := strconv.FormatFloat(math.Trunc(math.Abs(value)), 'f', 0, 64)
	var parts []string
	for len(whole) > 3 {
		parts = append([]string{whole[len(whole)-3:]}, parts...)
		whole = whole[:len(whole)-3]
	}
	parts = append([]string{whole}, parts...)
	sign := ""
	if value < 0 {
		sign = "-"
	}
	return sign + strings.Join(parts, ",")
}

// textWidth is the rendered width of a string at a font size, in EMU.
func textWidth(value string, size int) int {
	return int(measureEm(value) * float64(size) / 100 * float64(EMUPerPoint))
}

// BlockKind resolves the name an author writes to a supported component, so
// deck source can say "bar" or "steps" without knowing the internal spelling.
func BlockKind(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "bullets", "list", "목록":
		return BlockBullets
	case "kpi", "kpis", "metrics", "지표":
		return BlockKPI
	case "hero", "figure", "big", "숫자":
		return BlockHero
	case "steps", "process", "단계", "절차":
		return BlockSteps
	case "timeline", "roadmap", "일정", "로드맵":
		return BlockTimeline
	case "comparison", "compare", "versus", "vs", "비교":
		return BlockComparison
	case "columnchart", "columns", "column", "bar", "bars", "세로막대":
		return BlockColumns
	case "barchart", "hbar", "hbars", "ranking", "가로막대":
		return BlockBars
	case "linechart", "line", "trend", "추이":
		return BlockLine
	case "sharebar", "share", "split", "비중":
		return BlockShare
	case "meter", "gauge", "progress", "달성률":
		return BlockMeter
	case "table", "표":
		return BlockTable
	case "grid", "격자", "matrix", "raci":
		return BlockGrid
	case "quote", "statement", "인용":
		return BlockQuote
	case "callout", "note", "highlight", "강조":
		return BlockCallout
	}
	return ""
}
