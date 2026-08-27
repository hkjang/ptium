package pptx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Element is a freely positioned object drawn above a template layout. Frames
// are already converted to EMUs by the deck package, so export and preview use
// exactly the same geometry.
type Element struct {
	ID            string
	Kind          string
	Shape         string
	Frame         Frame
	Rotation      float64
	ZIndex        int
	Text          string
	Cells         [][]string
	HeaderRows    int
	HeaderColumns int
	FontFamily    string
	FontSize      int // hundredths of a point
	TextColor     string
	Bold          bool
	Italic        bool
	Underline     bool
	Align         string
	VerticalAlign string
	Fill          string
	Stroke        string
	StrokeWidth   int // EMU
	StartArrow    string
	EndArrow      string
	Dash          string
	Opacity       int // 1..100; zero means the default of 100
	Fit           string
	Caption       string
	Locked        bool
	Picture       *Picture
}

func (e Element) opacity() int {
	if e.Opacity <= 0 || e.Opacity > 100 {
		return 100
	}
	return e.Opacity
}

func (e Element) preset() string {
	if e.Kind == "line" {
		return "line"
	}
	switch strings.ToLower(strings.TrimSpace(e.Shape)) {
	case "rounded", "roundrect":
		return "roundRect"
	case "ellipse", "circle":
		return "ellipse"
	case "triangle":
		return "triangle"
	case "diamond":
		return "diamond"
	case "arrow", "rightarrow":
		return "rightArrow"
	case "star", "star5":
		return "star5"
	case "hexagon":
		return "hexagon"
	case "line":
		return "line"
	default:
		return "rect"
	}
}

func (e Element) transform() string {
	rotation := ""
	if math.Abs(e.Rotation) >= 0.005 {
		rotation = ` rot="` + strconv.Itoa(int(math.Round(e.Rotation*60000))) + `"`
	}
	return `<a:xfrm` + rotation + `><a:off x="` + strconv.Itoa(e.Frame.X) + `" y="` + strconv.Itoa(e.Frame.Y) +
		`"/><a:ext cx="` + strconv.Itoa(max(e.Frame.Width, 1)) + `" cy="` + strconv.Itoa(max(e.Frame.Height, 1)) + `"/></a:xfrm>`
}

func (e Element) outlineXML() string {
	if strings.TrimSpace(e.Stroke) == "" || strings.EqualFold(strings.TrimSpace(e.Stroke), "transparent") {
		return `<a:ln><a:noFill/></a:ln>`
	}
	width := e.StrokeWidth
	if width <= 0 {
		width = EMUPerPoint
	}
	dash := ""
	if e.Dash != "" && e.Dash != "solid" {
		dash = `<a:prstDash val="` + escapeAttribute(e.Dash) + `"/>`
	}
	arrow := func(tag, value string) string {
		if value == "" || value == "none" {
			return ""
		}
		return `<a:` + tag + ` type="` + escapeAttribute(value) + `" w="med" len="med"/>`
	}
	return `<a:ln w="` + strconv.Itoa(width) + `" cap="rnd"><a:solidFill>` +
		solidColor(e.Stroke, e.opacity()) + `</a:solidFill>` + dash + `<a:round/>` +
		arrow("headEnd", e.StartArrow) + arrow("tailEnd", e.EndArrow) + `</a:ln>`
}

func (e Element) fillXML() string {
	if e.Kind == "text" || e.Kind == "line" || strings.TrimSpace(e.Fill) == "" || strings.EqualFold(strings.TrimSpace(e.Fill), "transparent") {
		return `<a:noFill/>`
	}
	return `<a:solidFill>` + solidColor(e.Fill, e.opacity()) + `</a:solidFill>`
}

func (e Element) locksXML() string {
	if !e.Locked {
		return ""
	}
	return `<a:spLocks noMove="1" noResize="1" noRot="1"/>`
}

// An object can be aligned in the words a browser uses or in the words
// DrawingML uses. The workspace already converts between them when it lifts a
// template region onto the canvas, so both are read here — and a value that is
// neither is refused on the way in rather than drawn quietly at the left.
var horizontalAligns = map[string]string{
	"center": "ctr", "ctr": "ctr", "right": "r", "r": "r",
	"justify": "just", "just": "just", "left": "", "l": "", "": "",
}

