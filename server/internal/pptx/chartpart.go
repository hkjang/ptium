package pptx

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"
)

// A chart someone cannot change the numbers of is a picture of a chart.
//
// The same trade Ptium makes for tables it makes for charts. A drawing is what
// the preview shows and what the measurement pass reasons about, but the file
// carries a real PowerPoint chart: click it, "데이터 편집", and the figures open
// in Excel. That is what anyone who receives a deck expects to be able to do
// when a number turns out to be wrong an hour before the meeting.
//
// The chart is styled to the template's own design — its accent hues, its body
// font, its rule colour — so the editable chart looks like the drawn one rather
// than like Office's blue default.

const (
	chartColumn = "col"
	chartBar    = "bar"
	chartLine   = "line"
)

// ChartSeries is one plotted series: its name, its numbers, and the hue the
// design gives it. PointColors, when set, colours single points individually,
// which is how an emphasised category keeps the accent while the rest recede.
type ChartSeries struct {
	Name        string
	Values      []float64
	Color       string
	PointColors []string
	// LabelPoints are the points that carry their value, matching what the
	// drawing writes on the marks: every column of a short chart, only the
	// emphasised one of a crowded chart, only the end of a line.
	LabelPoints []int
}

// ChartPart is a chart as PowerPoint holds one.
type ChartPart struct {
	Frame      Frame
	Kind       string
	Categories []string
	Series     []ChartSeries
	// FormatCode is the number format the data labels are written in, carrying
	// the block's unit so a label reads "12%" rather than "12".
	FormatCode string
	FontSize   int
	Font       string
	LabelInk   string
	AxisInk    string
	AxisLine   string
	Legend     bool
}

// graphicFrame writes the frame that points at the chart part.
func (c *ChartPart) graphicFrame(shapeID int, relationshipID, description string) string {
	if c == nil || relationshipID == "" {
		return ""
	}
	return `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="Chart"` +
		descriptionAttribute(description) + `/>` +
		`<p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="` + strconv.Itoa(c.Frame.X) + `" y="` + strconv.Itoa(c.Frame.Y) + `"/>` +
		`<a:ext cx="` + strconv.Itoa(max(c.Frame.Width, 1)) + `" cy="` + strconv.Itoa(max(c.Frame.Height, 1)) + `"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart">` +
		`<c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="` +
		escapeAttribute(relationshipID) + `"/></a:graphicData></a:graphic></p:graphicFrame>`
}

const (
	chartCategoryAxisID = "111111111"
	chartValueAxisID    = "222222222"
)

