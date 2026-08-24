package pptx

import (
	"strings"
	"testing"
)

func regionFixture() (Manifest, Layout) {
	title := Placeholder{Slot: SlotTitle, Kind: "text", Type: "title",
		X: 800000, Y: 500000, Width: 9000000, Height: 900000, FontSize: 3200, MaxChars: 40, MaxLines: 2, LineEm: 20}
	body := Placeholder{Slot: SlotBody, Kind: "text", Type: "body",
		X: 800000, Y: 1800000, Width: 4000000, Height: 2400000, FontSize: 1800, MaxChars: 90, MaxLines: 6, LineEm: 20}
	side := Placeholder{Slot: "body2", Kind: "text", Type: "body",
		X: 5200000, Y: 1800000, Width: 4000000, Height: 2400000, FontSize: 1800, MaxChars: 90, MaxLines: 6, LineEm: 20}
	layout := Layout{ID: "content", Name: "Content", Role: RoleContent, Background: "FFFFFF",
		Placeholders: []Placeholder{title, body, side}}
	manifest := Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme:   Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111", "accent1": "1E6FFF"}},
		Layouts: []Layout{layout}}
	return manifest, layout
}

func TestSlideRegionsDescribesWhatEachRegionHolds(t *testing.T) {
	_, layout := regionFixture()
	slide := Slide{LayoutID: "content",
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "전환은 지금 결정해야 합니다"}},
			SlotBody: {{Text: "첫 번째 근거"}, {Text: "그 증거", Level: 1}}},
		Blocks: map[string]Block{"body2": {Kind: BlockKPI, Items: []Item{{Label: "대상", Value: "42개"}}}},
	}
	regions := SlideRegions(layout, slide)
	if len(regions) != 3 {
		t.Fatalf("every region of the layout is reported, got %d", len(regions))
	}
	byslot := map[string]Region{}
	for _, region := range regions {
		byslot[region.Slot] = region
	}
	if byslot[SlotTitle].Kind != RegionText || byslot[SlotTitle].Text() != "전환은 지금 결정해야 합니다" {
		t.Fatalf("title region = %+v", byslot[SlotTitle])
	}
	// A sub-bullet comes back indented, so editing the text and writing it back
	// keeps the level it was written at.
	if body := byslot[SlotBody].Text(); body != "첫 번째 근거\n  그 증거" {
		t.Fatalf("body text = %q", body)
	}
	if byslot["body2"].Kind != RegionComponent || byslot["body2"].Block == nil ||
		byslot["body2"].Block.Kind != BlockKPI {
		t.Fatalf("component region = %+v", byslot["body2"])
	}
	if byslot[SlotTitle].Moved {
		t.Fatal("a region the template placed is not moved")
	}
}

func TestMovedRegionMovesEverywhereItIsDrawn(t *testing.T) {
	manifest, layout := regionFixture()
	moved := Frame{X: 600000, Y: 3800000, Width: 6000000, Height: 1600000}
	slide := Slide{LayoutID: "content",
		Fields: map[string][]Paragraph{SlotBody: {{Text: "옮겨 놓은 본문"}}},
		Frames: map[string]Frame{SlotBody: moved},
	}
	regions := SlideRegions(layout, slide)
	var body Region
	for _, region := range regions {
		if region.Slot == SlotBody {
			body = region
		}
	}
	if body.Frame != moved || !body.Moved {
		t.Fatalf("the region reports where it now draws: %+v", body)
	}
	if body.Layout.X != 800000 || body.Layout.Y != 1800000 {
		t.Fatalf("it also reports where the template put it: %+v", body.Layout)
	}

	// The exported slide and the preview have to agree with that, or the canvas
	// shows one thing and PowerPoint another.
	xml, _, _ := slideXML(layout, slide, "ko", NewDesign(manifest), nil)
	if !strings.Contains(xml, `<a:off x="600000" y="3800000"/>`) {
		t.Fatalf("the exported shape is not where the region was moved to:\n%s", xml)
	}
	svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 960})
	if !strings.Contains(svg, "옮겨 놓은 본문") {
		t.Fatal("the preview still has to draw the text")
	}
	// A wider, shorter box holds a different amount of text; autofit is told so.
	placed := slide.Place(layout.Placeholders[1])
	if placed.MaxLines >= layout.Placeholders[1].MaxLines {
		t.Fatalf("a shorter box holds fewer lines: %d", placed.MaxLines)
	}
}

