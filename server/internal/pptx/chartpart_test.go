package pptx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func chartDeck(block Block) Deck {
	return Deck{Language: "ko", Title: "차트", Slides: []Slide{
		{LayoutID: "title", Fields: map[string][]Paragraph{SlotTitle: {{Text: "차트"}}}},
		{LayoutID: "content", Fields: map[string][]Paragraph{SlotTitle: {{Text: "분기 매출"}}},
			Blocks: map[string]Block{SlotBody: block}},
	}}
}

func renderedParts(t *testing.T, deck Deck) map[string]string {
	t.Helper()
	template, err := BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	data, err := RenderBytes(template, deck)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	parts := map[string]string{}
	for _, file := range archive.File {
		opened, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(opened)
		opened.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		parts[file.Name] = string(content)
	}
	return parts
}

// A chart in an exported deck is a chart, not the drawing of one: the numbers
// travel with it and open in Excel.
func TestPlottedBlocksExportAsEditableCharts(t *testing.T) {
	parts := renderedParts(t, chartDeck(Block{Kind: BlockColumns, Emphasis: 3, Items: []Item{
		{Label: "1분기", Value: "1,180"}, {Label: "2분기", Value: "1,240"}, {Label: "3분기", Value: "1,390"}}}))

	chart, ok := parts["ppt/charts/chart1.xml"]
	if !ok {
		t.Fatalf("the deck carries no chart part: %v", partNames(parts))
	}
	for _, wanted := range []string{`<c:barDir val="col"/>`, `<c:v>1180</c:v>`, `<c:v>1분기</c:v>`,
		`formatCode="#,##0"`, `<c:externalData`} {
		if !strings.Contains(chart, wanted) {
			t.Errorf("the chart part is missing %s", wanted)
		}
	}
	if _, ok := parts["ppt/embeddings/ptiumChart1.xlsx"]; !ok {
		t.Error("a chart nobody can open the numbers of is a picture of a chart")
	}
	if rels := parts["ppt/charts/_rels/chart1.xml.rels"]; !strings.Contains(rels, "ptiumChart1.xlsx") {
		t.Errorf("the chart does not point at its workbook: %s", rels)
	}
	slide := parts["ppt/slides/slide2.xml"]
	if !strings.Contains(slide, `<a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart">`) {
		t.Error("the slide holds no chart frame")
	}
	rels := parts["ppt/slides/_rels/slide2.xml.rels"]
	if !strings.Contains(rels, `Type="`+relationshipNamespace+`/chart"`) || !strings.Contains(rels, "chart1.xml") {
		t.Errorf("the slide does not relate to its chart: %s", rels)
	}
	if types := parts["[Content_Types].xml"]; !strings.Contains(types, "/ppt/charts/chart1.xml") ||
		!strings.Contains(types, `Extension="xlsx"`) {
		t.Errorf("PowerPoint cannot read a part whose type is undeclared: %s", types)
	}

	// The workbook is a package Excel can open, with the categories down the
	// first column and the figures beside them.
	book := parts["ppt/embeddings/ptiumChart1.xlsx"]
	archive, err := zip.NewReader(strings.NewReader(book), int64(len(book)))
	if err != nil {
		t.Fatalf("the workbook is not a package: %v", err)
	}
	sheet := ""
	for _, file := range archive.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		opened, _ := file.Open()
		content, _ := io.ReadAll(opened)
		opened.Close()
		sheet = string(content)
	}
	for _, wanted := range []string{`<t xml:space="preserve">1분기</t>`, `<c r="B2"><v>1180</v></c>`} {
		if !strings.Contains(sheet, wanted) {
			t.Errorf("the workbook is missing %s:\n%s", wanted, sheet)
		}
	}
}

// A chart whose marks say something a number format cannot reproduce keeps the
// drawing, because a label that comes out saying the wrong thing is worse than
// a picture.
func TestUnreproducibleLabelsKeepTheDrawing(t *testing.T) {
	parts := renderedParts(t, chartDeck(Block{Kind: BlockColumns, Items: []Item{
		{Label: "1분기", Value: "1,180억원"}, {Label: "2분기", Value: "12%"}}}))
	if _, ok := parts["ppt/charts/chart1.xml"]; ok {
		t.Fatalf("two units cannot share one number format, so the drawing should have stayed")
	}
	if slide := parts["ppt/slides/slide2.xml"]; !strings.Contains(slide, "1,180억원") {
		t.Error("the drawing lost the value it was labelling")
	}
}

