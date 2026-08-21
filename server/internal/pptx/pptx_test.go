package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"
)

func buildTemplate(t *testing.T, palette string) ([]byte, *Package, Manifest) {
	t.Helper()
	data, err := BuiltinTemplate(palette)
	if err != nil {
		t.Fatalf("BuiltinTemplate: %v", err)
	}
	pkg, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("AnalyzeBytes: %v", err)
	}
	return data, pkg, manifest
}

func TestBuiltinTemplateAnalysis(t *testing.T) {
	for _, key := range BuiltinDesignKeys() {
		_, _, manifest := buildTemplate(t, key)
		if manifest.SlideWidth != slideWidth || manifest.SlideHeight != slideHeight {
			t.Fatalf("%s: unexpected slide size %dx%d", key, manifest.SlideWidth, manifest.SlideHeight)
		}
		if manifest.AspectRatio != "16:9" {
			t.Fatalf("%s: aspect ratio = %q", key, manifest.AspectRatio)
		}
		if len(manifest.Layouts) != 10 {
			t.Fatalf("%s: expected 10 layouts, got %d", key, len(manifest.Layouts))
		}
		roles := map[string]bool{}
		for _, layout := range manifest.Layouts {
			roles[layout.Role] = true
		}
		for _, required := range []string{RoleTitle, RoleSection, RoleContent, RoleTwoContent, RoleComparison, RoleQuote, RolePicture, RoleClosing, RoleBlank} {
			if !roles[required] {
				t.Fatalf("%s: template is missing a %s layout", key, required)
			}
		}
		if manifest.TitleLayout == "" || manifest.DefaultLayout == "" || manifest.ClosingLayout == "" {
			t.Fatalf("%s: manifest did not resolve its key layouts: %+v", key, manifest)
		}
		if len(manifest.Warnings) != 0 {
			t.Fatalf("%s: unexpected warnings %v", key, manifest.Warnings)
		}
	}
}

func TestBuiltinTemplateLayoutSlots(t *testing.T) {
	_, _, manifest := buildTemplate(t, "slate-classic")
	content, ok := manifest.LayoutForRole(RoleContent)
	if !ok {
		t.Fatal("no content layout")
	}
	title, hasTitle := content.Slot(SlotTitle)
	if !hasTitle {
		t.Fatalf("content layout has no title slot: %+v", content.Placeholders)
	}
	if title.MaxChars < 10 || title.MaxLines < 1 {
		t.Fatalf("title capacity looks wrong: %+v", title)
	}
	body, hasBody := content.Slot(SlotBody)
	if !hasBody {
		t.Fatal("content layout has no body slot")
	}
	if body.MaxChars < 100 {
		t.Fatalf("body capacity looks too small: %+v", body)
	}
	if body.FontSize != 1800 {
		t.Fatalf("body font size = %d, want 1800", body.FontSize)
	}

	comparison, ok := manifest.LayoutForRole(RoleComparison)
	if !ok {
		t.Fatal("no comparison layout")
	}
	if len(comparison.BodySlots()) != 4 {
		t.Fatalf("comparison layout should expose four body slots, got %d", len(comparison.BodySlots()))
	}
	regions := map[string]bool{}
	for _, slot := range comparison.BodySlots() {
		regions[slot.Region] = true
	}
	if !regions["left-middle"] && !regions["left-top"] && !regions["left-bottom"] {
		t.Fatalf("comparison layout lost its left column: %+v", regions)
	}

	picture, ok := manifest.LayoutForRole(RolePicture)
	if !ok {
		t.Fatal("no picture layout")
	}
	if _, hasPicture := picture.Slot(SlotPicture); !hasPicture {
		t.Fatalf("picture layout has no picture slot: %+v", picture.Placeholders)
	}
}