// chartSpaceXML writes the chart part itself.
func (c *ChartPart) chartSpaceXML(workbookRelID string) string {
	if c == nil || len(c.Series) == 0 {
		return ""
	}
	var plot strings.Builder
	if c.Kind == chartLine {
		plot.WriteString(`<c:lineChart><c:grouping val="standard"/><c:varyColors val="0"/>`)
	} else {
		direction := "col"
		if c.Kind == chartBar {
			direction = "bar"
		}
		plot.WriteString(`<c:barChart><c:barDir val="` + direction + `"/><c:grouping val="clustered"/><c:varyColors val="0"/>`)
	}
	for index, series := range c.Series {
		plot.WriteString(c.seriesXML(index, series))
	}
	if c.Kind == chartLine {
		plot.WriteString(`<c:marker val="1"/>`)
	} else {
		// The drawn bar keeps to a little over half its band and leaves the rest
		// as air; the same proportion in a chart is a gap of eighty.
		plot.WriteString(`<c:gapWidth val="80"/><c:overlap val="-20"/>`)
	}
	plot.WriteString(`<c:axId val="` + chartCategoryAxisID + `"/><c:axId val="` + chartValueAxisID + `"/>`)
	if c.Kind == chartLine {
		plot.WriteString(`</c:lineChart>`)
	} else {
		plot.WriteString(`</c:barChart>`)
	}

	position := "b"
	if c.Kind == chartBar {
		position = "l"
	}
	plot.WriteString(`<c:catAx><c:axId val="` + chartCategoryAxisID + `"/><c:scaling><c:orientation val="minMax"/></c:scaling>` +
		`<c:delete val="0"/><c:axPos val="` + position + `"/><c:majorTickMark val="none"/><c:minorTickMark val="none"/>` +
		`<c:tickLblPos val="nextTo"/>` + c.axisLineXML() + c.axisTextXML() +
		`<c:crossAx val="` + chartValueAxisID + `"/><c:crosses val="autoZero"/><c:auto val="1"/>` +
		`<c:lblAlgn val="ctr"/><c:lblOffset val="100"/><c:noMultiLvlLbl val="0"/></c:catAx>`)
	// The value axis is deleted because the numbers are on the marks. An axis
	// and a label both saying 1,240 is the number twice.
	valuePosition := "l"
	if c.Kind == chartBar {
		valuePosition = "b"
	}
	plot.WriteString(`<c:valAx><c:axId val="` + chartValueAxisID + `"/><c:scaling><c:orientation val="minMax"/></c:scaling>` +
		`<c:delete val="1"/><c:axPos val="` + valuePosition + `"/><c:majorTickMark val="none"/><c:minorTickMark val="none"/>` +
		`<c:tickLblPos val="none"/><c:crossAx val="` + chartCategoryAxisID + `"/><c:crosses val="autoZero"/></c:valAx>`)

	legend := ""
	if c.Legend {
		legend = `<c:legend><c:legendPos val="t"/><c:overlay val="0"/>` + c.axisTextXML() + `</c:legend>`
	}
	external := ""
	if workbookRelID != "" {
		external = `<c:externalData r:id="` + escapeAttribute(workbookRelID) + `"><c:autoUpdate val="0"/></c:externalData>`
	}
	return xmlDeclaration +
		`<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" ` +
		`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<c:date1904 val="0"/><c:roundedCorners val="0"/><c:chart><c:autoTitleDeleted val="1"/>` +
		`<c:plotArea><c:layout/>` + plot.String() + `</c:plotArea>` + legend +
		`<c:plotVisOnly val="1"/><c:dispBlanksAs val="gap"/></c:chart>` +
		`<c:spPr><a:noFill/><a:ln><a:noFill/></a:ln></c:spPr>` + c.axisTextXML() + external + `</c:chartSpace>`
}

func (c *ChartPart) seriesXML(index int, series ChartSeries) string {
	var builder strings.Builder
	builder.WriteString(`<c:ser><c:idx val="` + strconv.Itoa(index) + `"/><c:order val="` + strconv.Itoa(index) + `"/>`)
	name := strings.TrimSpace(series.Name)
	if name != "" {
		builder.WriteString(`<c:tx><c:strRef><c:f>Sheet1!$` + columnLetter(index+1) + `$1</c:f>` +
			`<c:strCache><c:ptCount val="1"/><c:pt idx="0"><c:v>` + escapeText(name) + `</c:v></c:pt></c:strCache></c:strRef></c:tx>`)
	}
	color := series.Color
	if color == "" {
		color = "4472C4"
	}
	if c.Kind == chartLine {
		builder.WriteString(`<c:spPr><a:ln w="28575" cap="rnd"><a:solidFill><a:srgbClr val="` + hexColor(color) +
			`"/></a:solidFill><a:round/></a:ln></c:spPr>` +
			`<c:marker><c:symbol val="circle"/><c:size val="6"/><c:spPr><a:solidFill><a:srgbClr val="` + hexColor(color) +
			`"/></a:solidFill><a:ln><a:noFill/></a:ln></c:spPr></c:marker>`)
	} else {
		builder.WriteString(`<c:spPr><a:solidFill><a:srgbClr val="` + hexColor(color) + `"/></a:solidFill><a:ln><a:noFill/></a:ln></c:spPr>` +
			`<c:invertIfNegative val="0"/>`)
		for point, pointColor := range series.PointColors {
			if pointColor == "" || pointColor == color {
				continue
			}
			builder.WriteString(`<c:dPt><c:idx val="` + strconv.Itoa(point) + `"/><c:invertIfNegative val="0"/><c:bubble3D val="0"/>` +
				`<c:spPr><a:solidFill><a:srgbClr val="` + hexColor(pointColor) + `"/></a:solidFill><a:ln><a:noFill/></a:ln></c:spPr></c:dPt>`)
		}
	}
	builder.WriteString(c.labelsXML(series))
	builder.WriteString(`<c:cat><c:strRef><c:f>Sheet1!$A$2:$A$` + strconv.Itoa(len(c.Categories)+1) + `</c:f><c:strCache><c:ptCount val="` +
		strconv.Itoa(len(c.Categories)) + `"/>`)
	for point, category := range c.Categories {
		builder.WriteString(`<c:pt idx="` + strconv.Itoa(point) + `"><c:v>` + escapeText(category) + `</c:v></c:pt>`)
	}
	builder.WriteString(`</c:strCache></c:strRef></c:cat>`)
	column := columnLetter(index + 1)
	builder.WriteString(`<c:val><c:numRef><c:f>Sheet1!$` + column + `$2:$` + column + `$` + strconv.Itoa(len(series.Values)+1) +
		`</c:f><c:numCache><c:formatCode>General</c:formatCode><c:ptCount val="` + strconv.Itoa(len(series.Values)) + `"/>`)
	for point, value := range series.Values {
		builder.WriteString(`<c:pt idx="` + strconv.Itoa(point) + `"><c:v>` + formatChartNumber(value) + `</c:v></c:pt>`)
	}
	builder.WriteString(`</c:numCache></c:numRef></c:val>`)
	if c.Kind == chartLine {
		builder.WriteString(`<c:smooth val="0"/>`)
	}
	builder.WriteString(`</c:ser>`)
	return builder.String()
}