// A unit the drawing writes on every mark travels into the chart's number
// format, so the file says what the preview said.
func TestChartNumberFormatCarriesTheUnit(t *testing.T) {
	format, ok := chartNumberFormat([]string{"42%", "33%", "25%"}, []float64{42, 33, 25})
	if !ok || format != `0"%"` {
		t.Fatalf("format = %q, ok = %v", format, ok)
	}
	if format, ok := chartNumberFormat([]string{"9.8", "12.4"}, []float64{9.8, 12.4}); !ok || format != "0.#" {
		t.Fatalf("decimal format = %q, ok = %v", format, ok)
	}
	if _, ok := chartNumberFormat([]string{"많음", "적음"}, []float64{0, 0}); ok {
		t.Fatal("words are not a number format")
	}
}

func partNames(parts map[string]string) []string {
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	return names
}

// A template is often a deck someone already had. The charts of its slides go
// with those slides: nothing points at them once the deck is rebuilt, and dead
// parts would ride along in every file made from that template.
func TestATemplatesOwnChartsDoNotRideAlong(t *testing.T) {
	template, err := BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	withChart, err := RenderBytes(template, chartDeck(Block{Kind: BlockColumns, Items: []Item{
		{Label: "1분기", Value: "1180"}, {Label: "2분기", Value: "1240"}}}))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// That file is now the template for another deck, which plots nothing.
	pkg, manifest, err := AnalyzeBytes(withChart)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	plain, err := Render(pkg, manifest, Deck{Language: "ko", Title: "글만", Slides: []Slide{
		{LayoutID: manifest.Layouts[0].ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "글만"}}}}}})
	if err != nil {
		t.Fatalf("render on top: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(plain), int64(len(plain)))
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, "ppt/charts/") || strings.HasPrefix(file.Name, "ppt/embeddings/") {
			t.Errorf("%s belonged to a slide that is gone", file.Name)
		}
	}
}

// A drawing with no alternative text is nothing at all to a screen reader, and
// PowerPoint's own accessibility check reports it as an error — which is enough
// to keep a deck out of a public-sector filing.
func TestEveryDrawnObjectSaysWhatItShows(t *testing.T) {
	deck := chartDeck(Block{Kind: BlockColumns, Heading: "분기 매출", Items: []Item{
		{Label: "1분기", Value: "1,180"}, {Label: "2분기", Value: "1,240"}}})
	deck.Slides = append(deck.Slides, Slide{LayoutID: "content",
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "채널별"}}},
		Blocks: map[string]Block{SlotBody: {Kind: BlockTable, Heading: "채널별 매출",
			Columns: []string{"항목", "1분기"}, Rows: [][]string{{"매출", "120억"}, {"이익", "12억"}}}}})
	deck.Slides = append(deck.Slides, Slide{LayoutID: "content",
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "지표"}}},
		Blocks: map[string]Block{SlotBody: {Kind: BlockKPI, Items: []Item{
			{Label: "전환 대상", Value: "42개"}, {Label: "절감", Value: "18%"}}}}})
	parts := renderedParts(t, deck)

	chartFrame := parts["ppt/slides/slide2.xml"]
	if !strings.Contains(chartFrame, `name="Chart" descr="세로막대 차트. 분기 매출. 1분기 1,180; 2분기 1,240"`) {
		t.Errorf("the chart does not say what it shows:\n%s", chartFrame)
	}
	table := parts["ppt/slides/slide3.xml"]
	if !strings.Contains(table, `name="Table" descr="표. 채널별 매출. 항목 / 1분기; 매출 / 120억; 이익 / 12억 (2열 3행)"`) {
		t.Errorf("the table does not say what it shows:\n%s", table)
	}
	figures := parts["ppt/slides/slide4.xml"]
	if !strings.Contains(figures, `descr="핵심 지표. 전환 대상 42개; 절감 18%"`) {
		t.Errorf("the figures do not say what they show:\n%s", figures)
	}
	// A component whose body left the group says it once, not twice — and the
	// group it left behind is described by the words it still holds, because an
	// object with no alternative text is the error this all exists to avoid.
	if strings.Count(chartFrame, "세로막대 차트. 분기 매출. 1분기") != 1 {
		t.Errorf("the chart's description is repeated:\n%s", chartFrame)
	}
	if !strings.Contains(chartFrame, `name="Column chart" descr="분기 매출"`) {
		t.Errorf("the heading left beside the chart says nothing:\n%s", chartFrame)
	}
}

// Alt text is read aloud in the deck's language, not in Ptium's.
func TestAlternativeTextFollowsTheDeckLanguage(t *testing.T) {
	deck := chartDeck(Block{Kind: BlockColumns, Heading: "Quarterly revenue", Items: []Item{
		{Label: "Q1", Value: "1,180"}, {Label: "Q2", Value: "1,240"}}})
	deck.Language = "en"
	parts := renderedParts(t, deck)
	if slide := parts["ppt/slides/slide2.xml"]; !strings.Contains(slide, `descr="column chart. Quarterly revenue.`) {
		t.Errorf("the chart is described in the wrong language:\n%s", slide)
	}
}