func TestRenderProducesValidPackage(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	titleLayout, _ := manifest.Layout(manifest.TitleLayout)
	contentLayout, _ := manifest.Layout(manifest.DefaultLayout)
	deck := Deck{
		Title:    "2026 성장 전략",
		Author:   "Ptium",
		Language: "ko",
		Slides: []Slide{
			{LayoutID: titleLayout.ID, Fields: map[string][]Paragraph{
				SlotTitle:    {{Text: "2026 성장 전략"}},
				SlotSubtitle: {{Text: "국내 시장 재진입 로드맵"}},
			}, Notes: "인사와 함께 오늘의 목적을 한 문장으로 밝힙니다."},
			{LayoutID: contentLayout.ID, Fields: map[string][]Paragraph{
				SlotTitle: {{Text: "핵심 진단"}},
				SlotBody: {
					{Text: "기존 채널의 성장률이 3분기 연속 둔화"},
					{Text: "이탈 고객의 62%가 온보딩 단계에서 발생", Level: 1},
					{Text: "신규 세그먼트에서만 두 자릿수 성장 유지"},
				},
			}, Notes: "숫자보다 원인에 무게를 둡니다."},
		},
	}
	rendered, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	result, err := Open(rendered)
	if err != nil {
		t.Fatalf("rendered package does not open: %v", err)
	}

	for _, required := range []string{
		"[Content_Types].xml", "ppt/presentation.xml", "ppt/slides/slide1.xml", "ppt/slides/slide2.xml",
		"ppt/slides/_rels/slide1.xml.rels", "ppt/notesSlides/notesSlide1.xml", "ppt/notesMasters/notesMaster1.xml",
	} {
		if _, ok := result.Part(required); !ok {
			t.Fatalf("rendered package is missing %s", required)
		}
	}
	if _, ok := result.Part("ppt/slides/slide3.xml"); ok {
		t.Fatal("rendered package kept a stale slide")
	}

	// Every part must be well-formed XML.
	for _, name := range result.Names() {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		content, _ := result.Part(name)
		if err := xml.Unmarshal(content, new(struct {
			XMLName xml.Name
		})); err != nil {
			t.Fatalf("%s is not well-formed: %v", name, err)
		}
	}

	presentation, _ := result.Text("ppt/presentation.xml")
	if strings.Count(presentation, "<p:sldId ") != 2 {
		t.Fatalf("presentation.xml should list two slides:\n%s", presentation)
	}
	if !strings.Contains(presentation, "<p:notesMasterIdLst>") {
		t.Fatal("presentation.xml is missing the notes master list")
	}

	slide1, _ := result.Text("ppt/slides/slide1.xml")
	if !strings.Contains(slide1, "2026 성장 전략") || !strings.Contains(slide1, "국내 시장 재진입 로드맵") {
		t.Fatalf("slide 1 lost its content:\n%s", slide1)
	}
	if !strings.Contains(slide1, `<p:ph type="ctrTitle"/>`) {
		t.Fatalf("slide 1 is not bound to the layout placeholders:\n%s", slide1)
	}
	slide2, _ := result.Text("ppt/slides/slide2.xml")
	if !strings.Contains(slide2, `<a:pPr lvl="1"/>`) {
		t.Fatalf("slide 2 lost its sub-bullet level:\n%s", slide2)
	}
	if !strings.Contains(slide2, "62%") {
		t.Fatalf("slide 2 lost body text:\n%s", slide2)
	}

	rels, _ := result.Text("ppt/slides/_rels/slide1.xml.rels")
	if !strings.Contains(rels, "../slideLayouts/") {
		t.Fatalf("slide 1 is not linked to a layout: %s", rels)
	}
	if !strings.Contains(rels, "../notesSlides/notesSlide1.xml") {
		t.Fatalf("slide 1 is not linked to its notes: %s", rels)
	}

	contentTypes, _ := result.Text("[Content_Types].xml")
	for _, required := range []string{
		`PartName="/ppt/slides/slide1.xml"`, `PartName="/ppt/notesSlides/notesSlide1.xml"`,
		`PartName="/ppt/notesMasters/notesMaster1.xml"`, "presentationml.presentation.main+xml",
	} {
		if !strings.Contains(contentTypes, required) {
			t.Fatalf("content types missing %s:\n%s", required, contentTypes)
		}
	}
	if strings.Contains(contentTypes, "slide3.xml") {
		t.Fatal("content types reference a part that no longer exists")
	}
}

