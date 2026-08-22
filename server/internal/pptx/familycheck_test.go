package pptx

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestWriteFamilyPreviews dumps a cover and a content slide for each shipped
// design when PTIUM_FAMILY_PREVIEW names a directory.
func TestWriteFamilyPreviews(t *testing.T) {
	directory := os.Getenv("PTIUM_FAMILY_PREVIEW")
	if directory == "" {
		t.Skip("set PTIUM_FAMILY_PREVIEW to render design previews")
	}
	number := func(v float64) *float64 { return &v }
	for _, design := range BuiltinDesigns() {
		data, err := BuiltinTemplate(design.Key)
		if err != nil {
			t.Fatal(err)
		}
		_, manifest, err := AnalyzeBytes(data)
		if err != nil {
			t.Fatalf("%s: %v", design.Key, err)
		}
		cover, _ := manifest.Layout(manifest.TitleLayout)
		content, _ := manifest.Layout(manifest.DefaultLayout)
		coverSlide := Slide{LayoutID: cover.ID, Fields: map[string][]Paragraph{
			SlotTitle:    {{Text: "2026 성장 전략"}},
			SlotSubtitle: {{Text: design.Name + " · 대상: 경영진"}},
		}}
		// The roomiest body slot, which is the one the compiler fills. A family with
		// an eyebrow calls its first body slot "body", and drawing a chart there
		// would show the design badly for a reason the design is not guilty of.
		bodySlot := SlotBody
		widest := 0
		for _, placeholder := range content.BodySlots() {
			if area := placeholder.Width * placeholder.Height; area > widest {
				widest, bodySlot = area, placeholder.Slot
			}
		}
		contentSlide := Slide{LayoutID: content.ID,
			Fields: map[string][]Paragraph{SlotTitle: {{Text: "채널별 이탈률이 방향을 가른다"}}},
			Blocks: map[string]Block{bodySlot: {Kind: BlockColumns, Unit: "%", Emphasis: 4, Items: []Item{
				{Label: "검색", Number: number(18)}, {Label: "추천", Number: number(24)},
				{Label: "직접", Number: number(11)}, {Label: "이메일", Number: number(31)},
				{Label: "제휴", Number: number(9)}}}}}
		for name, pair := range map[string]struct {
			layout Layout
			slide  Slide
		}{"cover": {cover, coverSlide}, "content": {content, contentSlide}} {
			svg := PreviewSVG(manifest, pair.layout, pair.slide, PreviewOptions{Width: 800})
			path := fmt.Sprintf("%s/%s-%s.svg", directory, design.Key, name)
			if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Which pages carry a number is a decision about the deck: every page but the
// cover. It used to depend on whether the design had a rail — the section,
// quote and closing layouts hid the master's shapes, and the number went with
// them — so the same deck numbered its dividers in one design and not in
// another. Every shipped design now numbers the same pages.
func TestEveryDesignNumbersItsDividersAndItsClosingPage(t *testing.T) {
	for _, key := range BuiltinDesignKeys() {
		template, err := BuiltinTemplate(key)
		if err != nil {
			t.Fatalf("%s: builtin template: %v", key, err)
		}
		_, manifest, err := AnalyzeBytes(template)
		if err != nil {
			t.Fatalf("%s: analyze: %v", key, err)
		}
		for _, role := range []string{RoleSection, RoleQuote, RoleClosing, RoleContent, RoleTwoContent} {
			layout, ok := manifest.LayoutForRole(role)
			if !ok || layout.Role != role {
				continue
			}
			if layout.SlideNumber == nil {
				t.Fatalf("%s: the %q layout (%s) has nowhere to put a page number", key, role, layout.Name)
			}
			// The screen and the file have to agree about it, so the preview is
			// asked for the same slide.
			slide := Slide{LayoutID: layout.ID, Number: 4,
				Fields: map[string][]Paragraph{SlotTitle: {{Text: "1부"}}}}
			if svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 640}); !strings.Contains(svg, ">4</text>") {
				t.Fatalf("%s: the preview of the %q layout draws no page number", key, role)
			}
		}
	}
}