var verticalAnchors = map[string]string{
	"middle": "ctr", "center": "ctr", "ctr": "ctr",
	"bottom": "b", "b": "b", "top": "t", "t": "t", "": "t",
}

// AlignmentIsKnown reports whether an object's alignment is one the renderer
// understands, so a caller hears about a typo instead of finding its text at
// the left of the shape.
func AlignmentIsKnown(align, verticalAlign string) bool {
	_, horizontal := horizontalAligns[strings.ToLower(strings.TrimSpace(align))]
	_, vertical := verticalAnchors[strings.ToLower(strings.TrimSpace(verticalAlign))]
	return horizontal && vertical
}

// drawingML emits an editable PowerPoint shape or text box. Images are emitted
// by freeformPictureXML after their relationship id has been allocated.
func (e Element) drawingML(shapeID int, links *linkTable) string {
	if e.Kind == "image" {
		return ""
	}
	if e.Kind == "table" {
		return e.tableDrawingML(shapeID)
	}
	name := strings.TrimSpace(e.ID)
	if name == "" {
		name = fmt.Sprintf("Element %d", shapeID)
	}
	preset := e.preset()
	fill := e.fillXML()
	if e.Kind == "line" {
		preset = "line"
		fill = `<a:noFill/>`
		if strings.TrimSpace(e.Stroke) == "" {
			e.Stroke = "808080"
		}
	}
	txBox := ""
	if e.Kind == "text" {
		txBox = ` txBox="1"`
	}
	body := ""
	if e.Kind == "text" || strings.TrimSpace(e.Text) != "" {
		body = e.textBodyXML(links)
	} else {
		body = `<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p></p:txBody>`
	}
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="` + escapeAttribute(name) +
		`"/><p:cNvSpPr` + txBox + `>` + e.locksXML() + `</p:cNvSpPr><p:nvPr/></p:nvSpPr>` +
		`<p:spPr>` + e.transform() + `<a:prstGeom prst="` + preset + `"><a:avLst/></a:prstGeom>` + fill + e.outlineXML() +
		`</p:spPr>` + body + `</p:sp>`
}

func (e Element) tableDrawingML(shapeID int) string {
	if len(e.Cells) == 0 || len(e.Cells[0]) == 0 {
		return ""
	}
	rows, columns := len(e.Cells), len(e.Cells[0])
	name := strings.TrimSpace(e.ID)
	if name == "" {
		name = fmt.Sprintf("Table %d", shapeID)
	}
	rotation := ""
	if math.Abs(e.Rotation) >= .005 {
		rotation = ` rot="` + strconv.Itoa(int(math.Round(e.Rotation*60000))) + `"`
	}
	locks := ""
	if e.Locked {
		locks = `<a:graphicFrameLocks noMove="1" noResize="1" noRot="1"/>`
	}
	columnWidth := max(e.Frame.Width/columns, 1)
	rowHeight := max(e.Frame.Height/rows, 1)
	var grid, body strings.Builder
	for column := 0; column < columns; column++ {
		width := columnWidth
		if column == columns-1 {
			width = max(e.Frame.Width-columnWidth*(columns-1), 1)
		}
		grid.WriteString(`<a:gridCol w="` + strconv.Itoa(width) + `"/>`)
	}
	for rowIndex, row := range e.Cells {
		height := rowHeight
		if rowIndex == rows-1 {
			height = max(e.Frame.Height-rowHeight*(rows-1), 1)
		}
		body.WriteString(`<a:tr h="` + strconv.Itoa(height) + `">`)
		for columnIndex := 0; columnIndex < columns; columnIndex++ {
			text := ""
			if columnIndex < len(row) {
				text = row[columnIndex]
			}
			header := rowIndex < e.HeaderRows || columnIndex < e.HeaderColumns
			body.WriteString(e.tableCellXML(text, header))
		}
		body.WriteString(`</a:tr>`)
	}
	firstRow := "0"
	if e.HeaderRows > 0 {
		firstRow = "1"
	}
	return `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="` + escapeAttribute(name) +
		`"/><p:cNvGraphicFramePr>` + locks + `</p:cNvGraphicFramePr><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm` + rotation + `><a:off x="` + strconv.Itoa(e.Frame.X) + `" y="` + strconv.Itoa(e.Frame.Y) +
		`"/><a:ext cx="` + strconv.Itoa(max(e.Frame.Width, 1)) + `" cy="` + strconv.Itoa(max(e.Frame.Height, 1)) +
		`"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table">` +
		`<a:tbl><a:tblPr firstRow="` + firstRow + `" bandRow="0"/><a:tblGrid>` + grid.String() + `</a:tblGrid>` +
		body.String() + `</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`
}