// labelsXML writes the data labels. Which points carry one is the drawing's
// decision, kept here so the file and the preview label the same marks.
func (c *ChartPart) labelsXML(series ChartSeries) string {
	shown := `<c:numFmt formatCode="` + escapeAttribute(c.numberFormat()) + `" sourceLinked="0"/>` +
		`<c:spPr><a:noFill/><a:ln><a:noFill/></a:ln></c:spPr>` + c.labelTextXML() +
		`<c:dLblPos val="` + c.labelPosition() + `"/>` +
		`<c:showLegendKey val="0"/><c:showVal val="1"/><c:showCatName val="0"/><c:showSerName val="0"/>` +
		`<c:showPercent val="0"/><c:showBubbleSize val="0"/>`
	if series.LabelPoints == nil {
		return `<c:dLbls>` + shown + `</c:dLbls>`
	}
	var builder strings.Builder
	builder.WriteString(`<c:dLbls>`)
	for _, point := range series.LabelPoints {
		if point < 0 || point >= len(series.Values) {
			continue
		}
		builder.WriteString(`<c:dLbl><c:idx val="` + strconv.Itoa(point) + `"/>` + shown + `</c:dLbl>`)
	}
	builder.WriteString(`<c:delete val="1"/></c:dLbls>`)
	return builder.String()
}

func (c *ChartPart) labelPosition() string {
	switch c.Kind {
	case chartLine:
		return "r"
	default:
		return "outEnd"
	}
}

func (c *ChartPart) numberFormat() string {
	if format := strings.TrimSpace(c.FormatCode); format != "" {
		return format
	}
	return "General"
}

func (c *ChartPart) axisTextXML() string {
	return `<c:txPr><a:bodyPr/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="` + strconv.Itoa(c.size()) +
		`"><a:solidFill><a:srgbClr val="` + hexColor(c.AxisInk) + `"/></a:solidFill>` + c.typefaceXML() +
		`</a:defRPr></a:pPr><a:endParaRPr lang="ko-KR"/></a:p></c:txPr>`
}

func (c *ChartPart) labelTextXML() string {
	return `<c:txPr><a:bodyPr/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="` + strconv.Itoa(c.size()) +
		`" b="1"><a:solidFill><a:srgbClr val="` + hexColor(c.LabelInk) + `"/></a:solidFill>` + c.typefaceXML() +
		`</a:defRPr></a:pPr><a:endParaRPr lang="ko-KR"/></a:p></c:txPr>`
}

