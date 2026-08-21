package pptx

import (
	"fmt"
	"sort"
	"strings"
)

// EMU units used throughout DrawingML.
const (
	EMUPerInch  = 914400
	EMUPerPoint = 12700
)

// Placeholder slot names. These are the vocabulary the generator and the API
// speak, independent of the underlying OOXML placeholder taxonomy.
const (
	SlotTitle    = "title"
	SlotSubtitle = "subtitle"
	SlotBody     = "body"
	SlotPicture  = "picture"
	SlotChart    = "chart"
	SlotTable    = "table"
)

// Layout roles describe what a layout is for in narrative terms so an AI model
// can choose between them without seeing the XML.
const (
	RoleTitle      = "title"
	RoleSection    = "section"
	RoleContent    = "content"
	RoleTwoContent = "twoContent"
	RoleComparison = "comparison"
	RoleQuote      = "quote"
	RolePicture    = "picture"
	RoleTable      = "table"
	RoleChart      = "chart"
	RoleClosing    = "closing"
	RoleBlank      = "blank"
)

// Theme holds the palette and typography a template already defines.
type Theme struct {
	Name       string            `json:"name,omitempty"`
	Colors     map[string]string `json:"colors,omitempty"`
	MajorLatin string            `json:"majorLatin,omitempty"`
	MinorLatin string            `json:"minorLatin,omitempty"`
	MajorEA    string            `json:"majorEa,omitempty"`
	MinorEA    string            `json:"minorEa,omitempty"`
}

// Color returns a theme color as an uppercase RRGGBB string.
func (t Theme) Color(name string) string {
	if value, ok := t.Colors[name]; ok && value != "" {
		return value
	}
	switch name {
	case "lt1", "bg1":
		return "FFFFFF"
	case "dk1", "tx1":
		return "000000"
	case "accent1":
		return "4472C4"
	}
	return "808080"
}

// IsDark reports whether a template paints on a dark surface, which is the
// first thing anyone notices about a design and the first way they narrow a
// library down.
func (m Manifest) IsDark() bool {
	surface := m.Theme.Color("lt1")
	if len(m.Layouts) > 0 && m.Layouts[0].Background != "" {
		surface = m.Layouts[0].Background
	}
	red, green, blue := parseHex(surface)
	return 0.2126*toLinear(red)+0.7152*toLinear(green)+0.0722*toLinear(blue) < 0.35
}

// Placeholder is a single fillable region of a layout.
type Placeholder struct {
	Slot     string  `json:"slot"`
	Kind     string  `json:"kind"`
	Type     string  `json:"type"`
	Index    int     `json:"index"`
	Name     string  `json:"name,omitempty"`
	X        int     `json:"x"`
	Y        int     `json:"y"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FontSize int     `json:"fontSize"`
	MaxChars int     `json:"maxChars"`
	MaxLines int     `json:"maxLines"`
	LineEm   float64 `json:"lineEm,omitempty"`
	Region   string  `json:"region,omitempty"`
	Vertical bool    `json:"vertical,omitempty"`
	Bold     bool    `json:"bold,omitempty"`
	Italic   bool    `json:"italic,omitempty"`
	Color    string  `json:"color,omitempty"`
	Font     string  `json:"font,omitempty"`
	// Align is set only where a slide overrides it. Empty means the region keeps
	// whatever alignment the template's own layout gives it.
	Align  string `json:"align,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	// Synthetic marks a region Ptium derived from the layout's free space because
	// the layout has no text placeholder of its own. The renderer draws a real
	// text box for it, styled from the template's theme, instead of filling a
	// placeholder that does not exist.
	Synthetic bool `json:"synthetic,omitempty"`
}

// AcceptsText reports whether the generator may write text into the slot.
func (p Placeholder) AcceptsText() bool { return p.Kind == "text" }

// Decoration is a static solid-filled shape a layout draws behind its
// placeholders — an accent rule, a colour block, a sidebar. Preview rendering
// includes them so a template's identity is recognisable in the browser.
//
// Deprecated: superseded by Artwork, which carries the same shapes plus the
// pictures, gradients and static text that most real templates are built from.
// It is still read so a manifest stored by an older release keeps rendering.
type Decoration struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Fill   string `json:"fill"`
	Round  bool   `json:"round,omitempty"`
}

