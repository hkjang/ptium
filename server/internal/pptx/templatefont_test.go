package pptx

import "testing"

// A deck is set in the typeface its template uses, which is not always the one
// the template's theme names.
//
// One of the real decks measured against this product says Arial in its font
// scheme and Calibri in the placeholder styles on its master. PowerPoint draws
// the placeholders in Calibri — a placeholder's own style wins over the theme —
// so the exported deck came out in two typefaces: headings in Calibri because
// they inherit from the layout, and every KPI card and lead line in Arial
// because those are drawn rather than inherited. Neither was anybody's choice.
func TestBlocksAreDrawnInTheFontTheTemplatePlaceholdersUse(t *testing.T) {
	manifest := Manifest{
		Theme: Theme{MajorLatin: "Arial", MinorLatin: "Arial"},
		Layouts: []Layout{{
			Name: "TITLE",
			Placeholders: []Placeholder{
				{Type: "ctrTitle", Font: "Calibri"},
				{Type: "subTitle", Font: "Calibri"},
			},
		}, {
			Name: "OBJECT",
			Placeholders: []Placeholder{
				{Type: "title", Font: "Calibri"},
				{Type: "body", Font: "Calibri"},
			},
		}},
	}
	design := NewDesign(manifest)
	if design.Major != "Calibri" || design.Minor != "Calibri" {
		t.Errorf("blocks would be drawn in %q/%q beside placeholders set in Calibri", design.Major, design.Minor)
	}

	// A template whose placeholders name no typeface means its theme.
	bare := Manifest{
		Theme:   Theme{MajorLatin: "Pretendard", MinorLatin: "Pretendard"},
		Layouts: []Layout{{Name: "OBJECT", Placeholders: []Placeholder{{Type: "title"}, {Type: "body"}}}},
	}
	if design := NewDesign(bare); design.Major != "Pretendard" || design.Minor != "Pretendard" {
		t.Errorf("a template that names no placeholder font lost its theme: %q/%q", design.Major, design.Minor)
	}

	// A theme reference is not a typeface name.
	referenced := Manifest{
		Theme:   Theme{MajorLatin: "Pretendard", MinorLatin: "Pretendard"},
		Layouts: []Layout{{Name: "OBJECT", Placeholders: []Placeholder{{Type: "body", Font: "+mn-lt"}}}},
	}
	if design := NewDesign(referenced); design.Minor != "Pretendard" {
		t.Errorf("a placeholder pointing at the theme was taken literally: %q", design.Minor)
	}
}