func (c *ChartPart) typefaceXML() string {
	font := strings.TrimSpace(c.Font)
	if font == "" {
		return ""
	}
	return `<a:latin typeface="` + escapeAttribute(font) + `"/><a:ea typeface="` + escapeAttribute(font) + `"/>`
}

func (c *ChartPart) axisLineXML() string {
	return `<c:spPr><a:ln w="9525"><a:solidFill><a:srgbClr val="` + hexColor(c.AxisLine) + `"/></a:solidFill></a:ln></c:spPr>`
}

func (c *ChartPart) size() int {
	if c.FontSize > 0 {
		return c.FontSize
	}
	return 1000
}

// hexColor normalises a design colour to the six hex digits DrawingML wants.
func hexColor(value string) string {
	trimmed := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(value), "#"))
	if len(trimmed) == 8 {
		trimmed = trimmed[:6]
	}
	if len(trimmed) != 6 {
		return "595959"
	}
	for _, r := range trimmed {
		if !strings.ContainsRune("0123456789ABCDEF", r) {
			return "595959"
		}
	}
	return trimmed
}

func formatChartNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// columnLetter maps a zero-based column to its spreadsheet letter. A chart is
// capped well below twenty-six series, so one letter is enough.
func columnLetter(index int) string {
	if index < 0 {
		index = 0
	}
	return string(rune('A' + index%26))
}

// --- the workbook behind the chart ------------------------------------------

// chartWorkbook builds the spreadsheet PowerPoint opens when someone asks to
// edit the data. Without it the chart still draws — it carries its own cached
// numbers — but "데이터 편집" reports a missing file, and a chart nobody can edit
// is the picture of a chart this whole part exists to avoid.
func (c *ChartPart) chartWorkbook() []byte {
	var sheet strings.Builder
	sheet.WriteString(`<row r="1">`)
	for index, series := range c.Series {
		sheet.WriteString(cellXML(columnLetter(index+1)+"1", series.Name))
	}
	sheet.WriteString(`</row>`)
	for row, category := range c.Categories {
		reference := strconv.Itoa(row + 2)
		sheet.WriteString(`<row r="` + reference + `">` + cellXML("A"+reference, category))
		for index, series := range c.Series {
			if row >= len(series.Values) {
				continue
			}
			sheet.WriteString(`<c r="` + columnLetter(index+1) + reference + `"><v>` +
				formatChartNumber(series.Values[row]) + `</v></c>`)
		}
		sheet.WriteString(`</row>`)
	}

	files := [][2]string{
		{"[Content_Types].xml", xmlDeclaration +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`</Types>`},
		{"_rels/.rels", relationshipsDocument(`<Relationship Id="rId1" Type="` + relationshipNamespace +
			`/officeDocument" Target="xl/workbook.xml"/>`)},
		{"xl/workbook.xml", xmlDeclaration +
			`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
			`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", relationshipsDocument(`<Relationship Id="rId1" Type="` + relationshipNamespace +
			`/worksheet" Target="worksheets/sheet1.xml"/>`)},
		{"xl/worksheets/sheet1.xml", xmlDeclaration +
			`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
			`<dimension ref="A1:` + columnLetter(len(c.Series)) + strconv.Itoa(len(c.Categories)+1) + `"/>` +
			`<sheetViews><sheetView workbookViewId="0"/></sheetViews>` +
			`<sheetFormatPr defaultRowHeight="15"/>` +
			`<sheetData>` + sheet.String() + `</sheetData></worksheet>`},
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: file[0], Method: zip.Deflate})
		if err != nil {
			return nil
		}
		if _, err := entry.Write([]byte(file[1])); err != nil {
			return nil
		}
	}
	if err := writer.Close(); err != nil {
		return nil
	}
	return buffer.Bytes()
}

func cellXML(reference, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return `<c r="` + reference + `" t="inlineStr"><is><t xml:space="preserve">` + escapeText(value) + `</t></is></c>`
}