// Artwork is one element a master or layout paints behind the placeholders,
// captured in paint order.
//
// Most real templates carry their identity here rather than in the colour
// scheme: a full-bleed photograph, a brand bar, a logo, a gradient panel, a
// footer. A preview that skips it shows a blank white slide and the person who
// uploaded the template concludes, correctly, that their design was ignored.
type Artwork struct {
	// Kind is "shape", "picture" or "text".
	Kind   string `json:"kind"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Rotation is in sixtieths of a thousandth of a degree, as DrawingML stores it.
	Rotation int  `json:"rot,omitempty"`
	FlipH    bool `json:"flipH,omitempty"`
	FlipV    bool `json:"flipV,omitempty"`
	// Preset is the DrawingML preset geometry name; empty means a rectangle.
	Preset string `json:"preset,omitempty"`
	// Fill is a resolved hex colour, empty when the shape is unfilled.
	Fill string `json:"fill,omitempty"`
	// Opacity is 0..1; zero means fully opaque so the field can stay omitted.
	Opacity float64 `json:"opacity,omitempty"`
	// Gradient holds two or more stops when the fill is a gradient.
	Gradient      []GradientStop `json:"gradient,omitempty"`
	GradientAngle int            `json:"gradientAngle,omitempty"`
	Stroke        string         `json:"stroke,omitempty"`
	StrokeWidth   int            `json:"strokeWidth,omitempty"`
	// Image is the package part of a picture fill, e.g. "ppt/media/image3.png".
	Image string `json:"image,omitempty"`
	// Crop is the source rectangle inset in thousandths of a percent: l, t, r, b.
	Crop [4]int `json:"crop,omitempty"`
	// Average is a picture's mean colour, which is what decides whether text
	// placed over it should be light or dark.
	Average string `json:"average,omitempty"`
	// Text is the static copy a template writes into its own artwork.
	Text     string `json:"text,omitempty"`
	FontSize int    `json:"fontSize,omitempty"`
	Bold     bool   `json:"bold,omitempty"`
	Color    string `json:"color,omitempty"`
	Font     string `json:"font,omitempty"`
	// Align is "l", "ctr" or "r"; Anchor is "t", "ctr" or "b".
	Align  string `json:"align,omitempty"`
	Anchor string `json:"anchor,omitempty"`
}

// GradientStop is one colour stop, positioned 0..1 along the gradient.
type GradientStop struct {
	Position float64 `json:"pos"`
	Color    string  `json:"color"`
	Opacity  float64 `json:"opacity,omitempty"`
}

// Background is what a slide paints before anything else.
type Background struct {
	// Fill is a resolved hex colour, used when there is no gradient or picture.
	Fill          string         `json:"fill,omitempty"`
	Gradient      []GradientStop `json:"gradient,omitempty"`
	GradientAngle int            `json:"gradientAngle,omitempty"`
	Image         string         `json:"image,omitempty"`
}

// Layout is one slide layout offered by the template.
type Layout struct {
	ID           string        `json:"id"`
	Part         string        `json:"part"`
	MasterPart   string        `json:"masterPart"`
	Name         string        `json:"name"`
	Type         string        `json:"type,omitempty"`
	Role         string        `json:"role"`
	Background   string        `json:"background,omitempty"`
	Decorations  []Decoration  `json:"decorations,omitempty"`
	Placeholders []Placeholder `json:"placeholders"`
	// Fill describes the background in full: a gradient or picture background is
	// not expressible as the single colour in Background.
	Fill Background `json:"fill,omitzero"`
	// Artwork is everything the master and layout paint, in paint order.
	Artwork []Artwork `json:"artwork,omitempty"`
	// Composed marks a layout whose writable regions Ptium derived from its free
	// space because it declares no text placeholder of its own.
	Composed bool `json:"composed,omitempty"`
}

// Slot finds a placeholder by canonical slot name.
func (l Layout) Slot(name string) (Placeholder, bool) {
	for _, placeholder := range l.Placeholders {
		if placeholder.Slot == name {
			return placeholder, true
		}
	}
	return Placeholder{}, false
}

// TextSlots lists the writable slots in reading order.
func (l Layout) TextSlots() []Placeholder {
	var result []Placeholder
	for _, placeholder := range l.Placeholders {
		if placeholder.AcceptsText() {
			result = append(result, placeholder)
		}
	}
	return result
}

// BodySlots lists the writable slots that are not the title or subtitle.
func (l Layout) BodySlots() []Placeholder {
	var result []Placeholder
	for _, placeholder := range l.TextSlots() {
		if placeholder.Slot == SlotTitle || placeholder.Slot == SlotSubtitle {
			continue
		}
		result = append(result, placeholder)
	}
	return result
}

// Manifest is the machine-readable description of a template. It is persisted
// alongside the uploaded file so generation never has to re-parse the package.
type Manifest struct {
	Version        int      `json:"version"`
	SlideWidth     int      `json:"slideWidth"`
	SlideHeight    int      `json:"slideHeight"`
	AspectRatio    string   `json:"aspectRatio"`
	Theme          Theme    `json:"theme"`
	Layouts        []Layout `json:"layouts"`
	MasterCount    int      `json:"masterCount"`
	HasNotesMaster bool     `json:"hasNotesMaster"`
	SourceSlides   int      `json:"sourceSlides"`
	DefaultLayout  string   `json:"defaultLayout,omitempty"`
	TitleLayout    string   `json:"titleLayout,omitempty"`
	ClosingLayout  string   `json:"closingLayout,omitempty"`
	SectionLayout  string   `json:"sectionLayout,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

// ManifestVersion is bumped whenever the analyzer changes in a way that makes
// previously stored manifests stale.
const ManifestVersion = 4

// Layout finds a layout by identifier.
func (m Manifest) Layout(id string) (Layout, bool) {
	for _, layout := range m.Layouts {
		if layout.ID == id {
			return layout, true
		}
	}
	return Layout{}, false
}

// LayoutByReference finds a layout the way someone refers to one: by its id, by
// the name it has in PowerPoint, or by either written loosely. A model copies a
// layout's name out of the catalogue as often as its id — "콘텐츠 2개" for
// "콘텐츠-2개" — and refusing that costs the deck the layout its author chose.
func (m Manifest) LayoutByReference(reference string) (Layout, bool) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Layout{}, false
	}
	if layout, ok := m.Layout(reference); ok {
		return layout, true
	}
	wanted := slug(reference)
	for _, layout := range m.Layouts {
		if layout.ID == wanted || strings.EqualFold(layout.Name, reference) || slug(layout.Name) == wanted {
			return layout, true
		}
	}
	return Layout{}, false
}

