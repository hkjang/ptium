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
	// Text is the single statement a quote or callout carries.
	Text      string `json:"text,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Emphasis  int    `json:"emphasis,omitempty"` // 1-based item to highlight
	Attribute string `json:"attribute,omitempty"`
}

// maximumBlockItems caps a component so it stays readable at slide scale.
const maximumBlockItems = 8

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

// RenderBlock lays a component out inside a frame. It returns an empty
// component when the block carries nothing to draw, so callers can fall back to
// prose without a special case.
func RenderBlock(design Design, frame Frame, block Block) Component {
	if frame.Width <= 0 || frame.Height <= 0 {
		return Component{}
	}
	body := frame
	component := Component{Name: componentName(block.Kind), Frame: frame}
	if block.Kind == BlockQuote || block.Kind == BlockCallout {
		block.Heading = ""
	}
	if heading := strings.TrimSpace(block.Heading); heading != "" {
		height := lineHeightFor(design.Heading)
		component.Primitives = append(component.Primitives, text(
			Frame{X: body.X, Y: body.Y, Width: body.Width, Height: height},
			line(heading), textOptions{Size: design.Heading, Color: design.InkPrimary, Bold: true, Font: design.Major, Wrap: true}))
		body.Y += height + design.Unit
		body.Height -= height + design.Unit
	}
	statementBlock := block.Kind == BlockQuote || block.Kind == BlockCallout
	if caption := strings.TrimSpace(block.Caption); caption != "" && !statementBlock {
		// A caption labels what follows, so it sits above it. Pinned to the bottom
		// of a frame that its content does not fill, it reads as a stray line.
		height := lineHeightFor(design.Small)
		component.Primitives = append(component.Primitives, text(
			Frame{X: body.X, Y: body.Y, Width: body.Width, Height: height},
			line(caption), textOptions{Size: design.Small, Color: design.InkMuted, Font: design.Minor, Wrap: true}))
		body.Y += height + design.Unit/2
		body.Height -= height + design.Unit/2
	}
	if body.Height <= design.Unit {
		return Component{}
	}

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
	case BlockBars:
		primitives = design.layoutBars(body, block)
	case BlockLine:
		primitives = design.layoutLine(body, block)
	case BlockShare:
		primitives = design.layoutShare(body, block)
	case BlockMeter:
		primitives = design.layoutMeter(body, block)
	case BlockTable:
		primitives = design.layoutTable(body, block)
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
	if len(items) > 4 {
		items = items[:4]
	}
	valueSize := d.Display
	if len(items) > 2 {
		valueSize = d.Title
	}
	// A tile is as tall as what it holds; stretching it to the placeholder
	// leaves a bank of empty boxes down the slide.
	tileHeight := d.Unit*4 + lineHeightFor(d.Small) + d.Unit/2 + lineHeightFor(valueSize)
	if hasDetail(items) {
		tileHeight += d.Unit/2 + lineHeightFor(d.Small)
	}
	if tileHeight > frame.Height {
		tileHeight = frame.Height
	}
	row := Frame{X: frame.X, Y: frame.Y, Width: frame.Width, Height: tileHeight}
	var primitives []Primitive
	for index, tile := range row.Columns(len(items), d.Unit*2) {
		item := items[index]
		inner := tile.Inset(d.Unit * 2)
		primitives = append(primitives, rounded(tile, d.SurfaceRaised, d.Unit))
		primitives = append(primitives, filled(Frame{X: tile.X, Y: tile.Y, Width: d.Unit / 2, Height: tile.Height}, d.Series(index)))
		cursor := inner.Y
		labelHeight := lineHeightFor(d.Small)
		primitives = append(primitives, text(
			Frame{X: inner.X, Y: cursor, Width: inner.Width, Height: labelHeight},
			line(item.Label), textOptions{Size: d.Small, Color: d.InkMuted, Font: d.Minor, Wrap: true}))
		cursor += labelHeight + d.Unit/2

		valueHeight := lineHeightFor(valueSize)
		primitives = append(primitives, text(
			Frame{X: inner.X, Y: cursor, Width: inner.Width, Height: valueHeight},
			line(item.Display(block.Unit)), textOptions{Size: valueSize, Color: d.InkPrimary, Bold: true, Font: d.Major}))
		cursor += valueHeight

		if detail := strings.TrimSpace(item.Delta + " " + item.Detail); strings.TrimSpace(detail) != "" {
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
		primitives = append(primitives,
			Primitive{Kind: shapeEllipse, Frame: Frame{X: column.X, Y: frame.Y, Width: badge, Height: badge}, Fill: fill},
			text(Frame{X: column.X, Y: frame.Y, Width: badge, Height: badge},
				line(strconv.Itoa(index+1)), textOptions{Size: d.Body, Color: ink, Bold: true, Align: "ctr", Anchor: "ctr", Font: d.Minor}))
		cursor := frame.Y + badge + d.Unit*2
		titleHeight := lineHeightFor(d.Body) * 2
		primitives = append(primitives, text(
			Frame{X: column.X, Y: cursor, Width: column.Width, Height: titleHeight},
			line(item.Label), textOptions{Size: d.Body, Color: d.InkPrimary, Bold: true, Font: d.Minor, Wrap: true}))
		cursor += titleHeight
		if detail := strings.TrimSpace(item.Detail + item.Value); detail != "" {
			primitives = append(primitives, text(
				Frame{X: column.X, Y: cursor, Width: column.Width, Height: frame.Bottom() - cursor},
				line(strings.TrimSpace(item.Detail+" "+item.Value)),
				textOptions{Size: d.Small, Color: d.InkMuted, Font: d.Minor, Wrap: true}))
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
func (d Design) layoutTable(frame Frame, block Block) []Primitive {
	if len(block.Columns) == 0 || len(block.Rows) == 0 {
		return nil
	}
	columns := block.Columns
	if len(columns) > 5 {
		columns = columns[:5]
	}
	rows := block.Rows
	if len(rows) > 8 {
		rows = rows[:8]
	}
	var primitives []Primitive
	cells := frame.Columns(len(columns), d.Unit)
	headerHeight := lineHeightFor(d.Small) + d.Unit
	// The rule under the last row needs its own hairline of room. Dividing the
	// frame without reserving it made the loop's own guard drop that row.
	const hairlineHeight = 9525
	rowHeight := (frame.Height - headerHeight - hairlineHeight) / max(len(rows), 1)
	if minimum := lineHeightFor(d.Body) + d.Unit; rowHeight < minimum {
		rowHeight = minimum
	}
	for index, cell := range cells {
		align := "l"
		if index > 0 {
			align = "r"
		}
		primitives = append(primitives, text(
			Frame{X: cell.X, Y: frame.Y, Width: cell.Width, Height: headerHeight},
			line(columns[index]), textOptions{Size: d.Small, Color: d.InkMuted, Bold: true, Align: align, Anchor: "b", Font: d.Minor}))
	}
	cursor := frame.Y + headerHeight
	primitives = append(primitives, hairline(Frame{X: frame.X, Y: cursor, Width: frame.Width, Height: 9525}, d.Line))
	for _, row := range rows {
		if cursor+rowHeight+hairlineHeight > frame.Bottom() {
			break
		}
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
				line(row[index]), textOptions{Size: d.Body, Color: color, Align: align, Anchor: "ctr", Font: d.Minor, Wrap: true}))
		}
		cursor += rowHeight
		primitives = append(primitives, hairline(Frame{X: frame.X, Y: cursor, Width: frame.Width, Height: hairlineHeight}, mixColor(d.Surface, d.Line, 0.6)))
	}
	return primitives
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
func (d Design) comparisonValueType(value string, width int) (size, lines int) {
	size = d.Title
	if utf8.RuneCountInString(value) > 12 {
		size = d.Heading
	}
	if utf8.RuneCountInString(value) > 26 {
		size = d.Body
	}
	perLine := float64(width) / (float64(size) / 100 * EMUPerPoint)
	return size, min(max(1, int(measureEm(value)/math.Max(perLine, 1))+1), 3)
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