// A deck of twenty slides that nobody can refer to by number is harder to talk
// about, so the export carries the page number the design has a place for — and
// only where the design has one.
func TestRenderNumbersEverySlideButTheCover(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	titleLayout, _ := manifest.Layout(manifest.TitleLayout)
	contentLayout, _ := manifest.Layout(manifest.DefaultLayout)
	if contentLayout.SlideNumber == nil {
		t.Fatal("the shipped design declares no place for a page number")
	}
	deck := Deck{Title: "번호", Language: "ko", Slides: []Slide{
		{LayoutID: titleLayout.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "표지"}}}},
		{LayoutID: contentLayout.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "둘째"}}, SlotBody: {{Text: "줄"}}}},
		{LayoutID: contentLayout.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "셋째"}}, SlotBody: {{Text: "줄"}}}},
	}}
	rendered, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	result, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	cover, _ := result.Text("ppt/slides/slide1.xml")
	if strings.Contains(cover, `type="slidenum"`) {
		t.Fatalf("the cover carries a page number:\n%s", cover)
	}
	for _, part := range []string{"ppt/slides/slide2.xml", "ppt/slides/slide3.xml"} {
		body, _ := result.Text(part)
		if !strings.Contains(body, `type="slidenum"`) {
			t.Fatalf("%s has no page number:\n%s", part, body)
		}
		// The field is what makes the number follow a slide someone reorders in
		// PowerPoint; a literal digit would be wrong the moment they did.
		if !strings.Contains(body, `<a:fld id=`) {
			t.Fatalf("%s numbers itself with plain text rather than a field", part)
		}
	}
	// And the preview draws it in the same place, so the screen and the file agree.
	numbered := deck.Slides[1]
	numbered.Number = 2
	svg := PreviewSVG(manifest, contentLayout, numbered, PreviewOptions{Width: 640})
	if !strings.Contains(svg, ">2</text>") {
		t.Fatalf("the preview does not draw the page number:\n%s", svg)
	}
}

// A table someone cannot type into is a picture of a table, so the export holds
// a real one — beside the caption rather than inside its group, because
// PowerPoint will not let anyone into a table it finds grouped.
func TestRenderExportsATableAsATable(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	deck := Deck{Title: "표", Language: "ko", Slides: []Slide{{
		LayoutID: layout.ID,
		Fields:   map[string][]Paragraph{SlotTitle: {{Text: "분기 실적"}}},
		Blocks: map[string]Block{SlotBody: {Kind: BlockTable, Caption: "분기 실적",
			Columns: []string{"항목", "1분기", "2분기"},
			Rows:    [][]string{{"매출", "120억", "140억"}, {"영업이익", "12억", "18억"}}}},
	}}}
	rendered, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	result, _ := Open(rendered)
	slide, _ := result.Text("ppt/slides/slide1.xml")
	if !strings.Contains(slide, "<a:tbl>") {
		t.Fatalf("the table was drawn as shapes rather than exported as a table:\n%s", slide)
	}
	// Three columns and three rows: the header and the two rows of figures.
	if got := strings.Count(slide, "<a:gridCol"); got != 3 {
		t.Fatalf("the table has %d columns, want 3", got)
	}
	if got := strings.Count(slide, "<a:tr "); got != 3 {
		t.Fatalf("the table has %d rows, want 3", got)
	}
	for _, cell := range []string{"항목", "매출", "140억", "영업이익"} {
		if !strings.Contains(slide, ">"+cell+"<") {
			t.Fatalf("the table lost the cell %q:\n%s", cell, slide)
		}
	}
	// The caption is still a shape, and the table is not inside its group.
	frame := strings.Index(slide, "<p:graphicFrame>")
	group := strings.Index(slide, "</p:grpSp>")
	if frame < 0 || group < 0 || frame < group {
		t.Fatalf("the table sits inside the component group:\n%s", slide)
	}
	// And the preview still draws the same table, so the screen and the file agree.
	svg := PreviewSVG(manifest, layout, deck.Slides[0], PreviewOptions{Width: 900})
	for _, cell := range []string{"항목", "140억"} {
		if !strings.Contains(svg, cell) {
			t.Fatalf("the preview lost %q", cell)
		}
	}
}

