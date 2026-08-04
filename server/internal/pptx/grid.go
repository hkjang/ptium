package pptx

import (
	"fmt"
	"sort"
	"strings"
)

// Organisations have tables of their own: a RACI chart, a risk matrix, a
// readiness checklist. They are all grids — labelled columns, labelled rows, and
// a cell that is either a short value or a mark — so rather than adding a
// component per organisation, a grid is described as data and drawn by one
// renderer.
//
// A definition never names a colour. It names a role, which each template's own
// theme resolves, so the same RACI chart drawn into two templates comes out in
// two houses' colours.

// GridSpec describes a grid component: what its columns are and how its cell
// values are drawn.
type GridSpec struct {
	// Name is how deck source refers to the definition.
	Name string `json:"name"`
	// Title is shown above the grid when the source supplies no caption.
	Title string `json:"title,omitempty"`
	// Columns describe the header. The first column is the row label.
	Columns []GridColumn `json:"columns,omitempty"`
	// Values maps a cell's written value to how it is drawn. A value with no
	// entry is drawn as plain text.
	Values map[string]GridValue `json:"values,omitempty"`
	// Order lists the values in the order the legend should read them. A map has
	// none, and "R, A, C, I" is not alphabetical.
	Order []string `json:"order,omitempty"`
	// Zebra shades alternate rows, which helps a wide grid stay readable.
	Zebra bool `json:"zebra,omitempty"`
	// Legend lists the value meanings under the grid.
	Legend bool `json:"legend,omitempty"`
}

// GridColumn is one column of a grid.
type GridColumn struct {
	Label string `json:"label,omitempty"`
	// Weight is the column's share of the width, relative to the others. Zero
	// means one share.
	Weight float64 `json:"weight,omitempty"`
	// Align is "l", "ctr" or "r"; the default is centre for value columns and
	// left for the row label.
	Align string `json:"align,omitempty"`
}

// GridValue is how one cell value is drawn.
type GridValue struct {
	// Label replaces the written value, so a source can say "R" and the slide can
	// say "R" or "책임".
	Label string `json:"label,omitempty"`
	// Role is a colour role the template resolves: accent1..accent6, positive,
	// negative, muted or ink.
	Role string `json:"role,omitempty"`
	// Chip draws the value in a filled pill rather than as coloured text, which
	// is what makes a RACI chart scannable.
	Chip bool `json:"chip,omitempty"`
	// Meaning is shown in the legend.
	Meaning string `json:"meaning,omitempty"`
}

// BuiltinGrids are the definitions every deployment has. They exist so the
// feature is useful before anyone defines anything, and they are the worked
// examples a customer copies.
func BuiltinGrids() []GridSpec {
	return []GridSpec{
		{
			Name: "raci", Title: "담당 체계 (RACI)", Zebra: true, Legend: true,
			Order:   []string{"R", "A", "C", "I"},
			Columns: []GridColumn{{Label: "활동", Weight: 2.2, Align: "l"}},
			Values: map[string]GridValue{
				"R": {Label: "R", Role: "accent1", Chip: true, Meaning: "실행"},
				"A": {Label: "A", Role: "accent2", Chip: true, Meaning: "승인"},
				"C": {Label: "C", Role: "accent3", Chip: true, Meaning: "협의"},
				"I": {Label: "I", Role: "muted", Chip: true, Meaning: "통보"},
			},
		},
		{
			Name: "matrix", Title: "영향 · 가능성", Zebra: false,
			Order:   []string{"높음", "중간", "낮음"},
			Columns: []GridColumn{{Label: "", Weight: 1.4, Align: "l"}},
			Values: map[string]GridValue{
				"높음": {Label: "높음", Role: "negative", Chip: true},
				"중간": {Label: "중간", Role: "accent2", Chip: true},
				"낮음": {Label: "낮음", Role: "positive", Chip: true},
			},
		},
		{
			Name: "checklist", Title: "준비 상태", Zebra: true, Legend: true,
			Order:   []string{"완료", "진행", "미착수", "위험"},
			Columns: []GridColumn{{Label: "항목", Weight: 2.6, Align: "l"}},
			Values: map[string]GridValue{
				"완료":  {Label: "완료", Role: "positive", Chip: true, Meaning: "확인됨"},
				"진행":  {Label: "진행", Role: "accent2", Chip: true, Meaning: "진행 중"},
				"미착수": {Label: "미착수", Role: "muted", Chip: true, Meaning: "시작 전"},
				"위험":  {Label: "위험", Role: "negative", Chip: true, Meaning: "조치 필요"},
			},
		},
	}
}