// LayoutForRole returns the best layout for a narrative role, falling back to
// the default content layout when the template has nothing more specific.
func (m Manifest) LayoutForRole(role string) (Layout, bool) {
	preferences := map[string][]string{
		RoleTitle:      {RoleTitle, RoleSection, RoleContent},
		RoleSection:    {RoleSection, RoleTitle, RoleContent},
		RoleTwoContent: {RoleTwoContent, RoleComparison, RoleContent},
		RoleComparison: {RoleComparison, RoleTwoContent, RoleContent},
		RoleQuote:      {RoleQuote, RoleSection, RoleContent},
		RolePicture:    {RolePicture, RoleContent},
		RoleTable:      {RoleTable, RoleContent},
		RoleChart:      {RoleChart, RoleContent},
		RoleClosing:    {RoleClosing, RoleSection, RoleContent},
		RoleContent:    {RoleContent, RoleTwoContent},
		RoleBlank:      {RoleBlank, RoleContent},
	}
	chain, ok := preferences[role]
	if !ok {
		chain = []string{RoleContent}
	}
	for _, candidate := range chain {
		for _, layout := range m.Layouts {
			if layout.Role == candidate {
				return layout, true
			}
		}
	}
	if len(m.Layouts) > 0 {
		return m.Layouts[0], true
	}
	return Layout{}, false
}

// Summary renders the layout catalog for an AI prompt using a mixed-script
// character budget.
func (m Manifest) Summary(limit int) string { return m.SummaryFor("", limit) }