func TestRenderIsRepeatableFromOneTemplate(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "ivory-editorial")
	deck := Deck{Title: "A", Slides: []Slide{{LayoutID: manifest.DefaultLayout, Fields: map[string][]Paragraph{SlotTitle: {{Text: "첫 번째"}}}}}}
	first, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	deck.Slides[0].Fields[SlotTitle] = []Paragraph{{Text: "두 번째"}}
	second, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	firstPkg, _ := Open(first)
	secondPkg, _ := Open(second)
	firstSlide, _ := firstPkg.Text("ppt/slides/slide1.xml")
	secondSlide, _ := secondPkg.Text("ppt/slides/slide1.xml")
	if !strings.Contains(firstSlide, "첫 번째") || !strings.Contains(secondSlide, "두 번째") {
		t.Fatal("rendering mutated the shared template package")
	}
	if strings.Contains(secondSlide, "첫 번째") {
		t.Fatal("the second render leaked content from the first")
	}
}

func TestRenderEscapesHostileText(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "slate-classic")
	deck := Deck{Title: "x", Slides: []Slide{{LayoutID: manifest.DefaultLayout, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: `</a:t></a:r></p:txBody><evil attr="1">`}},
		SlotBody:  {{Text: "A & B < C\u0000"}},
	}}}}
	rendered, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	result, err := Open(rendered)
	if err != nil {
		t.Fatalf("hostile text broke the package: %v", err)
	}
	slide, _ := result.Text("ppt/slides/slide1.xml")
	if strings.Contains(slide, "<evil") {
		t.Fatalf("markup injection was not escaped:\n%s", slide)
	}
	if strings.Contains(slide, "\u0000") {
		t.Fatal("control characters survived into the XML")
	}
	if err := xml.Unmarshal([]byte(slide), new(struct{ XMLName xml.Name })); err != nil {
		t.Fatalf("slide is not well-formed: %v", err)
	}
}

func TestAutofitShrinksOverflowingText(t *testing.T) {
	placeholder := Placeholder{Slot: SlotBody, Kind: "text", MaxChars: 100, MaxLines: 5, FontSize: 1800}
	if scale, _ := autofit(placeholder, []Paragraph{{Text: strings.Repeat("가", 40)}}); scale != 100 {
		t.Fatalf("short text should not be scaled, got %v", scale)
	}
	scale, reduction := autofit(placeholder, []Paragraph{{Text: strings.Repeat("가", 400)}})
	if scale >= 100 || scale < 40 {
		t.Fatalf("overflowing text should shrink into range, got %v", scale)
	}
	if reduction == 0 {
		t.Fatalf("heavy overflow should also reduce line spacing, got %d", reduction)
	}
}

func TestOpenRejectsNonPackages(t *testing.T) {
	if _, err := Open(nil); err == nil {
		t.Fatal("empty input should fail")
	}
	if _, err := Open([]byte("not a zip file")); err == nil {
		t.Fatal("plain text should fail")
	}
}

func TestPreviewSVGUsesTemplateGeometry(t *testing.T) {
	_, _, manifest := buildTemplate(t, "plum-rail")
	layout, _ := manifest.Layout(manifest.DefaultLayout)
	svg := PreviewSVG(manifest, layout, Slide{Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "제목 & 요약"}},
		SlotBody:  {{Text: "첫 번째 요점"}, {Text: "두 번째 요점"}},
	}}, PreviewOptions{Width: 640})
	if !strings.HasPrefix(svg, "<svg") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("preview is not an SVG document: %.80s", svg)
	}
	// The canvas keeps the template's aspect ratio but is emitted in pixels so
	// font sizes stay in a range every SVG renderer handles.
	if !strings.Contains(svg, `viewBox="0 0 640 360"`) {
		t.Fatalf("preview does not use the template canvas:\n%.200s", svg)
	}
	if strings.Contains(svg, "font-size=\"1") && strings.Contains(svg, "00000") {
		t.Fatalf("preview still emits EMU font sizes:\n%.400s", svg)
	}
	if !strings.Contains(svg, "제목 &amp; 요약") {
		t.Fatalf("preview did not escape or render the title:\n%s", svg)
	}
	if err := xml.Unmarshal([]byte(svg), new(struct{ XMLName xml.Name })); err != nil {
		t.Fatalf("preview is not well-formed: %v", err)
	}
}