// LookupBuiltinGrid finds a shipped definition by name.
func LookupBuiltinGrid(name string) (GridSpec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, spec := range BuiltinGrids() {
		if spec.Name == name {
			return spec, true
		}
	}
	return GridSpec{}, false
}

// roleColor resolves a definition's colour role against the template's theme.
func (d Design) roleColor(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "ink":
		return d.InkPrimary
	case "muted":
		return d.DeEmphasis
	case "positive":
		return d.Positive
	case "negative":
		return d.Negative
	}
	if strings.HasPrefix(role, "accent") {
		index := 0
		if _, err := fmt.Sscanf(role, "accent%d", &index); err == nil && index >= 1 {
			return d.Series(index - 1)
		}
	}
	return d.InkPrimary
}

// column returns the spec for a column, filling in defaults.
func (g GridSpec) column(index int) GridColumn {
	if index < len(g.Columns) {
		spec := g.Columns[index]
		if spec.Weight <= 0 {
			spec.Weight = 1
		}
		if spec.Align == "" {
			spec.Align = "ctr"
			if index == 0 {
				spec.Align = "l"
			}
		}
		return spec
	}
	return GridColumn{Weight: 1, Align: "ctr"}
}

// layoutGrid draws a grid: a header row, a labelled row per entry, and cells
// that are either a coloured chip or plain text.
func (d Design) layoutGrid(frame Frame, block Block) []Primitive {
	spec := block.Grid
	if spec == nil || len(block.Rows) == 0 {
		return nil
	}
	header := block.Columns
	rows := block.Rows
	if len(rows) > 10 {
		rows = rows[:10]
	}
	columns := len(header)
	for _, row := range rows {
		columns = max(columns, len(row))
	}
	if columns == 0 {
		return nil
	}
	if columns > 7 {
		columns = 7
	}

	// Widths follow the definition's weights, so a label column can be wider.
	weights := make([]float64, columns)
	total := 0.0
	for index := range weights {
		weights[index] = spec.column(index).Weight
		total += weights[index]
	}
	gap := d.Unit
	usable := frame.Width - gap*(columns-1)
	edges := make([]int, columns+1)
	cursor := frame.X
	for index := 0; index < columns; index++ {
		edges[index] = cursor
		cursor += int(float64(usable)*weights[index]/total) + gap
	}
	edges[columns] = frame.Right()

	headerHeight := 0
	if len(header) > 0 {
		headerHeight = lineHeightFor(d.Small) + d.Unit
	}
	legendHeight := 0
	if spec.Legend && len(spec.Values) > 0 {
		legendHeight = lineHeightFor(d.Small) + d.Unit
	}
	const hairlineHeight = 9525
	rowHeight := (frame.Height - headerHeight - legendHeight - hairlineHeight) / max(len(rows), 1)
	if minimum := lineHeightFor(d.Body) + d.Unit; rowHeight < minimum {
		rowHeight = minimum
	}

	var primitives []Primitive
	for index := 0; index < columns && index < len(header); index++ {
		column := spec.column(index)
		primitives = append(primitives, text(
			Frame{X: edges[index], Y: frame.Y, Width: edges[index+1] - edges[index] - gap, Height: headerHeight},
			line(header[index]), textOptions{Size: d.Small, Color: d.InkMuted, Bold: true,
				Align: column.Align, Anchor: "b", Font: d.Minor}))
	}
	top := frame.Y + headerHeight
	if headerHeight > 0 {
		primitives = append(primitives, hairline(Frame{X: frame.X, Y: top, Width: frame.Width, Height: hairlineHeight}, d.Line))
	}

	for rowIndex, row := range rows {
		rowTop := top + rowIndex*rowHeight
		if rowTop+rowHeight+hairlineHeight > frame.Bottom()-legendHeight {
			break
		}
		if spec.Zebra && rowIndex%2 == 1 {
			primitives = append(primitives, filled(
				Frame{X: frame.X, Y: rowTop, Width: frame.Width, Height: rowHeight}, d.SurfaceRaised))
		}
		for columnIndex := 0; columnIndex < columns && columnIndex < len(row); columnIndex++ {
			cell := strings.TrimSpace(row[columnIndex])
			if cell == "" {
				continue
			}
			column := spec.column(columnIndex)
			cellFrame := Frame{X: edges[columnIndex], Y: rowTop,
				Width: edges[columnIndex+1] - edges[columnIndex] - gap, Height: rowHeight}
			value, mapped := spec.Values[cell]
			switch {
			case mapped && value.Chip:
				primitives = append(primitives, d.gridChip(cellFrame, value, column.Align)...)
			case mapped:
				primitives = append(primitives, text(cellFrame.Inset(d.Unit/2),
					line(chipLabel(value, cell)), textOptions{Size: d.Body, Color: d.roleColor(value.Role),
						Bold: true, Align: column.Align, Anchor: "ctr", Font: d.Minor}))
			default:
				color := d.InkSecondary
				if columnIndex == 0 {
					color = d.InkPrimary
				}
				primitives = append(primitives, text(cellFrame.Inset(d.Unit/2),
					line(cell), textOptions{Size: d.Body, Color: color, Align: column.Align,
						Anchor: "ctr", Font: d.Minor, Wrap: true}))
			}
		}
		primitives = append(primitives, hairline(
			Frame{X: frame.X, Y: rowTop + rowHeight, Width: frame.Width, Height: hairlineHeight},
			mixColor(d.Surface, d.Line, 0.6)))
	}

	if legendHeight > 0 {
		primitives = append(primitives, d.gridLegend(
			Frame{X: frame.X, Y: frame.Bottom() - legendHeight, Width: frame.Width, Height: legendHeight}, spec)...)
	}
	return primitives
}