// SummaryFor renders the layout catalog as compact text for an AI prompt, with
// each slot's character budget scaled to the language the deck is written in.
// Korean and Japanese glyphs are twice the width of Latin ones, so a single
// budget would either overflow one script or waste the other.
func (m Manifest) SummaryFor(language string, limit int) string {
	adjust := referenceAdvance / LanguageAdvance(language)
	if limit <= 0 || limit > len(m.Layouts) {
		limit = len(m.Layouts)
	}
	var builder strings.Builder
	for _, layout := range m.Layouts[:limit] {
		fmt.Fprintf(&builder, "- id=%s role=%s name=%q slots:", layout.ID, layout.Role, layout.Name)
		slots := layout.TextSlots()
		if len(slots) == 0 {
			builder.WriteString(" (none)")
		}
		for _, slot := range slots {
			lines := slot.MaxLines
			if slot.Slot == SlotTitle || slot.Slot == SlotSubtitle {
				lines = 1
			}
			budget := int(float64(slot.MaxChars) * adjust)
			if slot.Slot == SlotTitle || slot.Slot == SlotSubtitle {
				budget = budget * lines / max(slot.MaxLines, 1)
			}
			fmt.Fprintf(&builder, " %s(maxChars=%d,maxLines=%d,pos=%s)", slot.Slot, max(budget, 1), lines, slot.Region)
		}
		for _, slot := range layout.Placeholders {
			if slot.AcceptsText() || slot.Kind == "" {
				continue
			}
			fmt.Fprintf(&builder, " [%s:%s]", slot.Slot, slot.Kind)
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func (m *Manifest) finalize() {
	for layoutIndex := range m.Layouts {
		for index := range m.Layouts[layoutIndex].Placeholders {
			m.Layouts[layoutIndex].Placeholders[index].Region = region(m.Layouts[layoutIndex].Placeholders[index], m.SlideWidth, m.SlideHeight)
		}
	}
	sort.SliceStable(m.Layouts, func(i, j int) bool {
		if roleRank(m.Layouts[i].Role) != roleRank(m.Layouts[j].Role) {
			return roleRank(m.Layouts[i].Role) < roleRank(m.Layouts[j].Role)
		}
		return preferenceRank(m.Layouts[i]) < preferenceRank(m.Layouts[j])
	})
	for _, layout := range m.Layouts {
		switch layout.Role {
		case RoleTitle:
			if m.TitleLayout == "" {
				m.TitleLayout = layout.ID
			}
		case RoleSection:
			if m.SectionLayout == "" {
				m.SectionLayout = layout.ID
			}
		case RoleClosing:
			if m.ClosingLayout == "" {
				m.ClosingLayout = layout.ID
			}
		case RoleContent:
			if m.DefaultLayout == "" {
				m.DefaultLayout = layout.ID
			}
		}
	}
	if m.DefaultLayout == "" {
		if layout, ok := m.LayoutForRole(RoleContent); ok {
			m.DefaultLayout = layout.ID
		}
	}
	if m.TitleLayout == "" {
		m.TitleLayout = m.DefaultLayout
	}
	if m.SectionLayout == "" {
		m.SectionLayout = m.DefaultLayout
	}
	if m.ClosingLayout == "" {
		m.ClosingLayout = m.SectionLayout
	}
	m.AspectRatio = aspectRatio(m.SlideWidth, m.SlideHeight)
}

// preferenceRank orders layouts that share a role, so automatic selection
// reaches for the conventional one first. Vertical-text layouts exist for
// traditional CJK typesetting and read as a mistake when a deck's ordinary
// bullet slides land in one; title-only layouts have nowhere to put content.
func preferenceRank(layout Layout) int {
	rank := 0
	switch layout.Type {
	case "vertTx", "vertTitleAndTx", "vertTitleAndTxOverChart", "clipArtAndVertTx":
		rank += 8
	case "titleOnly":
		rank += 4
	}
	writable := 0
	for _, placeholder := range layout.Placeholders {
		if !placeholder.AcceptsText() {
			continue
		}
		if placeholder.Vertical {
			rank += 6
		}
		if placeholder.Slot != SlotTitle {
			writable++
		}
	}
	if writable == 0 {
		rank += 3
	}
	return rank
}

// region labels where a placeholder sits on the canvas so a writer can tell a
// left column from a right one without reading coordinates.
func region(placeholder Placeholder, slideWidth, slideHeight int) string {
	if slideWidth <= 0 || slideHeight <= 0 {
		return ""
	}
	centerX := placeholder.X + placeholder.Width/2
	centerY := placeholder.Y + placeholder.Height/2
	horizontal := "center"
	switch {
	case placeholder.Width*100/slideWidth >= 70:
		horizontal = "full"
	case centerX*100/slideWidth < 40:
		horizontal = "left"
	case centerX*100/slideWidth > 60:
		horizontal = "right"
	}
	vertical := "middle"
	switch {
	case centerY*100/slideHeight < 33:
		vertical = "top"
	case centerY*100/slideHeight > 66:
		vertical = "bottom"
	}
	return horizontal + "-" + vertical
}

func roleRank(role string) int {
	order := map[string]int{
		RoleTitle: 0, RoleSection: 1, RoleContent: 2, RoleTwoContent: 3, RoleComparison: 4,
		RolePicture: 5, RoleChart: 6, RoleTable: 7, RoleQuote: 8, RoleClosing: 9, RoleBlank: 10,
	}
	if rank, ok := order[role]; ok {
		return rank
	}
	return 11
}

func aspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	ratio := float64(width) / float64(height)
	switch {
	case ratio > 1.7 && ratio < 1.8:
		return "16:9"
	case ratio > 1.55 && ratio <= 1.7:
		return "16:10"
	case ratio > 1.2 && ratio <= 1.4:
		return "4:3"
	}
	return fmt.Sprintf("%.2f:1", ratio)
}