func TestRelativePath(t *testing.T) {
	cases := []struct{ from, to, want string }{
		{"ppt/slides/slide1.xml", "ppt/slideLayouts/slideLayout2.xml", "../slideLayouts/slideLayout2.xml"},
		{"ppt/notesSlides/notesSlide1.xml", "ppt/notesMasters/notesMaster1.xml", "../notesMasters/notesMaster1.xml"},
		{"ppt/presentation.xml", "ppt/notesMasters/notesMaster1.xml", "notesMasters/notesMaster1.xml"},
		{"ppt/slides/slide1.xml", "ppt/slides/slide2.xml", "slide2.xml"},
	}
	for _, testCase := range cases {
		if got := relativePath(testCase.from, testCase.to); got != testCase.want {
			t.Fatalf("relativePath(%q,%q) = %q, want %q", testCase.from, testCase.to, got, testCase.want)
		}
	}
}

func TestLayoutSelectionAvoidsVerticalAndTitleOnlyLayouts(t *testing.T) {
	horizontal := Layout{ID: "content", Role: RoleContent, Type: "obj", Placeholders: []Placeholder{
		{Slot: SlotTitle, Kind: "text"}, {Slot: SlotBody, Kind: "text"},
	}}
	vertical := Layout{ID: "vertical", Role: RoleContent, Type: "vertTx", Placeholders: []Placeholder{
		{Slot: SlotTitle, Kind: "text"}, {Slot: SlotBody, Kind: "text", Vertical: true},
	}}
	titleOnly := Layout{ID: "title-only", Role: RoleContent, Type: "titleOnly", Placeholders: []Placeholder{
		{Slot: SlotTitle, Kind: "text"},
	}}
	// Declaration order puts the awkward layouts first on purpose.
	manifest := Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Layouts: []Layout{vertical, titleOnly, horizontal}}
	manifest.finalize()
	layout, ok := manifest.LayoutForRole(RoleContent)
	if !ok || layout.ID != "content" {
		t.Fatalf("content role resolved to %q, want the horizontal layout", layout.ID)
	}
	if manifest.DefaultLayout != "content" {
		t.Fatalf("default layout = %q, want content", manifest.DefaultLayout)
	}
}

// A one-pixel PNG, enough to prove the bytes travel into the package.
var onePixelPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00,
	0x1F, 0x15, 0xC4, 0x89,
	0x00, 0x00, 0x00, 0x0A, 'I', 'D', 'A', 'T',
	0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
	0x0D, 0x0A, 0x2D, 0xB4,
	0x00, 0x00, 0x00, 0x00, 'I', 'E', 'N', 'D', 0xAE, 0x42, 0x60, 0x82,
}

