package pptx

import (
	"strconv"
	"strings"
)

// A table someone cannot type into is a picture of a table.
//
// Ptium draws every component itself, which is what keeps the preview and the
// exported file identical. For a table that trade is wrong: the first thing
// anyone does with one is add a row, fix a number, or paste it into a document —
// and none of that is possible with fifteen shapes. So a table is exported as a
// real PowerPoint table, styled to look exactly like the drawn one: no boxes, a
// rule under the header, hairlines between rows, labels left and figures right.

// drawingML writes the table as a graphic frame.
func (t *TablePart) drawingML(shapeID int, description string, links *linkTable) string {
	if t == nil || len(t.Columns) == 0 || len(t.Rows) == 0 {
		return ""
	}
	columns := len(t.Columns)
	// The first column holds the row's name and the rest hold figures, so the
	// label column takes the room it needs and the others share what is left.
	label := t.Frame.Width * 2 / (columns + 1)
	if columns == 1 {
		label = t.Frame.Width
	}
	rest := 0
	if columns > 1 {
		rest = (t.Frame.Width - label) / (columns - 1)
	}
	var grid strings.Builder
	used := 0
	for index := 0; index < columns; index++ {
		width := rest
		if index == 0 {
			width = label
		}
		if index == columns-1 {
			width = max(t.Frame.Width-used, 1)
		}
		used += width
		grid.WriteString(`<a:gridCol w="` + strconv.Itoa(width) + `"/>`)
	}

	headerHeight, rowHeight := t.HeaderHeight, t.RowHeight
	if headerHeight <= 0 {
		headerHeight = lineHeightFor(t.HeaderSize)
	}
	if rowHeight <= 0 {
		rowHeight = lineHeightFor(t.BodySize)
	}
	var body strings.Builder
	body.WriteString(`<a:tr h="` + strconv.Itoa(headerHeight) + `">`)
	for index, heading := range t.Columns {
		body.WriteString(t.cellXML(heading, index, true, links))
	}
	body.WriteString(`</a:tr>`)
	for _, row := range t.Rows {
		body.WriteString(`<a:tr h="` + strconv.Itoa(rowHeight) + `">`)
		for index := 0; index < columns; index++ {
			value := ""
			if index < len(row) {
				value = row[index]
			}
			body.WriteString(t.cellXML(value, index, false, links))
		}
		body.WriteString(`</a:tr>`)
	}

	height := headerHeight + rowHeight*len(t.Rows)
	return `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="Table"` +
		descriptionAttribute(description) + `/>` +
		`<p:cNvGraphicFramePr><a:graphicFrameLocks noGrp="1"/></p:cNvGraphicFramePr><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="` + strconv.Itoa(t.Frame.X) + `" y="` + strconv.Itoa(t.Frame.Y) + `"/>` +
		`<a:ext cx="` + strconv.Itoa(max(t.Frame.Width, 1)) + `" cy="` + strconv.Itoa(max(height, 1)) + `"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table">` +
		`<a:tbl><a:tblPr firstRow="1" bandRow="0"/><a:tblGrid>` + grid.String() + `</a:tblGrid>` +
		body.String() + `</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
}

// cellXML writes one cell in the design's own type, with the rule under it that
// the drawn table paints.
//
// A cell is written as runs rather than as one piece of text, for the same
// reason every other line on the slide is: a word marked bold in a cell is
// bold, and an address written in one is a link someone can follow. Written as
// one piece it was neither — the preview drew the words alone while the
// exported file printed **1분기** and [근거](https://…) in the header, so the
// markup only ever appeared where the author had stopped looking.
func (t *TablePart) cellXML(value string, column int, header bool, links *linkTable) string {
	align := "l"
	if column < len(t.Aligns) && t.Aligns[column] != "" {
		align = t.Aligns[column]
	}
	size, colour, bold := t.BodySize, t.ValueInk, ""
	if column == 0 {
		colour = t.LabelInk
	}
	if header {
		size, colour, bold = t.HeaderSize, t.HeaderInk, ` b="1"`
	}
	font := ""
	if typeface := strings.TrimSpace(t.Font); typeface != "" && !strings.HasPrefix(typeface, "+") {
		font = latinTypefaceXML(typeface)
	}
	rule, width := t.Hairline, 9525
	if header {
		rule, width = t.Rule, 12700
	}
	// Only a bottom rule: an editorial table has no boxes, and PowerPoint's own
	// default would draw four sides around every cell.
	lines := `<a:lnB w="` + strconv.Itoa(width) + `" cap="flat" cmpd="sng" algn="ctr">` +
		`<a:solidFill><a:srgbClr val="` + escapeAttribute(strings.TrimPrefix(rule, "#")) + `"/></a:solidFill></a:lnB>`
	properties := `<a:rPr lang="ko-KR" sz="` + strconv.Itoa(size) + `"` + bold + `>` +
		`<a:solidFill><a:srgbClr val="` + escapeAttribute(strings.TrimPrefix(colour, "#")) + `"/></a:solidFill>` +
		font + `</a:rPr>`
	return `<a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:pPr algn="` + align + `"/>` +
		runsXML(value, properties, links) + `</a:p></a:txBody>` +
		`<a:tcPr marL="0" marR="91440" marT="45720" marB="45720" anchor="ctr">` + lines + `<a:noFill/></a:tcPr></a:tc>`
}