// gridChip draws a value as a filled pill, which is what makes a dense grid
// scannable at slide distance.
func (d Design) gridChip(cell Frame, value GridValue, align string) []Primitive {
	label := chipLabel(value, "")
	height := lineHeightFor(d.Small) + d.Unit/2
	width := textWidth(label, d.Small) + d.Unit*3
	if width > cell.Width {
		width = cell.Width
	}
	x := cell.X + (cell.Width-width)/2
	switch align {
	case "l":
		x = cell.X
	case "r":
		x = cell.Right() - width
	}
	frame := Frame{X: x, Y: cell.Y + (cell.Height-height)/2, Width: width, Height: height}
	fill := d.roleColor(value.Role)
	return []Primitive{
		rounded(frame, fill, height/2),
		text(frame, line(label), textOptions{Size: d.Small, Color: readableInk(fill, d.OnAccent, d.InkPrimary),
			Bold: true, Align: "ctr", Anchor: "ctr", Font: d.Minor}),
	}
}

// gridLegend explains the values under the grid, in the order the definition
// lists them.
func (d Design) gridLegend(frame Frame, spec *GridSpec) []Primitive {
	var primitives []Primitive
	cursor := frame.X
	for _, key := range spec.valueOrder() {
		value := spec.Values[key]
		meaning := strings.TrimSpace(value.Meaning)
		if meaning == "" {
			continue
		}
		label := chipLabel(value, key) + " " + meaning
		width := textWidth(label, d.Small) + d.Unit*4
		if cursor+width > frame.Right() {
			break
		}
		marker := Frame{X: cursor, Y: frame.Y + frame.Height/2 - d.Unit/2, Width: d.Unit, Height: d.Unit}
		primitives = append(primitives,
			rounded(marker, d.roleColor(value.Role), d.Unit/2),
			text(Frame{X: cursor + d.Unit*2, Y: frame.Y, Width: width, Height: frame.Height},
				line(label), textOptions{Size: d.Small, Color: d.InkMuted, Anchor: "ctr", Font: d.Minor}))
		cursor += width + d.Unit
	}
	return primitives
}

// valueOrder lists a definition's values deterministically: a legend that
// reorders itself between renders is a diff nobody asked for.
func (spec *GridSpec) valueOrder() []string {
	keys := make([]string, 0, len(spec.Values))
	seen := map[string]bool{}
	// The definition's own order first: a RACI legend reads R, A, C, I.
	for _, key := range spec.Order {
		if _, ok := spec.Values[key]; ok && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	remaining := make([]string, 0, len(spec.Values))
	for key := range spec.Values {
		if !seen[key] {
			remaining = append(remaining, key)
		}
	}
	// Whatever the definition did not order is alphabetical, because a map has
	// none and a legend that reorders itself between renders is a spurious diff.
	sort.Strings(remaining)
	return append(keys, remaining...)
}

func chipLabel(value GridValue, fallback string) string {
	if label := strings.TrimSpace(value.Label); label != "" {
		return label
	}
	return strings.TrimSpace(fallback)
}