func (e Element) tableCellXML(text string, header bool) string {
	fontSize := e.FontSize
	if fontSize <= 0 {
		fontSize = 1400
	}
	fill := "FFFFFF"
	textColor := e.TextColor
	if strings.TrimSpace(textColor) == "" {
		textColor = "20242D"
	}
	bold := ""
	if header {
		fill = e.Fill
		if strings.TrimSpace(fill) == "" || strings.EqualFold(fill, "transparent") {
			fill = "725BD6"
		}
		textColor = "FFFFFF"
		bold = ` b="1"`
	}
	font := ""
	if family := strings.TrimSpace(e.FontFamily); family != "" && !strings.HasPrefix(family, "+") {
		font = latinTypefaceXML(family)
	}
	border := e.Stroke
	if strings.TrimSpace(border) == "" || strings.EqualFold(border, "transparent") {
		border = "D9D6E1"
	}
	borderWidth := e.StrokeWidth
	if borderWidth <= 0 {
		borderWidth = EMUPerPoint
	}
	line := `<a:solidFill>` + solidColor(border, e.opacity()) + `</a:solidFill><a:prstDash val="solid"/>`
	properties := `<a:tcPr marL="45720" marR="45720" marT="27432" marB="27432"><a:solidFill>` + solidColor(fill, e.opacity()) +
		`</a:solidFill><a:lnL w="` + strconv.Itoa(borderWidth) + `">` + line + `</a:lnL><a:lnR w="` + strconv.Itoa(borderWidth) + `">` + line +
		`</a:lnR><a:lnT w="` + strconv.Itoa(borderWidth) + `">` + line + `</a:lnT><a:lnB w="` + strconv.Itoa(borderWidth) + `">` + line + `</a:lnB></a:tcPr>`
	return `<a:tc><a:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"><a:buNone/></a:pPr><a:r><a:rPr lang="ko-KR" sz="` +
		strconv.Itoa(fontSize) + `" dirty="0"` + bold + `><a:solidFill>` + solidColor(textColor, e.opacity()) + `</a:solidFill>` + font +
		`</a:rPr><a:t>` + escapeText(text) + `</a:t></a:r></a:p></a:txBody>` + properties + `</a:tc>`
}