func TestRenderPlacesAPictureInThePackage(t *testing.T) {
	data, err := BuiltinTemplate("slate-classic")
	if err != nil {
		t.Fatalf("BuiltinTemplate: %v", err)
	}
	pkg, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("AnalyzeBytes: %v", err)
	}
	layout, ok := manifest.LayoutForRole(RoleContent)
	if !ok {
		t.Fatal("the built-in template should offer a content layout")
	}
	slot := ""
	for _, placeholder := range layout.BodySlots() {
		slot = placeholder.Slot
		break
	}
	if slot == "" {
		t.Fatal("the content layout should offer a body region")
	}
	rendered, err := Render(pkg, manifest, Deck{
		Title: "그림", Language: "ko",
		Slides: []Slide{{
			LayoutID: layout.ID,
			Fields:   map[string][]Paragraph{SlotTitle: {{Text: "브랜드"}}},
			Pictures: map[string]Picture{slot: {Data: onePixelPNG, ContentType: "image/png",
				Width: 1600, Height: 400, Caption: "로고"}},
		}},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	result, err := Open(rendered)
	if err != nil {
		t.Fatalf("the rendered package does not open: %v", err)
	}

	// The bytes are in the package, once.
	media := result.NamesUnder("ppt/media/ptium")
	if len(media) != 1 {
		t.Fatalf("media parts = %v", media)
	}
	if stored, _ := result.Part(media[0]); !bytes.Equal(stored, onePixelPNG) {
		t.Fatal("the stored image is not the image that was given")
	}
	// The slide refers to it through a relationship, and the relationship points
	// at the part that exists.
	slide, _ := result.Text("ppt/slides/slide1.xml")
	if !strings.Contains(slide, "<p:pic>") || !strings.Contains(slide, `descr="로고"`) {
		t.Fatalf("the slide does not draw the picture:\n%s", slide)
	}
	embed := regexp.MustCompile(`r:embed="(rId\d+)"`).FindStringSubmatch(slide)
	if embed == nil {
		t.Fatalf("the picture has no relationship reference:\n%s", slide)
	}
	target, ok := result.RelationshipByID("ppt/slides/slide1.xml", embed[1])
	if !ok || target != media[0] {
		t.Fatalf("relationship %s resolves to %q, want %q", embed[1], target, media[0])
	}
	// A wide image in a narrower frame is cropped rather than squashed.
	if !strings.Contains(slide, "<a:srcRect") {
		t.Fatalf("a picture whose aspect differs from its frame must be cropped:\n%s", slide)
	}
	// PowerPoint refuses a package whose image extension is not declared.
	types, _ := result.Text("[Content_Types].xml")
	if !strings.Contains(types, `Extension="png"`) {
		t.Fatalf("the png content type was not declared:\n%s", types)
	}
	// The slot holds the picture instead of prose, so nothing is drawn twice.
	// The page number is a shape too, and it is not one of the slide's regions.
	shapes := strings.Count(slide, "<p:sp>") - strings.Count(slide, `type="sldNum"`)
	if shapes > 1 {
		t.Fatalf("the picture's slot should hold no text shape:\n%s", slide)
	}
}

// A deck someone already has is read for its argument, not its artwork: the
// words come across and are recompiled into whatever design they land in.
func TestReadDeckCarriesTheWordsAndSaysWhatItLeft(t *testing.T) {
	_, pkg, manifest := buildTemplate(t, "plum-rail")
	titleLayout, _ := manifest.Layout(manifest.TitleLayout)
	contentLayout, _ := manifest.Layout(manifest.DefaultLayout)
	deck := Deck{Title: "지난 분기 보고", Language: "ko", Slides: []Slide{
		{LayoutID: titleLayout.ID, Fields: map[string][]Paragraph{
			SlotTitle:    {{Text: "2025년 4분기 영업 실적"}},
			SlotSubtitle: {{Text: "영업기획팀"}},
		}, Notes: "결론을 먼저 말합니다."},
		{LayoutID: contentLayout.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "실적 요약"}},
			SlotBody:  {{Text: "매출 1,240억"}, {Text: "신규 채널이 절반", Level: 1}},
		}},
		{LayoutID: contentLayout.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "채널별 매출"}}},
			Blocks: map[string]Block{SlotBody: {Kind: BlockTable,
				Columns: []string{"채널", "3분기", "4분기"},
				Rows:    [][]string{{"직영", "420억", "480억"}}}}},
		{LayoutID: contentLayout.ID, Fields: map[string][]Paragraph{SlotTitle: {{Text: "신규 매장"}}},
			Pictures: map[string]Picture{SlotBody: {Data: onePixelPNG, ContentType: "image/png", Width: 8, Height: 8}}},
	}}
	rendered, err := Render(pkg, manifest, deck)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	stored, err := Open(rendered)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	read := ReadDeck(stored)
	if len(read.Slides) != 4 {
		t.Fatalf("read %d slides, want 4", len(read.Slides))
	}
	if read.Slides[0].Title != "2025년 4분기 영업 실적" {
		t.Fatalf("first title = %q", read.Slides[0].Title)
	}
	if read.Slides[0].Lead != "영업기획팀" {
		t.Fatalf("the subtitle did not come across: %q", read.Slides[0].Lead)
	}
	if read.Slides[0].Notes == "" {
		t.Fatal("the speaker notes did not come across")
	}
	// The depth of a point is part of the argument.
	if len(read.Slides[1].Bullets) != 2 || read.Slides[1].Bullets[1].Level != 1 {
		t.Fatalf("the points came back as %+v", read.Slides[1].Bullets)
	}
	// A photograph is the author's; where it sat is not. The bytes come across so
	// the picture can go into the region the new design keeps for one.
	if len(read.Slides[3].Pictures) != 1 || len(read.Slides[3].Pictures[0].Data) == 0 {
		t.Fatalf("the picture did not come across: %+v", read.Slides[3].Pictures)
	}
	if read.Slides[3].Pictures[0].Area == 0 {
		t.Fatal("the picture came across without how much of the slide it covered")
	}
	// A table is words in a grid, so it comes across whole.
	if len(read.Slides[2].Tables) != 1 {
		t.Fatalf("the table did not come across: %+v", read.Slides[2])
	}
	if got := read.Slides[2].Tables[0][0]; len(got) != 3 || got[0] != "채널" {
		t.Fatalf("the table's header reads %v", got)
	}
}