func TestBareRenderDrawsOnlyTheSlideItself(t *testing.T) {
	manifest, layout := regionFixture()
	layout.Artwork = []Artwork{{Kind: "shape", X: 0, Y: 0, Width: 12192000, Height: 400000, Fill: "1E6FFF"}}
	manifest.Layouts = []Layout{layout}
	slide := Slide{LayoutID: "content", Fields: map[string][]Paragraph{SlotTitle: {{Text: "제목만"}}}}
	bare := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 960, Bare: true})
	if strings.Contains(bare, "1E6FFF") {
		t.Fatalf("a lifted region carries no template artwork:\n%s", bare)
	}
	if !strings.Contains(bare, "제목만") {
		t.Fatal("it does carry the slide's own content")
	}
}

func TestRepeatedPointIsReported(t *testing.T) {
	// Both lines come from a deck a model actually wrote: the second says the
	// first again, longer.
	slide := Slide{LayoutID: "content", Fields: map[string][]Paragraph{
		SlotBody: {
			{Text: "기존 온프레미스 유지비 증가로 비용 효율성 저하"},
			{Text: "온프레미스 유지비 급증으로 비용 효율성이 심각하게 저하됩니다"},
			{Text: "시장 변화 대응을 위한 인프라 민첩성이 절실히 필요합니다"},
		}}}
	findings := repeatedPoints(slide)
	if len(findings) != 1 || findings[0].Kind != FindingRepeat || !findings[0].Advisory {
		t.Fatalf("the restatement should be reported once, got %v", findings)
	}

	// Parallel lines are good writing, not repetition.
	parallel := Slide{LayoutID: "content", Fields: map[string][]Paragraph{
		SlotBody: {
			{Text: "매출은 전년 대비 12% 늘어 목표를 넘었습니다"},
			{Text: "비용은 전년 대비 8% 줄어 계획에 맞췄습니다"},
		}}}
	if findings := repeatedPoints(parallel); len(findings) != 0 {
		t.Fatalf("parallel lines must not be reported: %v", findings)
	}
}

func TestRegionStyleOverridesTheTemplate(t *testing.T) {
	manifest, layout := regionFixture()
	yes := true
	slide := Slide{LayoutID: "content",
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "가운데로 크게"}}},
		Styles: map[string]Style{SlotTitle: {Scale: 1.5, Color: "FF0055", Bold: &yes, Align: "center"}},
	}
	placed := slide.Place(layout.Placeholders[0])
	if placed.FontSize != 4800 || placed.Color != "FF0055" || !placed.Bold || placed.Align != "ctr" {
		t.Fatalf("the slide's own type is applied: %+v", placed)
	}
	// Bigger type holds less text, and autofit has to know.
	if placed.MaxChars >= layout.Placeholders[0].MaxChars {
		t.Fatalf("a larger size fits fewer characters: %d", placed.MaxChars)
	}

	xml, _, _ := slideXML(layout, slide, "ko", NewDesign(manifest), nil)
	for _, want := range []string{`algn="ctr"`, `sz="4800"`, `b="1"`, `val="FF0055"`} {
		if !strings.Contains(xml, want) {
			t.Fatalf("the exported slide is missing %s:\n%s", want, xml)
		}
	}
	svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 960})
	if !strings.Contains(svg, `text-anchor="middle"`) || !strings.Contains(svg, "FF0055") {
		t.Fatalf("the preview does not match the export:\n%s", svg)
	}

	// A region nobody restyled still inherits everything from the template.
	plain := Slide{LayoutID: "content", Fields: map[string][]Paragraph{SlotTitle: {{Text: "그대로"}}}}
	if plainXML, _, _ := slideXML(layout, plain, "ko", NewDesign(manifest), nil); strings.Contains(plainXML, "algn=") ||
		strings.Contains(plainXML, "sz=") || strings.Contains(plainXML, "solidFill") {
		t.Fatalf("an untouched region states nothing of its own:\n%s", plainXML)
	}
}
