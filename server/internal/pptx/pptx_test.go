package pptx

import (
	"encoding/xml"
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