// Without a role of its own, an imported slide falls to the deck's position
// rules and the last one becomes a closing page — which in most designs holds a
// line, so its points are dropped. Microsoft's own content layout is type "obj".
func TestImportedLayoutsKeepTheirKind(t *testing.T) {
	cases := []struct{ layoutType, name, want string }{
		{"obj", "Title and Content", RoleContent},
		{"tx", "Title and Text", RoleContent},
		{"", "Title and Content", RoleContent},
		{"", "제목 및 내용", RoleContent},
		{"title", "Title Slide", RoleTitle},
		{"secHead", "Section Header", RoleSection},
		{"twoObj", "Two Content", RoleTwoContent},
		{"picTx", "Picture with Caption", RolePicture},
		{"blank", "Blank", RoleBlank},
		{"titleOnly", "Title Only", ""},
	}
	for _, testCase := range cases {
		if got := roleForLayoutType(testCase.layoutType, testCase.name); got != testCase.want {
			t.Errorf("roleForLayoutType(%q, %q) = %q, want %q",
				testCase.layoutType, testCase.name, got, testCase.want)
		}
	}
}

// A lead placed in a body region is the slide's own sentence. A placeholder
// inherits its bullets from the template, so the lead has to say it wants none
// — otherwise it arrives as the first point, which is not what it is.
func TestALeadIsNotDrawnAsAPoint(t *testing.T) {
	data, err := BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	pkg, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	layout, _ := manifest.LayoutForRole(RoleContent)
	rendered, err := Render(pkg, manifest, Deck{Language: "ko", Title: "리드", Slides: []Slide{{
		LayoutID: layout.ID,
		Fields: map[string][]Paragraph{
			SlotTitle: {{Text: "요점"}},
			SlotBody:  {{Text: "이 장의 한 줄 요약입니다", Lead: true}, {Text: "첫 번째 요점"}},
		},
	}}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(rendered), int64(len(rendered)))
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	slide := ""
	for _, file := range archive.File {
		if file.Name != "ppt/slides/slide1.xml" {
			continue
		}
		opened, _ := file.Open()
		content, _ := io.ReadAll(opened)
		opened.Close()
		slide = string(content)
	}
	if !strings.Contains(slide, `<a:pPr marL="0" indent="0"><a:spcAft><a:spcPts val="600"/></a:spcAft><a:buNone/></a:pPr><a:r>`) {
		t.Errorf("the lead carries no paragraph properties of its own:\n%s", slide)
	}
	if strings.Count(slide, "<a:buNone/>") != 1 {
		t.Errorf("only the lead asks for no bullet:\n%s", slide)
	}
}