func (e Element) textBodyXML(links *linkTable) string {
	anchor := verticalAnchors[strings.ToLower(strings.TrimSpace(e.VerticalAlign))]
	if anchor == "" {
		anchor = "t"
	}
	align := horizontalAligns[strings.ToLower(strings.TrimSpace(e.Align))]
	if align == "" {
		align = "l"
	}
	size := e.FontSize
	if size <= 0 {
		size = 1800
	}
	color := e.TextColor
	if strings.TrimSpace(color) == "" {
		color = "20242D"
	}
	font := ""
	if family := strings.TrimSpace(e.FontFamily); family != "" && !strings.HasPrefix(family, "+") {
		font = latinTypefaceXML(family)
	}
	runFlags := ""
	if e.Bold {
		runFlags += ` b="1"`
	}
	if e.Italic {
		runFlags += ` i="1"`
	}
	if e.Underline {
		runFlags += ` u="sng"`
	}
	lines := strings.Split(strings.ReplaceAll(e.Text, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	var paragraphs strings.Builder
	for _, line := range lines {
		properties := `<a:rPr lang="ko-KR" sz="` + strconv.Itoa(size) + `" dirty="0"` + runFlags +
			`><a:solidFill>` + solidColor(color, e.opacity()) + `</a:solidFill>` + font + `</a:rPr>`
		paragraphs.WriteString(`<a:p><a:pPr algn="` + align + `"><a:buNone/></a:pPr>` +
			runsXML(line, properties, links) + `</a:p>`)
	}
	return `<p:txBody><a:bodyPr wrap="square" lIns="45720" tIns="27432" rIns="45720" bIns="27432" anchor="` +
		anchor + `"><a:normAutofit/></a:bodyPr><a:lstStyle/>` + paragraphs.String() + `</p:txBody>`
}

// SVG draws the browser preview equivalent of drawingML.
func (e Element) SVG(scale float64, density int) string {
	x := float64(e.Frame.X) * scale
	y := float64(e.Frame.Y) * scale
	width := float64(e.Frame.Width) * scale
	height := float64(e.Frame.Height) * scale
	if width <= 0 || height <= 0 {
		return ""
	}
	transform := ""
	if math.Abs(e.Rotation) >= 0.005 {
		transform = fmt.Sprintf(` transform="rotate(%.3f %.2f %.2f)"`, e.Rotation, x+width/2, y+height/2)
	}
	opacity := ""
	if e.opacity() < 100 {
		opacity = fmt.Sprintf(` opacity="%.3f"`, float64(e.opacity())/100)
	}
	var body strings.Builder
	body.WriteString(`<g` + transform + opacity + `>`)
	if e.Kind == "table" {
		body.WriteString(e.tableSVG(x, y, width, height, scale))
	} else if e.Kind == "image" {
		body.WriteString(e.imageSVG(x, y, width, height, density))
	} else {
		body.WriteString(e.geometrySVG(x, y, width, height, scale))
		if e.Kind == "text" || strings.TrimSpace(e.Text) != "" {
			body.WriteString(e.textSVG(x, y, width, height, scale))
		}
	}
	body.WriteString(`</g>`)
	return body.String()
}

func (e Element) tableSVG(x, y, width, height, scale float64) string {
	if len(e.Cells) == 0 || len(e.Cells[0]) == 0 {
		return ""
	}
	rows, columns := len(e.Cells), len(e.Cells[0])
	cellWidth, cellHeight := width/float64(columns), height/float64(rows)
	fontSize := e.FontSize
	if fontSize <= 0 {
		fontSize = 1400
	}
	pixelFont := float64(fontSize) / 100 * float64(EMUPerPoint) * scale
	if pixelFont < 4 {
		pixelFont = 4
	}
	border := e.Stroke
	if strings.TrimSpace(border) == "" || strings.EqualFold(border, "transparent") {
		border = "D9D6E1"
	}
	strokeWidth := math.Max(float64(e.StrokeWidth)*scale, .7)
	family := strings.TrimSpace(e.FontFamily)
	if family == "" {
		family = "Aptos"
	}
	var builder strings.Builder
	for rowIndex, row := range e.Cells {
		for columnIndex := 0; columnIndex < columns; columnIndex++ {
			cellX, cellY := x+float64(columnIndex)*cellWidth, y+float64(rowIndex)*cellHeight
			header := rowIndex < e.HeaderRows || columnIndex < e.HeaderColumns
			fill, color, weight := "FFFFFF", colorOrGrey(e.TextColor), "400"
			if strings.TrimSpace(e.TextColor) == "" {
				color = "20242D"
			}
			if header {
				fill, color, weight = colorOrGrey(e.Fill), "FFFFFF", "700"
				if strings.TrimSpace(e.Fill) == "" || strings.EqualFold(e.Fill, "transparent") {
					fill = "725BD6"
				}
			}
			text := ""
			if columnIndex < len(row) {
				text = row[columnIndex]
			}
			fmt.Fprintf(&builder, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="#%s" stroke="#%s" stroke-width="%.2f"/>`,
				cellX, cellY, cellWidth, cellHeight, fill, colorOrGrey(border), strokeWidth)
			fmt.Fprintf(&builder, `<text x="%.2f" y="%.2f" text-anchor="middle" dominant-baseline="middle" fill="#%s" font-size="%.2f" font-weight="%s" font-family="%s, `+previewFallbacks+`">%s</text>`,
				cellX+cellWidth/2, cellY+cellHeight/2, color, pixelFont, weight, escapeAttribute(family), escapeText(text))
		}
	}
	return builder.String()
}

func (e Element) geometrySVG(x, y, width, height, scale float64) string {
	fill := "none"
	if e.Kind != "text" && e.Kind != "line" && strings.TrimSpace(e.Fill) != "" && !strings.EqualFold(e.Fill, "transparent") {
		fill = "#" + colorOrGrey(e.Fill)
	}
	stroke := "none"
	if strings.TrimSpace(e.Stroke) != "" && !strings.EqualFold(e.Stroke, "transparent") {
		stroke = "#" + colorOrGrey(e.Stroke)
	}
	strokeWidth := float64(e.StrokeWidth) * scale
	if stroke != "none" && strokeWidth < 0.8 {
		strokeWidth = 1
	}
	attributes := fmt.Sprintf(` fill="%s" stroke="%s" stroke-width="%.2f"`, fill, stroke, strokeWidth)
	switch e.preset() {
	case "ellipse":
		return fmt.Sprintf(`<ellipse cx="%.2f" cy="%.2f" rx="%.2f" ry="%.2f"%s/>`, x+width/2, y+height/2, width/2, height/2, attributes)
	case "triangle":
		return fmt.Sprintf(`<polygon points="%.2f,%.2f %.2f,%.2f %.2f,%.2f"%s/>`, x+width/2, y, x+width, y+height, x, y+height, attributes)
	case "diamond":
		return fmt.Sprintf(`<polygon points="%.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f"%s/>`, x+width/2, y, x+width, y+height/2, x+width/2, y+height, x, y+height/2, attributes)
	case "rightArrow":
		return fmt.Sprintf(`<polygon points="%.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f"%s/>`,
			x, y+height*.25, x+width*.62, y+height*.25, x+width*.62, y, x+width, y+height*.5,
			x+width*.62, y+height, x+width*.62, y+height*.75, x, y+height*.75, attributes)
	case "star5":
		points := make([]string, 0, 10)
		for index := 0; index < 10; index++ {
			angle := -math.Pi/2 + float64(index)*math.Pi/5
			radius := 0.5
			if index%2 == 1 {
				radius = 0.22
			}
			points = append(points, fmt.Sprintf("%.2f,%.2f", x+width/2+math.Cos(angle)*width*radius, y+height/2+math.Sin(angle)*height*radius))
		}
		return `<polygon points="` + strings.Join(points, " ") + `"` + attributes + `/>`
	case "hexagon":
		return fmt.Sprintf(`<polygon points="%.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f"%s/>`,
			x+width*.25, y, x+width*.75, y, x+width, y+height/2, x+width*.75, y+height, x+width*.25, y+height, x, y+height/2, attributes)
	case "line":
		if stroke == "none" {
			stroke = "#808080"
		}
		markerID := svgIdentifier(e.ID)
		markers := ""
		start, end := "", ""
		if e.StartArrow != "" && e.StartArrow != "none" {
			start = ` marker-start="url(#` + markerID + `-start)"`
			markers += svgMarker(markerID+"-start", e.StartArrow, stroke)
		}
		if e.EndArrow != "" && e.EndArrow != "none" {
			end = ` marker-end="url(#` + markerID + `-end)"`
			markers += svgMarker(markerID+"-end", e.EndArrow, stroke)
		}
		dash := ""
		switch e.Dash {
		case "dash":
			dash = ` stroke-dasharray="8 5"`
		case "dot":
			dash = ` stroke-dasharray="2 4"`
		case "dashDot":
			dash = ` stroke-dasharray="8 4 2 4"`
		}
		return `<defs>` + markers + `</defs>` + fmt.Sprintf(`<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="%s" stroke-width="%.2f" stroke-linecap="round"%s%s%s/>`,
			x, y, x+width, y+height, stroke, math.Max(strokeWidth, 1), dash, start, end)
	case "roundRect":
		return fmt.Sprintf(`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="%.2f"%s/>`, x, y, width, height, math.Min(width, height)*.12, attributes)
	default:
		return fmt.Sprintf(`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f"%s/>`, x, y, width, height, attributes)
	}
}

func svgIdentifier(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' {
			builder.WriteRune(character)
		}
	}
	if builder.Len() == 0 {
		return "freeform-line"
	}
	return builder.String()
}

func svgMarker(id, kind, color string) string {
	shape := `<path d="M 0 0 L 10 5 L 0 10 z" fill="` + color + `"/>`
	switch kind {
	case "diamond":
		shape = `<path d="M 0 5 L 5 0 L 10 5 L 5 10 z" fill="` + color + `"/>`
	case "oval":
		shape = `<circle cx="5" cy="5" r="4" fill="` + color + `"/>`
	case "stealth":
		shape = `<path d="M 0 1 L 10 5 L 0 9 L 3 5 z" fill="` + color + `"/>`
	}
	return `<marker id="` + escapeAttribute(id) + `" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">` + shape + `</marker>`
}

func (e Element) textSVG(x, y, width, height, scale float64) string {
	size := e.FontSize
	if size <= 0 {
		size = 1800
	}
	fontSize := float64(size) / 100 * float64(EMUPerPoint) * scale
	if fontSize < 4 {
		fontSize = 4
	}
	lineHeight := fontSize * 1.22
	lineEm := float64(e.Frame.Width) / (float64(size) / 100 * float64(EMUPerPoint))
	if lineEm < 1 {
		lineEm = 1
	}
	// A text box carries links the same way a bullet does, so the picture of it
	// draws the words rather than the markup that puts them there.
	var runs []TextRun
	wrapped := make([]string, 0)
	for _, line := range strings.Split(strings.ReplaceAll(e.Text, "\r\n", "\n"), "\n") {
		runs = append(runs, SplitLinks(line)...)
		pieces := wrapLines(PlainText(line), lineEm)
		if len(pieces) == 0 {
			pieces = []string{""}
		}
		wrapped = append(wrapped, pieces...)
	}
	blockHeight := float64(len(wrapped)) * lineHeight
	top := y + math.Max(2, height*.04)
	switch verticalAnchors[strings.ToLower(strings.TrimSpace(e.VerticalAlign))] {
	case "ctr":
		top = y + (height-blockHeight)/2
	case "b":
		top = y + height - blockHeight - math.Max(2, height*.04)
	}
	anchor := "start"
	textX := x + math.Max(3, width*.025)
	switch horizontalAligns[strings.ToLower(strings.TrimSpace(e.Align))] {
	case "ctr":
		anchor, textX = "middle", x+width/2
	case "r":
		anchor, textX = "end", x+width-math.Max(3, width*.025)
	}
	weight := "400"
	if e.Bold {
		weight = "700"
	}
	style := ""
	if e.Italic {
		style += ` font-style="italic"`
	}
	if e.Underline {
		style += ` text-decoration="underline"`
	}
	color := e.TextColor
	if strings.TrimSpace(color) == "" {
		color = "20242D"
	}
	family := strings.TrimSpace(e.FontFamily)
	if family == "" {
		family = "Aptos"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, `<text x="%.2f" y="%.2f" fill="#%s" font-size="%.2f" font-weight="%s" text-anchor="%s" font-family="%s, `+previewFallbacks+`"%s>`,
		textX, top+fontSize*.84, colorOrGrey(color), fontSize, weight, anchor, escapeAttribute(family), style)
	for index, line := range wrapped {
		fmt.Fprintf(&builder, `<tspan x="%.2f" y="%.2f">%s</tspan>`, textX, top+fontSize*.84+float64(index)*lineHeight,
			// The exported run keeps the box's own colour, so the picture of it
			// does too: the link is told by the underline, and the two agree.
			markedUpLine(line, runs, colorOrGrey(color)))
	}
	builder.WriteString(`</text>`)
	return builder.String()
}

func (e Element) imageSVG(x, y, width, height float64, density int) string {
	if e.Picture == nil || len(e.Picture.Data) == 0 {
		return ""
	}
	// The box is already in the preview's own pixels, so the picture inside can
	// be embedded at the size it is drawn at rather than at a fixed ceiling.
	cover := strings.ToLower(e.Fit) != "contain"
	uri := mediaDataURI(pictureCacheName(*e.Picture), e.Picture.Data,
		pictureBox{Width: width, Height: height, Cover: cover, Density: density}, previewImagePixels)
	if uri == "" {
		return ""
	}
	fit := "xMidYMid slice"
	switch strings.ToLower(e.Fit) {
	case "contain":
		fit = "xMidYMid meet"
	case "fill":
		fit = "none"
	}
	return fmt.Sprintf(`<svg x="%.2f" y="%.2f" width="%.2f" height="%.2f" overflow="hidden"><image x="0" y="0" width="100%%" height="100%%" href="%s" preserveAspectRatio="%s"/></svg>`,
		x, y, width, height, escapeAttribute(uri), fit)
}
