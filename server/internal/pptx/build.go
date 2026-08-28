package pptx

import (
	"encoding/xml"
	"fmt"
	"html"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Paragraph is one line of text inside a placeholder. Level 0 is the top
// bullet level; deeper levels indent using the template's own bullet styling.
type Paragraph struct {
	Text  string `json:"text"`
	Level int    `json:"level,omitempty"`
	// Lead marks the slide's one-line summary, kept in a body region because the
	// layout has no region of its own for it. It is drawn without a bullet: it is
	// what the slide says, not one of the points that support it.
	Lead bool `json:"lead,omitempty"`
}

// Citation is where one of a slide's figures came from.
type Citation struct {
	Marker  string `json:"marker,omitempty"`
	Title   string `json:"title"`
	Locator string `json:"locator,omitempty"`
}

// Slide is one rendered slide bound to a template layout. A slot carries
// either paragraphs of prose or one visual component, never both.
type Slide struct {
	LayoutID string                 `json:"layoutId"`
	Fields   map[string][]Paragraph `json:"fields"`
	Blocks   map[string]Block       `json:"blocks,omitempty"`
	// Pictures maps a slot to an image drawn in it, bytes and all: a rendered
	// package has to carry the picture, not a reference to it.
	Pictures map[string]Picture `json:"pictures,omitempty"`
	// Elements are freely positioned editable objects layered above the
	// template-bound content.
	Elements []Element `json:"elements,omitempty"`
	// Frames overrides where a slot draws. The canvas writes one when someone
	// moves or resizes a region the template placed; every slot without an entry
	// keeps the template's own geometry, which is what lets a deck be rearranged
	// by hand without losing the design it was built from.
	Frames map[string]Frame `json:"frames,omitempty"`
	// Styles overrides how a region's text is set — its size, colour, weight and
	// alignment. Like Frames, a slot with no entry keeps the template's own
	// styling, so a deck looks like its template until someone decides otherwise.
	Styles map[string]Style `json:"styles,omitempty"`
	Notes  string           `json:"notes,omitempty"`
	// Skipped is a slide that stays in the file and out of the talk. PowerPoint
	// reads the same flag off the slide part, so a deck exported from here walks
	// past it too.
	Skipped bool `json:"skipped,omitempty"`
	// Sources are where the slide's figures came from. They are printed at the
	// end of the speaker notes, which is where a presenter looks when someone
	// asks — and where PowerPoint keeps them without touching the slide.
	Sources []Citation `json:"sources,omitempty"`
	// Number is the page number this slide shows, and HideNumber suppresses it.
	// The cover of a deck carries no number, the way covers do not.
	Number     int  `json:"number,omitempty"`
	HideNumber bool `json:"hideNumber,omitempty"`
	// Accent colours the components this product draws — the tiles, the bars,
	// the rules — when the author has chosen a colour of their own. The
	// template's masters, layouts, fonts and text colours are untouched by it:
	// what a company designed stays as they designed it, and an empty value
	// uses the template's own accent, which is what it always did.
	Accent string `json:"accent,omitempty"`
}

// SeededAccent is the brand colour this product ships with. A deployment that
// has never opened the branding screen still carries it, so it is nobody's
// choice — and every deck generated before the accent reached the drawing has
// it written on every slide. Painting a customer's own template with it, on the
// next export of a deck they made months ago, is not what any screen promised.
//
// An author who deliberately picks this exact purple gets their template's own
// accent instead. That is the price of not repainting everybody else's decks.
const SeededAccent = "#7C3AED"

// withAccent is the design a slide's components are drawn in.
//
// The colour was computed for every slide, stored on every slide and read by
// nothing: a person could set a brand colour, be told it would be used, and see
// no difference anywhere.
func (s Slide) withAccent(design Design) Design {
	color := strings.ToUpper(strings.TrimSpace(s.Accent))
	if !looksLikeHexColor(color) || strings.EqualFold(color, design.Accent) ||
		strings.EqualFold(color, SeededAccent) {
		return design
	}
	design.Accent = color
	design.OnAccent = readableInk(color, design.Surface, design.InkPrimary)
	return design
}

// looksLikeHexColor is #RRGGBB and nothing else: a colour that reaches a
// drawing has to be one.
func looksLikeHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'A' && character <= 'F':
		case character >= 'a' && character <= 'f':
		default:
			return false
		}
	}
	return true
}

// Style is what a slide changes about one region's type. Every field is
// optional: an author who only centres a line has not also chosen its size.
type Style struct {
	// Scale multiplies the template's own size. Zero means unchanged.
	Scale  float64 `json:"scale,omitempty"`
	Color  string  `json:"color,omitempty"`
	Bold   *bool   `json:"bold,omitempty"`
	Italic *bool   `json:"italic,omitempty"`
	// Align is left, center, right or justify. Empty keeps the template's.
	Align string `json:"align,omitempty"`
}

// Empty reports whether the style changes nothing.
func (s Style) Empty() bool {
	return s.Scale == 0 && s.Color == "" && s.Bold == nil && s.Italic == nil && s.Align == ""
}

// alignment maps an author's word to the DrawingML attribute.
func (s Style) alignment() string {
	switch strings.ToLower(strings.TrimSpace(s.Align)) {
	case "center", "ctr":
		return "ctr"
	case "right", "r", "end":
		return "r"
	case "justify", "just":
		return "just"
	case "left", "l", "start":
		return "l"
	}
	return ""
}

// Place returns the placeholder as this slide positions it. A moved region
// carries its capacity with it: a taller box holds more lines, and autofit has to
// know that or it shrinks text that now fits.
func (s Slide) Place(placeholder Placeholder) Placeholder {
	placeholder = s.style(placeholder)
	frame, ok := s.Frames[placeholder.Slot]
	if !ok || frame.Width <= 0 || frame.Height <= 0 {
		return placeholder
	}
	horizontal, vertical := 1.0, 1.0
	if placeholder.Width > 0 {
		horizontal = float64(frame.Width) / float64(placeholder.Width)
	}
	if placeholder.Height > 0 {
		vertical = float64(frame.Height) / float64(placeholder.Height)
	}
	placeholder.X, placeholder.Y = frame.X, frame.Y
	placeholder.Width, placeholder.Height = frame.Width, frame.Height
	if placeholder.MaxChars > 0 {
		placeholder.MaxChars = max(1, int(math.Round(float64(placeholder.MaxChars)*horizontal*vertical)))
	}
	if placeholder.MaxLines > 0 {
		placeholder.MaxLines = max(1, int(math.Round(float64(placeholder.MaxLines)*vertical)))
	}
	if placeholder.LineEm > 0 {
		placeholder.LineEm = placeholder.LineEm * horizontal
	}
	return placeholder
}

// style applies a slide's own typography to a region. Setting text larger means
// less of it fits on a line, and autofit has to be told that or it lets the text
// spill instead of shrinking it back.
func (s Slide) style(placeholder Placeholder) Placeholder {
	style, ok := s.Styles[placeholder.Slot]
	if !ok || style.Empty() {
		return placeholder
	}
	if style.Scale > 0 && math.Abs(style.Scale-1) > 0.001 {
		scale := min(max(style.Scale, 0.4), 3)
		if placeholder.FontSize > 0 {
			placeholder.FontSize = max(400, int(math.Round(float64(placeholder.FontSize)*scale)))
		}
		if placeholder.MaxChars > 0 {
			placeholder.MaxChars = max(1, int(math.Round(float64(placeholder.MaxChars)/scale)))
		}
		if placeholder.MaxLines > 0 {
			placeholder.MaxLines = max(1, int(math.Round(float64(placeholder.MaxLines)/scale)))
		}
		if placeholder.LineEm > 0 {
			placeholder.LineEm = placeholder.LineEm / scale
		}
	}
	if style.Color != "" {
		placeholder.Color = style.Color
	}
	if style.Bold != nil {
		placeholder.Bold = *style.Bold
	}
	if style.Italic != nil {
		placeholder.Italic = *style.Italic
	}
	if aligned := style.alignment(); aligned != "" {
		placeholder.Align = aligned
	}
	return placeholder
}

// spannedSlots is every region covered by a component placed elsewhere.
func (s Slide) spannedSlots() map[string]bool {
	spanned := map[string]bool{}
	for slot, block := range s.Blocks {
		for _, other := range block.Span {
			if other != slot {
				spanned[other] = true
			}
		}
	}
	return spanned
}

// blockFrame is the area a component draws in: its own region, plus the regions
// it spans.
func (s Slide) blockFrame(layout Layout, placeholder Placeholder, block Block) Frame {
	placeholder = s.Place(placeholder)
	frame := Frame{X: placeholder.X, Y: placeholder.Y, Width: placeholder.Width, Height: placeholder.Height}
	for _, slot := range block.Span {
		other, ok := layout.Slot(slot)
		if !ok || other.Slot == placeholder.Slot {
			continue
		}
		other = s.Place(other)
		right := max(frame.X+frame.Width, other.X+other.Width)
		bottom := max(frame.Y+frame.Height, other.Y+other.Height)
		frame.X = min(frame.X, other.X)
		frame.Y = min(frame.Y, other.Y)
		frame.Width = right - frame.X
		frame.Height = bottom - frame.Y
	}
	return frame
}

// Carries reports whether the slide already shows this sentence, in any region
// or as a component's heading. A lead line is recorded in more than one place, so
// writing it again without checking puts it on the slide twice.
func (s Slide) Carries(text string) bool {
	wanted := strings.TrimSpace(text)
	if wanted == "" {
		return true
	}
	for _, paragraphs := range s.Fields {
		for _, paragraph := range paragraphs {
			if strings.TrimSpace(paragraph.Text) == wanted {
				return true
			}
		}
	}
	for _, block := range s.Blocks {
		if strings.TrimSpace(block.Heading) == wanted {
			return true
		}
	}
	return false
}

// Picture is an image to place on a slide.
type Picture struct {
	Data        []byte
	ContentType string
	// Width and Height are the image's pixels, used to crop it into its frame
	// without distorting it. Zero means unknown, and the image is stretched.
	Width   int
	Height  int
	Caption string
}

// extension is the part suffix PowerPoint expects for the picture's type.
func (p Picture) extension() string {
	switch strings.ToLower(strings.TrimSpace(p.ContentType)) {
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/gif":
		return "gif"
	case "image/svg+xml":
		return "svg"
	}
	return "png"
}

// Deck is the complete input to Render.
type Deck struct {
	Title    string `json:"title"`
	Subject  string `json:"subject,omitempty"`
	Author   string `json:"author,omitempty"`
	Language string `json:"language,omitempty"`
	// Source is the text the deck was written from. It is carried in the
	// exported file so that importing a Ptium deck restores what was written
	// rather than reading it back off the drawing.
	Source string `json:"source,omitempty"`
	// Brief is what the deck was asked for, in the author's own words. It is
	// not drawn; it is how measuring can tell a figure the author supplied —
	// the budget they are asking for — from one the deck states about the
	// world and cannot source.
	Brief  string  `json:"brief,omitempty"`
	Slides []Slide `json:"slides"`
}

// Render rebuilds a template package into a finished deck. Masters, layouts,
// theme, fonts and media are carried over untouched; only the slide parts and
// the package plumbing that references them are rewritten.
func Render(template *Package, manifest Manifest, deck Deck) ([]byte, error) {
	if template == nil {
		return nil, fmt.Errorf("a template package is required")
	}
	if len(deck.Slides) == 0 {
		return nil, fmt.Errorf("the deck does not contain any slide")
	}
	pkg := template.Clone()
	dropExistingSlides(pkg)
	design := NewDesign(manifest)

	language := normalizeLanguage(deck.Language)
	notesNeeded := false
	for _, slide := range deck.Slides {
		if strings.TrimSpace(notesWithSources(slide, language)) != "" {
			notesNeeded = true
			break
		}
	}
	notesMasterPart, _ := pkg.RelatedPart("ppt/presentation.xml", "notesMaster")
	createdNotesMaster := false
	if notesNeeded && notesMasterPart == "" {
		notesMasterPart = ensureNotesMaster(pkg, manifest)
		createdNotesMaster = true
	}

	slideRelIDs := make([]string, 0, len(deck.Slides))
	nextRelationshipID := maxRelationshipNumber(pkg, "ppt/presentation.xml") + 1
	notesIndex, chartIndex := 0, 0
	for index, slide := range deck.Slides {
		layout, ok := manifest.Layout(slide.LayoutID)
		if !ok {
			if layout, ok = manifest.LayoutForRole(RoleContent); !ok {
				return nil, fmt.Errorf("slide %d references unknown layout %q", index+1, slide.LayoutID)
			}
		}
		// Numbering is the deck's, not the slide's: a slide does not know where it
		// sits until it is placed in one. A caller that already numbered its deck
		// — the same code path every preview goes through — is left alone, so the
		// file and the screen cannot disagree.
		if slide.Number == 0 {
			slide.Number = index + 1
			slide.HideNumber = layout.Role == RoleTitle
		}
		slidePart := fmt.Sprintf("ppt/slides/slide%d.xml", index+1)
		var notesPart string
		if strings.TrimSpace(notesWithSources(slide, language)) != "" && notesMasterPart != "" {
			notesIndex++
			notesPart = fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", notesIndex)
		}
		// Pictures become package parts and slide relationships before the slide
		// itself is written, because the shape refers to them by relationship id.
		pictures := addSlidePictures(pkg, slidePart, index+1, slide, layout)
		markup, charts, links := slideXML(layout, slide, language, design, pictures, len(deck.Slides))
		pkg.SetText(slidePart, markup)
		chartParts := addSlideCharts(pkg, slidePart, &chartIndex, charts)
		pkg.SetText(RelationshipsPath(slidePart), slideRelationshipsXML(slidePart, layout.Part, notesPart, pictures, chartParts, links))
		if notesPart != "" {
			notesMarkup, notesLinks := notesSlideXML(notesWithSources(slide, language), language, len(deck.Slides))
			pkg.SetText(notesPart, notesMarkup)
			pkg.SetText(RelationshipsPath(notesPart), notesRelationshipsXML(notesPart, notesMasterPart, slidePart, notesLinks))
		}
		slideRelIDs = append(slideRelIDs, fmt.Sprintf("rId%d", nextRelationshipID))
		nextRelationshipID++
	}

	notesMasterRelID := ""
	if createdNotesMaster {
		notesMasterRelID = fmt.Sprintf("rId%d", nextRelationshipID)
		nextRelationshipID++
	}
	sourceTarget := writeDeckSource(pkg, deck.Source)
	pkg.SetText(RelationshipsPath("ppt/presentation.xml"),
		presentationRelationshipsXML(pkg, slideRelIDs, notesMasterRelID, notesMasterPart, sourceTarget))

	presentation, _ := pkg.Text("ppt/presentation.xml")
	pkg.SetText("ppt/presentation.xml", rewritePresentation(presentation, slideRelIDs, notesMasterRelID, language))
	pkg.SetText("[Content_Types].xml", contentTypesXML(pkg))
	pkg.SetText("docProps/app.xml", appPropertiesXML(manifest, deck))
	pkg.SetText("docProps/core.xml", corePropertiesXML(deck))
	return pkg.Bytes()
}

// RenderBytes analyzes and renders in one call. Callers that already hold a
// manifest should prefer Render to avoid re-parsing.
func RenderBytes(templateData []byte, deck Deck) ([]byte, error) {
	pkg, manifest, err := AnalyzeBytes(templateData)
	if err != nil {
		return nil, err
	}
	return Render(pkg, manifest, deck)
}

func dropExistingSlides(pkg *Package) {
	for _, name := range pkg.Names() {
		switch {
		case strings.HasPrefix(name, "ppt/slides/"),
			strings.HasPrefix(name, "ppt/notesSlides/"):
			pkg.Delete(name)
		}
	}
	dropOrphanedParts(pkg)
}

// dropOrphanedParts removes what the departed slides left behind.
//
// A template is often a deck: someone uploads last quarter's report and builds
// on it. Its charts and their workbooks belonged to slides that are now gone,
// and nothing points at them any more — so they would ride along, unseen, in
// every file made from that template. The workbooks go after the charts, since
// a chart is the only thing that points at one.
func dropOrphanedParts(pkg *Package) {
	for _, prefix := range []string{"ppt/charts/", "ppt/embeddings/"} {
		referenced := map[string]bool{}
		for _, name := range pkg.Names() {
			if strings.Contains(name, "/_rels/") || strings.HasPrefix(name, "_rels/") {
				continue
			}
			for _, relationship := range pkg.Relationships(name) {
				if relationship.TargetMode == "External" {
					continue
				}
				referenced[Resolve(name, relationship.Target)] = true
			}
		}
		for _, name := range pkg.Names() {
			if !strings.HasPrefix(name, prefix) || strings.Contains(name, "/_rels/") || referenced[name] {
				continue
			}
			pkg.Delete(name)
			pkg.Delete(RelationshipsPath(name))
		}
	}
}

func maxRelationshipNumber(pkg *Package, part string) int {
	highest := 0
	for _, relationship := range pkg.Relationships(part) {
		value := strings.TrimPrefix(relationship.ID, "rId")
		if number, err := strconv.Atoi(value); err == nil && number > highest {
			highest = number
		}
	}
	if highest < 1 {
		highest = 1
	}
	return highest
}

// placedPicture is one image bound to a slot, with the relationship id the slide
// refers to it by.
type placedPicture struct {
	Slot           string
	ElementID      string
	RelationshipID string
	// Part is the package part the image was written to.
	Part    string
	Picture Picture
}

// addSlidePictures writes each of a slide's images into the package and returns
// them with the relationship ids the slide will use.
func addSlidePictures(pkg *Package, slidePart string, position int, slide Slide, layout Layout) []placedPicture {
	if len(slide.Pictures) == 0 && len(slide.Elements) == 0 {
		return nil
	}
	// Slots in the layout's own order, with anything unknown to the layout after
	// them in name order, so a deck renders identically twice.
	seen := map[string]bool{}
	slots := make([]string, 0, len(slide.Pictures))
	for _, placeholder := range layout.Placeholders {
		if _, ok := slide.Pictures[placeholder.Slot]; ok && !seen[placeholder.Slot] {
			seen[placeholder.Slot] = true
			slots = append(slots, placeholder.Slot)
		}
	}
	remaining := make([]string, 0, len(slide.Pictures))
	for slot := range slide.Pictures {
		if !seen[slot] {
			remaining = append(remaining, slot)
		}
	}
	sort.Strings(remaining)
	slots = append(slots, remaining...)

	placed := make([]placedPicture, 0, len(slots)+len(slide.Elements))
	// Slide relationship ids start after the layout and any notes slide.
	next := firstPictureRelationship
	mediaIndex := 0
	for _, slot := range slots {
		picture := slide.Pictures[slot]
		if len(picture.Data) == 0 {
			continue
		}
		mediaIndex++
		part := fmt.Sprintf("ppt/media/ptium%d-%d.%s", position, mediaIndex, picture.extension())
		pkg.Set(part, picture.Data)
		placed = append(placed, placedPicture{
			Slot: slot, RelationshipID: fmt.Sprintf("rId%d", next+len(placed)), Part: part, Picture: picture,
		})
	}
	for _, element := range slide.Elements {
		if element.Kind != "image" || element.Picture == nil || len(element.Picture.Data) == 0 {
			continue
		}
		mediaIndex++
		picture := *element.Picture
		part := fmt.Sprintf("ppt/media/ptium%d-%d.%s", position, mediaIndex, picture.extension())
		pkg.Set(part, picture.Data)
		placed = append(placed, placedPicture{
			ElementID: element.ID, RelationshipID: fmt.Sprintf("rId%d", next+len(placed)), Part: part, Picture: picture,
		})
	}
	return placed
}

// pictureXML draws an image inside its slot, cropped to the frame rather than
// stretched: a squashed logo is worse than a tight one.
func pictureXML(shapeID int, placeholder Placeholder, placed placedPicture) string {
	name := placeholder.Name
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("Picture %d", shapeID-1)
	}
	crop := ""
	if placed.Picture.Width > 0 && placed.Picture.Height > 0 && placeholder.Width > 0 && placeholder.Height > 0 {
		frameRatio := float64(placeholder.Width) / float64(placeholder.Height)
		imageRatio := float64(placed.Picture.Width) / float64(placed.Picture.Height)
		switch {
		case imageRatio > frameRatio*1.01:
			// Wider than the frame: trim the sides.
			inset := int(math.Round((1 - frameRatio/imageRatio) / 2 * 100000))
			crop = fmt.Sprintf(`<a:srcRect l="%d" r="%d"/>`, inset, inset)
		case imageRatio < frameRatio*0.99:
			inset := int(math.Round((1 - imageRatio/frameRatio) / 2 * 100000))
			crop = fmt.Sprintf(`<a:srcRect t="%d" b="%d"/>`, inset, inset)
		}
	}
	return `<p:pic><p:nvPicPr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="` + escapeAttribute(name) + `"` +
		descriptionAttribute(placed.Picture.Caption) + `/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr>` +
		`<p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="` + placed.RelationshipID + `"/>` + crop +
		`<a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr><a:xfrm><a:off x="` +
		strconv.Itoa(placeholder.X) + `" y="` + strconv.Itoa(placeholder.Y) + `"/><a:ext cx="` +
		strconv.Itoa(placeholder.Width) + `" cy="` + strconv.Itoa(placeholder.Height) +
		`"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`
}

func descriptionAttribute(caption string) string {
	if strings.TrimSpace(caption) == "" {
		return ""
	}
	return ` descr="` + escapeAttribute(caption) + `"`
}

// freeformPictureXML draws a freely positioned picture. The image remains a
// normal PowerPoint picture so it can still be cropped or replaced after export.
func freeformPictureXML(shapeID int, element Element, placed placedPicture) string {
	frame := element.Frame
	crop := ""
	if placed.Picture.Width > 0 && placed.Picture.Height > 0 && frame.Width > 0 && frame.Height > 0 {
		frameRatio := float64(frame.Width) / float64(frame.Height)
		imageRatio := float64(placed.Picture.Width) / float64(placed.Picture.Height)
		switch strings.ToLower(element.Fit) {
		case "contain":
			if imageRatio > frameRatio {
				height := int(math.Round(float64(frame.Width) / imageRatio))
				frame.Y += (frame.Height - height) / 2
				frame.Height = height
			} else {
				width := int(math.Round(float64(frame.Height) * imageRatio))
				frame.X += (frame.Width - width) / 2
				frame.Width = width
			}
		case "fill":
			// Stretching is deliberate for this explicit mode.
		default:
			switch {
			case imageRatio > frameRatio*1.01:
				inset := int(math.Round((1 - frameRatio/imageRatio) / 2 * 100000))
				crop = fmt.Sprintf(`<a:srcRect l="%d" r="%d"/>`, inset, inset)
			case imageRatio < frameRatio*0.99:
				inset := int(math.Round((1 - imageRatio/frameRatio) / 2 * 100000))
				crop = fmt.Sprintf(`<a:srcRect t="%d" b="%d"/>`, inset, inset)
			}
		}
	}
	name := strings.TrimSpace(element.ID)
	if name == "" {
		name = fmt.Sprintf("Picture %d", shapeID-1)
	}
	rotation := ""
	if math.Abs(element.Rotation) >= .005 {
		rotation = ` rot="` + strconv.Itoa(int(math.Round(element.Rotation*60000))) + `"`
	}
	alpha := ""
	if element.opacity() < 100 {
		alpha = `<a:alphaModFix amt="` + strconv.Itoa(element.opacity()*1000) + `"/>`
	}
	locks := `<a:picLocks noChangeAspect="1"`
	if element.Locked {
		locks += ` noMove="1" noResize="1" noRot="1"`
	}
	locks += `/>`
	return `<p:pic><p:nvPicPr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="` + escapeAttribute(name) + `"` +
		descriptionAttribute(element.Caption) + `/><p:cNvPicPr>` + locks + `</p:cNvPicPr><p:nvPr/></p:nvPicPr>` +
		`<p:blipFill><a:blip r:embed="` + placed.RelationshipID + `">` + alpha + `</a:blip>` + crop +
		`<a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr><a:xfrm` + rotation + `><a:off x="` +
		strconv.Itoa(frame.X) + `" y="` + strconv.Itoa(frame.Y) + `"/><a:ext cx="` + strconv.Itoa(max(frame.Width, 1)) +
		`" cy="` + strconv.Itoa(max(frame.Height, 1)) + `"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`
}

// slideXML writes the slide, and returns the charts it placed on it in the
// order their relationship ids were handed out. The parts themselves are
// written by the caller, which is the one holding the package.
// spokenLanguage is the tag a run declares itself in. A screen reader picks its
// voice from it and PowerPoint spell-checks against it, so a deck written in
// English whose table cells said ko-KR had those cells read aloud in Korean and
// underlined as Korean misspellings. Empty is the Korean this product was
// written for, which is what every one of those places used to hardcode.
func spokenLanguage(language string) string {
	if language = strings.TrimSpace(language); language == "" {
		return "ko-KR"
	}
	return language
}

func slideXML(layout Layout, slide Slide, language string, design Design,
	pictures []placedPicture, slides int) (string, []*ChartPart, []slideLink) {
	var shapes, components, freeform strings.Builder
	var charts []*ChartPart
	links := &linkTable{slides: slides}
	// A chart is a relationship of the slide, and the picture relationships came
	// first.
	chartRelationship := func(chart *ChartPart) string {
		if chart == nil {
			return ""
		}
		charts = append(charts, chart)
		return fmt.Sprintf("rId%d", firstPictureRelationship+len(pictures)+len(charts)-1)
	}
	shapeID := 2
	placedBySlot := map[string]placedPicture{}
	placedByElement := map[string]placedPicture{}
	for _, placed := range pictures {
		if placed.ElementID != "" {
			placedByElement[placed.ElementID] = placed
		} else {
			placedBySlot[placed.Slot] = placed
		}
	}
	spanned := slide.spannedSlots()
	for _, placeholder := range layout.Placeholders {
		if spanned[placeholder.Slot] {
			// A component placed elsewhere covers this region.
			continue
		}
		placeholder = slide.Place(placeholder)
		// An image replaces whatever the slot would otherwise hold.
		if placed, ok := placedBySlot[placeholder.Slot]; ok {
			components.WriteString(pictureXML(shapeID, placeholder, placed))
			shapeID++
			continue
		}
		// A component replaces its placeholder rather than sitting on top of
		// it, so the exported slide has no empty text box behind the drawing.
		if block, ok := slide.Blocks[placeholder.Slot]; ok && placeholder.AcceptsText() {
			frame := slide.blockFrame(layout, placeholder, block)
			if component := RenderBlock(slide.withAccent(design), frame, block); len(component.Primitives) > 0 {
				if slide.StandsAlone(layout, placeholder.Slot) {
					centreInFrame(&component, frame)
				}
				// The language is known here rather than inside the drawing, and
				// alternative text is read aloud in the deck's own language.
				component.Description = describeBlock(block, language)
				if component.Table != nil {
					component.Description = describeTable(component.Table, block, language)
				}
				markup, next := component.DrawingML(shapeID, chartRelationship(component.Chart), links, language)
				components.WriteString(markup)
				shapeID = next
				continue
			}
		}
		paragraphs := slide.Fields[placeholder.Slot]
		if len(paragraphs) == 0 && placeholder.AcceptsText() {
			continue
		}
		_, moved := slide.Frames[placeholder.Slot]
		shapes.WriteString(placeholderShapeXML(shapeID, placeholder, paragraphs, language, moved,
			slide.Styles[placeholder.Slot], links))
		shapeID++
	}
	for _, element := range slide.Elements {
		// A text box nobody typed into is nothing. One reached a deck carrying the
		// editor's own prompt — "텍스트를 입력하세요" printed on the slide, in the
		// preview and in the exported file — because the box was created with that
		// sentence as its content rather than as a hint.
		if element.Kind == "text" && strings.TrimSpace(element.Text) == "" {
			continue
		}
		if element.Kind == "image" {
			if placed, ok := placedByElement[element.ID]; ok {
				freeform.WriteString(freeformPictureXML(shapeID, element, placed))
				shapeID++
			}
			continue
		}
		freeform.WriteString(element.drawingML(shapeID, links, language))
		shapeID++
	}
	// show="0" is how the format says "not part of the show". Written only when
	// it is true: the attribute's absence is the default in every reader.
	shown := ""
	if slide.Skipped {
		shown = ` show="0"`
	}
	return xmlDeclaration + `<p:sld ` + presentationNamespaces + shown + `><p:cSld><p:spTree>` + emptyGroupHeader +
		shapes.String() + components.String() + freeform.String() +
		sourceNoteXML(shapeID, slideSourceNote(layout, slide, language), language, links) +
		slideNumberXML(shapeID+1, layout, slide, language) +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`, charts, links.links
}

// slideNumberXML writes the page number where the design put its placeholder.
//
// A master can declare the placeholder and still show nothing: PowerPoint draws
// the number only for slides that carry the shape themselves, which is what its
// Header & Footer dialog inserts. Ptium writes it for the same reason a person
// ticks that box — a deck of twenty slides that nobody can refer to by number is
// harder to talk about — and leaves the cover alone, as every deck does.
// latinTypefaceXML names the font a run is set in, for Latin characters only.
//
// It used to name the same font for East Asian characters as well, which said
// that Hangul and kanji were to be set in Aptos — a face with neither. The
// template's own East Asian font is what those characters are for, and leaving
// the element out is how a run inherits it: a Korean or Japanese deck exported
// from here was overriding its template's Korean font with a Latin one on every
// run of text it contained.
func latinTypefaceXML(font string) string {
	if strings.TrimSpace(font) == "" {
		return ""
	}
	return `<a:latin typeface="` + escapeAttribute(font) + `"/>`
}

func slideNumberXML(shapeID int, layout Layout, slide Slide, language string) string {
	slot := layout.SlideNumber
	if slot == nil || slide.Number <= 0 || slide.HideNumber {
		return ""
	}
	if language == "" {
		language = "ko-KR"
	}
	reference := `<p:ph type="sldNum"`
	if slot.Index > 0 {
		reference += ` idx="` + strconv.Itoa(slot.Index) + `"`
	}
	reference += `/>`
	align := strings.TrimSpace(slot.Align)
	if align == "" {
		align = "r"
	}
	properties := `<a:rPr lang="` + escapeAttribute(language) + `" smtClean="0"`
	if slot.FontSize > 0 {
		properties += ` sz="` + strconv.Itoa(slot.FontSize) + `"`
	}
	if slot.Bold {
		properties += ` b="1"`
	}
	properties += `>`
	if colour := strings.TrimSpace(slot.Color); colour != "" {
		properties += `<a:solidFill><a:srgbClr val="` + escapeAttribute(strings.TrimPrefix(colour, "#")) + `"/></a:solidFill>`
	}
	if font := strings.TrimSpace(slot.Font); font != "" {
		properties += latinTypefaceXML(font)
	}
	properties += `</a:rPr>`
	// The field is what makes the number follow the slide when someone reorders
	// the deck in PowerPoint; the literal text is only what renderers show before
	// they evaluate it.
	body := `<a:p><a:pPr algn="` + escapeAttribute(align) + `"/>` +
		`<a:fld id="{5B2B4A21-6D1F-4A76-9E30-6B4C1E9F4B10}" type="slidenum">` + properties +
		`<a:t>` + strconv.Itoa(slide.Number) + `</a:t></a:fld>` +
		`<a:endParaRPr lang="` + escapeAttribute(language) + `"/></a:p>`
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="Slide Number Placeholder"/>` +
		`<p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>` + reference + `</p:nvPr></p:nvSpPr>` +
		`<p:spPr><a:xfrm><a:off x="` + strconv.Itoa(slot.X) + `" y="` + strconv.Itoa(slot.Y) + `"/>` +
		`<a:ext cx="` + strconv.Itoa(slot.Width) + `" cy="` + strconv.Itoa(slot.Height) + `"/></a:xfrm></p:spPr>` +
		`<p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/>` + body + `</p:txBody></p:sp>`
}

func placeholderShapeXML(shapeID int, placeholder Placeholder, paragraphs []Paragraph, language string,
	moved bool, style Style, links *linkTable) string {
	name := placeholder.Name
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%s %d", capitalize(placeholder.Slot), shapeID-1)
	}
	if placeholder.Synthetic {
		return composedShapeXML(shapeID, name, placeholder, paragraphs, language)
	}
	reference := `<p:ph type="` + escapeAttribute(placeholder.Type) + `"`
	if placeholder.Index > 0 {
		reference += ` idx="` + strconv.Itoa(placeholder.Index) + `"`
	}
	reference += `/>`
	body := ""
	if placeholder.AcceptsText() {
		body = `<p:txBody>` + bodyPropertiesXML(placeholder, paragraphs) + `<a:lstStyle/>` +
			styledParagraphsXML(paragraphs, language, placeholder, style, links) + `</p:txBody>`
	}
	// A placeholder normally inherits its box from the layout, which is what keeps
	// a deck inside its template. A region the author dragged has to say where it
	// went, or the exported file would put it back.
	properties := `<p:spPr/>`
	if moved {
		properties = `<p:spPr><a:xfrm><a:off x="` + strconv.Itoa(placeholder.X) + `" y="` + strconv.Itoa(placeholder.Y) +
			`"/><a:ext cx="` + strconv.Itoa(max(placeholder.Width, 1)) + `" cy="` + strconv.Itoa(max(placeholder.Height, 1)) +
			`"/></a:xfrm></p:spPr>`
	}
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="` + escapeAttribute(name) + `"/>` +
		`<p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>` + reference + `</p:nvPr></p:nvSpPr>` + properties + body + `</p:sp>`
}

// composedShapeXML draws a text box for a region Ptium derived itself.
//
// A synthetic region has no placeholder to inherit from, so everything the
// template would normally supply — position, size, colour, typeface — is written
// out explicitly, taken from the template's own theme. The shape is a plain text
// box rather than a placeholder reference, which keeps it editable in PowerPoint
// and keeps the layout's artwork untouched behind it.
func composedShapeXML(shapeID int, name string, placeholder Placeholder, paragraphs []Paragraph, language string) string {
	geometry := `<a:xfrm><a:off x="` + strconv.Itoa(placeholder.X) + `" y="` + strconv.Itoa(placeholder.Y) + `"/>` +
		`<a:ext cx="` + strconv.Itoa(placeholder.Width) + `" cy="` + strconv.Itoa(placeholder.Height) + `"/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/>`
	anchor := ""
	if placeholder.Slot == SlotTitle {
		anchor = ` anchor="b"`
	}
	scale, reduction := autofit(placeholder, paragraphs)
	autofitXML := `<a:normAutofit/>`
	if scale < 100 {
		autofitXML = `<a:normAutofit fontScale="` + strconv.Itoa(int(scale*1000)) + `"`
		if reduction > 0 {
			autofitXML += ` lnSpcReduction="` + strconv.Itoa(reduction*1000) + `"`
		}
		autofitXML += `/>`
	}
	body := `<p:txBody><a:bodyPr wrap="square" lIns="91440" tIns="45720" rIns="91440" bIns="45720"` + anchor + `>` +
		autofitXML + `</a:bodyPr><a:lstStyle/>` + composedParagraphsXML(placeholder, paragraphs, language) + `</p:txBody>`
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="` + escapeAttribute(name) + `"/>` +
		`<p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr>` + geometry + `</p:spPr>` + body + `</p:sp>`
}

// composedParagraphsXML writes run properties explicitly, since a composed box
// inherits nothing from a placeholder.
func composedParagraphsXML(placeholder Placeholder, paragraphs []Paragraph, language string) string {
	runProperties := func(level int) string {
		size := placeholder.FontSize
		if size <= 0 {
			size = 1800
		}
		// A sub-bullet steps down a little, the way a designed template would.
		for range level {
			size = size * 88 / 100
		}
		properties := `<a:rPr lang="` + language + `" sz="` + strconv.Itoa(size) + `" dirty="0"`
		if placeholder.Bold {
			properties += ` b="1"`
		}
		properties += `>`
		if placeholder.Color != "" {
			properties += `<a:solidFill><a:srgbClr val="` + escapeAttribute(placeholder.Color) + `"/></a:solidFill>`
		}
		if font := strings.TrimSpace(placeholder.Font); font != "" && !strings.HasPrefix(font, "+") {
			properties += latinTypefaceXML(font)
		}
		return properties + `</a:rPr>`
	}
	if len(paragraphs) == 0 {
		return `<a:p><a:endParaRPr lang="` + language + `"/></a:p>`
	}
	var builder strings.Builder
	for _, paragraph := range paragraphs {
		level := min(max(paragraph.Level, 0), 8)
		text := strings.TrimSpace(paragraph.Text)
		// Bullets are drawn by the paragraph properties, and a title never gets one.
		properties := `<a:pPr`
		if level > 0 {
			properties += ` lvl="` + strconv.Itoa(level) + `"`
		}
		switch {
		case placeholder.Slot == SlotTitle || placeholder.Slot == SlotSubtitle:
			properties += `><a:buNone/></a:pPr>`
		case paragraph.Lead:
			// A lead sits flush with the region and keeps a little air under it, so
			// the points that follow read as its support rather than as its equals.
			properties += ` marL="0" indent="0"><a:spcAft><a:spcPts val="600"/></a:spcAft><a:buNone/></a:pPr>`
		default:
			properties += ` indent="-171450" marL="` + strconv.Itoa(228600+level*228600) + `"><a:buChar char="•"/></a:pPr>`
		}
		if text == "" {
			builder.WriteString(`<a:p>` + properties + `<a:endParaRPr lang="` + language + `"/></a:p>`)
			continue
		}
		builder.WriteString(`<a:p>` + properties + `<a:r>` + runProperties(level) + `<a:t>` +
			escapeText(text) + `</a:t></a:r></a:p>`)
	}
	return builder.String()
}

// bodyPropertiesXML asks PowerPoint to shrink text that would otherwise spill
// out of the template's box. The scale is precomputed so the exported file
// looks right before it is ever opened for editing.
func bodyPropertiesXML(placeholder Placeholder, paragraphs []Paragraph) string {
	scale, reduction := autofit(placeholder, paragraphs)
	if scale >= 100 {
		return `<a:bodyPr/>`
	}
	value := `<a:bodyPr><a:normAutofit fontScale="` + strconv.Itoa(int(scale*1000)) + `"`
	if reduction > 0 {
		value += ` lnSpcReduction="` + strconv.Itoa(reduction*1000) + `"`
	}
	return value + `/></a:bodyPr>`
}

func autofit(placeholder Placeholder, paragraphs []Paragraph) (scale float64, lineSpaceReduction int) {
	if placeholder.MaxLines <= 0 || len(paragraphs) == 0 {
		return 100, 0
	}
	lineEm := placeholder.LineEm
	if lineEm <= 0 {
		if placeholder.MaxChars <= 0 {
			return 100, 0
		}
		lineEm = float64(placeholder.MaxChars) / float64(placeholder.MaxLines) * referenceAdvance
	}
	needed := 0
	for _, paragraph := range paragraphs {
		// A sub-bullet loses roughly two ems of width to its indent.
		available := lineEm - float64(paragraph.Level)*2
		if available < 1 {
			available = 1
		}
		needed += wrappedLines(paragraph.Text, available)
	}
	if needed <= placeholder.MaxLines {
		return 100, 0
	}
	ratio := float64(placeholder.MaxLines) / float64(needed)
	// Font scale shrinks with the square root of the area ratio because
	// narrower glyphs also fit more characters per line.
	scale = math.Round(math.Sqrt(ratio)*100*2) / 2
	if scale < 40 {
		scale = 40
	}
	if scale >= 100 {
		return 100, 0
	}
	switch {
	case scale < 70:
		lineSpaceReduction = 20
	case scale < 90:
		lineSpaceReduction = 10
	}
	return scale, lineSpaceReduction
}

func paragraphsXML(paragraphs []Paragraph, language string) string {
	return styledParagraphsXML(paragraphs, language, Placeholder{}, Style{}, nil)
}

// paragraphsWithLinksXML is paragraphsXML for a part that can carry links of
// its own: the notes, whose relationships are the notes part's, not the
// slide's.
func paragraphsWithLinksXML(paragraphs []Paragraph, language string, links *linkTable) string {
	return styledParagraphsXML(paragraphs, language, Placeholder{}, Style{}, links)
}

// runsXML draws one paragraph as the runs it is made of: one run for text with
// no link in it, and a run of its own for each link.
//
// A caller with nowhere to put the relationships passes no table, and the words
// are still drawn without their markup — a slide that cannot carry a link must
// not print [보기](https://…) on the wall.
func runsXML(text, properties string, links *linkTable) string {
	parts := SplitRuns(text)
	if len(parts) == 1 && parts[0].Href == "" && !parts[0].Bold && !parts[0].Italic {
		return `<a:r>` + properties + `<a:t>` + escapeText(text) + `</a:t></a:r>`
	}
	var builder strings.Builder
	for _, part := range parts {
		run := properties
		if part.Bold {
			run = withRunAttribute(run, "b", "1")
		}
		if part.Italic {
			run = withRunAttribute(run, "i", "1")
		}
		if part.Href != "" {
			if id := links.id(part.Href); id != "" {
				run = withHyperlink(run, id, part.Href)
			}
		}
		builder.WriteString(`<a:r>` + run + `<a:t>` + escapeText(part.Text) + `</a:t></a:r>`)
	}
	return builder.String()
}

// withRunAttribute says something about one run on top of what the region
// already says about all of them.
//
// The attribute is replaced rather than repeated: a region set in bold whose
// author marked a word bold would otherwise write b="1" twice on the same
// element, which is not XML at all — the file would open as a deck PowerPoint
// offers to repair.
func withRunAttribute(properties, name, value string) string {
	if at := strings.Index(properties, ` `+name+`="`); at >= 0 {
		start := at + len(name) + 3
		if end := strings.Index(properties[start:], `"`); end >= 0 {
			return properties[:start] + value + properties[start+end:]
		}
	}
	return strings.Replace(properties, `<a:rPr`, `<a:rPr `+name+`="`+value+`"`, 1)
}

// withHyperlink puts the link inside a run's properties. hlinkClick is last in
// what the schema allows inside rPr, so it goes at the end of whatever the run
// already said about itself; PowerPoint colours the run from the theme's own
// hyperlink colour, and the underline is what makes it read as a link
// everywhere else.
func withHyperlink(properties, relationshipID, href string) string {
	action := ""
	if _, ok := SlideJump(href); ok {
		action = ` action="ppaction://hlinksldjump"`
	}
	// r: is declared on the slide's root element, where every other relationship
	// on the slide is named from.
	link := `<a:hlinkClick r:id="` + relationshipID + `"` + action + `/>`
	underlined := strings.Replace(properties, `<a:rPr `, `<a:rPr u="sng" `, 1)
	if cut, ok := strings.CutSuffix(underlined, `/>`); ok {
		return cut + `>` + link + `</a:rPr>`
	}
	return strings.TrimSuffix(underlined, `</a:rPr>`) + link + `</a:rPr>`
}

// styledParagraphsXML writes the paragraphs of a placeholder, stating only what
// the slide overrides.
//
// A placeholder that says nothing inherits its type from the layout, which is
// what keeps a deck inside its template — so nothing is written out unless
// somebody changed it, and then only the part they changed.
func styledParagraphsXML(paragraphs []Paragraph, language string, placeholder Placeholder, style Style,
	links *linkTable) string {
	if len(paragraphs) == 0 {
		return `<a:p><a:endParaRPr lang="` + language + `"/></a:p>`
	}
	aligned := style.alignment()
	var builder strings.Builder
	for _, paragraph := range paragraphs {
		level := min(max(paragraph.Level, 0), 8)
		properties := ""
		if level > 0 || aligned != "" || paragraph.Lead {
			properties = `<a:pPr`
			if level > 0 {
				properties += ` lvl="` + strconv.Itoa(level) + `"`
			}
			if aligned != "" {
				properties += ` algn="` + aligned + `"`
			}
			if paragraph.Lead {
				// A placeholder inherits its bullets from the template, so a lead has
				// to say it wants none — and it keeps a little air under it, so the
				// points that follow read as its support rather than as its equals.
				properties += ` marL="0" indent="0"><a:spcAft><a:spcPts val="600"/></a:spcAft><a:buNone/></a:pPr>`
			} else {
				properties += `/>`
			}
		}
		text := strings.TrimSpace(paragraph.Text)
		if text == "" {
			builder.WriteString(`<a:p>` + properties + `<a:endParaRPr lang="` + language + `"/></a:p>`)
			continue
		}
		run := styleRunXML(language, placeholder, style, level)
		builder.WriteString(`<a:p>` + properties + runsXML(text, run, links) +
			`<a:endParaRPr lang="` + language + `" dirty="0"/></a:p>`)
	}
	return builder.String()
}

// styleRunXML is the run properties for one paragraph: the language alone when
// the template's own type is kept, plus whatever the slide changed.
func styleRunXML(language string, placeholder Placeholder, style Style, level int) string {
	properties := `<a:rPr lang="` + language + `" dirty="0"`
	if style.Scale > 0 && placeholder.FontSize > 0 {
		size := placeholder.FontSize
		// A sub-bullet steps down the way a designed template sets it.
		for range level {
			size = size * 88 / 100
		}
		properties += ` sz="` + strconv.Itoa(max(size, 400)) + `"`
	}
	if style.Bold != nil {
		properties += ` b="` + boolAttribute(*style.Bold) + `"`
	}
	if style.Italic != nil {
		properties += ` i="` + boolAttribute(*style.Italic) + `"`
	}
	if style.Color == "" {
		return properties + `/>`
	}
	return properties + `><a:solidFill><a:srgbClr val="` + escapeAttribute(style.Color) + `"/></a:solidFill></a:rPr>`
}

func boolAttribute(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

// firstPictureRelationship is where a slide's own relationships start: the
// layout is rId1 and a notes slide rId2, whether or not one is there.
const firstPictureRelationship = 3

// addSlideCharts writes a chart part, its workbook and its relationships for
// every chart the slide placed, and returns where they landed.
func addSlideCharts(pkg *Package, slidePart string, index *int, charts []*ChartPart) []string {
	parts := make([]string, 0, len(charts))
	for _, chart := range charts {
		*index++
		chartPart := fmt.Sprintf("ppt/charts/chart%d.xml", *index)
		workbookPart := fmt.Sprintf("ppt/embeddings/ptiumChart%d.xlsx", *index)
		workbook := chart.chartWorkbook()
		workbookRelID := ""
		if len(workbook) > 0 {
			workbookRelID = "rId1"
			pkg.Set(workbookPart, workbook)
		}
		pkg.SetText(chartPart, chart.chartSpaceXML(workbookRelID))
		if workbookRelID != "" {
			pkg.SetText(RelationshipsPath(chartPart), relationshipsDocument(
				`<Relationship Id="rId1" Type="`+relationshipNamespace+`/package" Target="`+
					escapeAttribute(relativePath(chartPart, workbookPart))+`"/>`))
		}
		parts = append(parts, chartPart)
	}
	return parts
}

func slideRelationshipsXML(slidePart, layoutPart, notesPart string, pictures []placedPicture, charts []string,
	links []slideLink) string {
	relationships := `<Relationship Id="rId1" Type="` + relationshipNamespace + `/slideLayout" Target="` + escapeAttribute(relativePath(slidePart, layoutPart)) + `"/>`
	if notesPart != "" {
		relationships += `<Relationship Id="rId2" Type="` + relationshipNamespace + `/notesSlide" Target="` + escapeAttribute(relativePath(slidePart, notesPart)) + `"/>`
	}
	for _, placed := range pictures {
		relationships += `<Relationship Id="` + placed.RelationshipID + `" Type="` + relationshipNamespace +
			`/image" Target="` + escapeAttribute(relativePath(slidePart, placed.Part)) + `"/>`
	}
	for index, part := range charts {
		relationships += fmt.Sprintf(`<Relationship Id="rId%d" Type="%s/chart" Target="%s"/>`,
			firstPictureRelationship+len(pictures)+index, relationshipNamespace,
			escapeAttribute(relativePath(slidePart, part)))
	}
	for _, link := range links {
		// A link out of the deck is external and written as the author gave it; a
		// jump names another slide of the same package, which is a part like any
		// other and so a plain relationship to it.
		if link.Slide > 0 {
			relationships += `<Relationship Id="` + link.ID + `" Type="` + relationshipNamespace +
				`/slide" Target="` + fmt.Sprintf("slide%d.xml", link.Slide) + `"/>`
			continue
		}
		relationships += `<Relationship Id="` + link.ID + `" Type="` + relationshipNamespace +
			`/hyperlink" Target="` + escapeAttribute(link.Target) + `" TargetMode="External"/>`
	}
	return relationshipsDocument(relationships)
}

func notesRelationshipsXML(notesPart, notesMasterPart, slidePart string, links []slideLink) string {
	relationships := `<Relationship Id="rId1" Type="` + relationshipNamespace + `/notesMaster" Target="` + escapeAttribute(relativePath(notesPart, notesMasterPart)) + `"/>` +
		`<Relationship Id="rId2" Type="` + relationshipNamespace + `/slide" Target="` + escapeAttribute(relativePath(notesPart, slidePart)) + `"/>`
	for _, link := range links {
		if link.Slide > 0 {
			relationships += `<Relationship Id="` + link.ID + `" Type="` + relationshipNamespace +
				`/slide" Target="` + escapeAttribute(relativePath(notesPart, fmt.Sprintf("ppt/slides/slide%d.xml", link.Slide))) + `"/>`
			continue
		}
		relationships += `<Relationship Id="` + link.ID + `" Type="` + relationshipNamespace +
			`/hyperlink" Target="` + escapeAttribute(link.Target) + `" TargetMode="External"/>`
	}
	return relationshipsDocument(relationships)
}

func presentationRelationshipsXML(pkg *Package, slideRelIDs []string, notesMasterRelID, notesMasterPart,
	sourcePart string) string {
	var builder strings.Builder
	for _, relationship := range pkg.Relationships("ppt/presentation.xml") {
		switch relationship.ShortType() {
		case "slide", "notesSlide":
			continue
		}
		builder.WriteString(`<Relationship Id="` + escapeAttribute(relationship.ID) + `" Type="` + escapeAttribute(relationship.Type) + `" Target="` + escapeAttribute(relationship.Target) + `"`)
		if relationship.TargetMode != "" {
			builder.WriteString(` TargetMode="` + escapeAttribute(relationship.TargetMode) + `"`)
		}
		builder.WriteString(`/>`)
	}
	for index, id := range slideRelIDs {
		target := fmt.Sprintf("slides/slide%d.xml", index+1)
		builder.WriteString(`<Relationship Id="` + escapeAttribute(id) + `" Type="` + relationshipNamespace + `/slide" Target="` + target + `"/>`)
	}
	if notesMasterRelID != "" && notesMasterPart != "" {
		builder.WriteString(`<Relationship Id="` + escapeAttribute(notesMasterRelID) + `" Type="` + relationshipNamespace + `/notesMaster" Target="` + escapeAttribute(relativePath("ppt/presentation.xml", notesMasterPart)) + `"/>`)
	}
	if sourcePart != "" {
		// A part nothing points at is an orphan, and an orphan is what an editor
		// drops on the next save. The deck's own source hangs off the
		// presentation like everything else in the file.
		builder.WriteString(`<Relationship Id="rIdPtiumSource" Type="` + sourceRelationship +
			`" Target="` + escapeAttribute(relativePath("ppt/presentation.xml", sourcePart)) + `"/>`)
	}
	return relationshipsDocument(builder.String())
}

func relationshipsDocument(body string) string {
	return xmlDeclaration + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + body + `</Relationships>`
}

func rewritePresentation(presentation string, slideRelIDs []string, notesMasterRelID, language string) string {
	presentation = removeElement(presentation, "p:sldIdLst")
	presentation = removeElement(presentation, "p:custShowLst")
	var list strings.Builder
	list.WriteString(`<p:sldIdLst>`)
	for index, id := range slideRelIDs {
		list.WriteString(fmt.Sprintf(`<p:sldId id="%d" r:id="%s"/>`, 256+index, escapeAttribute(id)))
	}
	list.WriteString(`</p:sldIdLst>`)

	if notesMasterRelID != "" && !strings.Contains(presentation, "<p:notesMasterIdLst") {
		notesList := `<p:notesMasterIdLst><p:notesMasterId r:id="` + escapeAttribute(notesMasterRelID) + `"/></p:notesMasterIdLst>`
		if index := strings.Index(presentation, "</p:sldMasterIdLst>"); index >= 0 {
			cut := index + len("</p:sldMasterIdLst>")
			presentation = presentation[:cut] + notesList + presentation[cut:]
		} else {
			presentation = insertBefore(presentation, "<p:sldSz", notesList)
		}
	}
	presentation = insertBefore(presentation, "<p:sldSz", list.String())
	if !strings.Contains(presentation, "<p:sldIdLst>") {
		presentation = insertBefore(presentation, "</p:presentation>", list.String())
	}
	_ = language
	return presentation
}

func insertBefore(document, marker, value string) string {
	index := strings.Index(document, marker)
	if index < 0 {
		return document
	}
	return document[:index] + value + document[index:]
}

// removeElement deletes an element and its children, tolerating the
// self-closing form.
func removeElement(document, name string) string {
	for {
		start := strings.Index(document, "<"+name)
		if start < 0 {
			return document
		}
		rest := document[start:]
		selfClosing := strings.Index(rest, "/>")
		open := strings.Index(rest, ">")
		if open < 0 {
			return document
		}
		if selfClosing == open-1 {
			document = document[:start] + document[start+open+1:]
			continue
		}
		closeTag := "</" + name + ">"
		end := strings.Index(rest, closeTag)
		if end < 0 {
			return document
		}
		document = document[:start] + document[start+end+len(closeTag):]
	}
}

type contentTypesDocument struct {
	Defaults []struct {
		Extension   string `xml:"Extension,attr"`
		ContentType string `xml:"ContentType,attr"`
	} `xml:"Default"`
	Overrides []struct {
		PartName    string `xml:"PartName,attr"`
		ContentType string `xml:"ContentType,attr"`
	} `xml:"Override"`
}

func contentTypesXML(pkg *Package) string {
	original, _ := pkg.Text("[Content_Types].xml")
	var parsed contentTypesDocument
	_ = xml.Unmarshal([]byte(original), &parsed)

	defaults := map[string]string{
		"rels": "application/vnd.openxmlformats-package.relationships+xml",
		"xml":  "application/xml",
	}
	order := []string{"rels", "xml"}
	// A picture Ptium added needs its extension declared, or PowerPoint refuses
	// to open the file. A template that already used the format declares it
	// itself, and its own declaration wins below.
	for extension, contentType := range map[string]string{
		"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "svg": "image/svg+xml",
	} {
		if len(pkg.NamesUnder("ppt/media/ptium")) == 0 {
			break
		}
		if _, exists := defaults[extension]; !exists {
			defaults[extension] = contentType
			order = append(order, extension)
		}
	}
	// The workbook behind a chart is a part like any other, and PowerPoint reads
	// none of them unless the extension is declared.
	if len(pkg.NamesUnder("ppt/embeddings/")) > 0 {
		if _, exists := defaults["xlsx"]; !exists {
			defaults["xlsx"] = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
			order = append(order, "xlsx")
		}
	}
	sort.Strings(order[2:])
	for _, entry := range parsed.Defaults {
		key := strings.ToLower(entry.Extension)
		if _, exists := defaults[key]; !exists {
			order = append(order, key)
		}
		defaults[key] = entry.ContentType
	}

	overrides := make([][2]string, 0, len(parsed.Overrides)+len(pkg.NamesUnder("ppt/slides/")))
	seen := map[string]bool{}
	for _, entry := range parsed.Overrides {
		name := strings.TrimPrefix(entry.PartName, "/")
		if _, exists := pkg.Part(name); !exists {
			continue
		}
		contentType := entry.ContentType
		if name == "ppt/presentation.xml" {
			contentType = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
		}
		overrides = append(overrides, [2]string{"/" + name, contentType})
		seen["/"+name] = true
	}
	add := func(name, contentType string) {
		if seen["/"+name] {
			return
		}
		if _, exists := pkg.Part(name); !exists {
			return
		}
		overrides = append(overrides, [2]string{"/" + name, contentType})
		seen["/"+name] = true
	}
	for _, name := range pkg.NamesUnder("ppt/slides/") {
		if strings.Contains(name, "/_rels/") {
			continue
		}
		add(name, "application/vnd.openxmlformats-officedocument.presentationml.slide+xml")
	}
	for _, name := range pkg.NamesUnder("ppt/notesSlides/") {
		if strings.Contains(name, "/_rels/") {
			continue
		}
		add(name, "application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml")
	}
	for _, name := range pkg.NamesUnder("ppt/notesMasters/") {
		if strings.Contains(name, "/_rels/") {
			continue
		}
		add(name, "application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml")
	}
	for _, name := range pkg.NamesUnder("ppt/theme/") {
		if strings.Contains(name, "/_rels/") {
			continue
		}
		add(name, "application/vnd.openxmlformats-officedocument.theme+xml")
	}
	for _, name := range pkg.NamesUnder("ppt/charts/") {
		if strings.Contains(name, "/_rels/") {
			continue
		}
		add(name, "application/vnd.openxmlformats-officedocument.drawingml.chart+xml")
	}
	sort.SliceStable(overrides, func(i, j int) bool { return overrides[i][0] < overrides[j][0] })

	var builder strings.Builder
	builder.WriteString(xmlDeclaration + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	for _, extension := range order {
		builder.WriteString(`<Default Extension="` + escapeAttribute(extension) + `" ContentType="` + escapeAttribute(defaults[extension]) + `"/>`)
	}
	for _, entry := range overrides {
		builder.WriteString(`<Override PartName="` + escapeAttribute(entry[0]) + `" ContentType="` + escapeAttribute(entry[1]) + `"/>`)
	}
	builder.WriteString(`</Types>`)
	return builder.String()
}

func ensureNotesMaster(pkg *Package, manifest Manifest) string {
	const part = "ppt/notesMasters/notesMaster1.xml"
	const themePart = "ppt/theme/themeNotesMaster.xml"
	if _, exists := pkg.Part(part); exists {
		return part
	}
	source := ""
	if masters := pkg.RelatedParts("ppt/presentation.xml", "slideMaster"); len(masters) > 0 {
		if themeName, ok := pkg.RelatedPart(masters[0], "theme"); ok {
			if content, ok := pkg.Text(themeName); ok {
				source = content
			}
		}
	}
	if source == "" {
		source = minimalTheme(manifest.Theme)
	}
	pkg.SetText(themePart, source)
	pkg.SetText(part, notesMasterXML(manifest))
	pkg.SetText(RelationshipsPath(part), relationshipsDocument(
		`<Relationship Id="rId1" Type="`+relationshipNamespace+`/theme" Target="`+escapeAttribute(relativePath(part, themePart))+`"/>`))
	return part
}

func notesMasterXML(manifest Manifest) string {
	width, height := 6858000, 9144000
	if manifest.SlideWidth > 0 && manifest.SlideHeight > 0 && manifest.SlideWidth < manifest.SlideHeight {
		width, height = manifest.SlideWidth, manifest.SlideHeight
	}
	imageWidth := width * 3 / 4
	imageHeight := imageWidth * manifest.SlideHeight / max(manifest.SlideWidth, 1)
	imageX := (width - imageWidth) / 2
	imageY := height / 12
	bodyY := imageY + imageHeight + height/24
	bodyHeight := height - bodyY - height/12
	return xmlDeclaration + `<p:notesMaster ` + presentationNamespaces + `><p:cSld><p:spTree>` + emptyGroupHeader +
		fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image Placeholder 1"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg"/></p:nvPr></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/><a:ln w="12700"><a:solidFill><a:prstClr val="black"/></a:solidFill></a:ln></p:spPr></p:sp>`, imageX, imageY, imageWidth, imageHeight) +
		fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr><p:txBody><a:bodyPr vert="horz" wrap="square" lIns="91440" tIns="45720" rIns="91440" bIns="45720" numCol="1" anchor="t"><a:normAutofit/></a:bodyPr><a:lstStyle/><a:p><a:endParaRPr lang="en-US"/></a:p></p:txBody></p:sp>`, imageX, bodyY, imageWidth, max(bodyHeight, 914400)) +
		`</p:spTree></p:cSld><p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
		`<p:notesStyle><a:lvl1pPr marL="0" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1200" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:notesStyle></p:notesMaster>`
}

// notesWithSources is what the notes page says: what the presenter wrote, then
// where the slide's figures came from. A source nobody can see is not a source,
// and the notes page is the one place it can stand without changing the design.
// NotesWithSources is what the presenter reads: what they wrote to say, and
// under it where the slide's figures came from, stated in full rather than as
// the short line the slide itself draws.
//
// The notes page of the exported file and the printed handout are the same
// document for the same person, so they are written from here rather than each
// deciding what a presenter needs.
func NotesWithSources(slide Slide, language string) string {
	return notesWithSources(slide, language)
}

func notesWithSources(slide Slide, language string) string {
	notes := strings.TrimSpace(slide.Notes)
	if len(slide.Sources) == 0 {
		return notes
	}
	heading := "출처"
	switch describeLanguage(language) {
	case "en":
		heading = "Sources"
	case "ja":
		heading = "出典"
	case "zh":
		heading = "来源"
	}
	lines := make([]string, 0, len(slide.Sources)+1)
	if notes != "" {
		lines = append(lines, notes, "")
	}
	lines = append(lines, heading)
	for index, source := range slide.Sources {
		entry := strings.TrimSpace(source.Title)
		// One unmarked source needs no number: "1." in front of the only line
		// there is refers to nothing on the slide.
		if mark := strings.TrimSpace(source.Marker); mark != "" {
			entry = mark + ". " + entry
		} else if len(slide.Sources) > 1 {
			entry = strconv.Itoa(index+1) + ". " + entry
		}
		if locator := strings.TrimSpace(source.Locator); locator != "" {
			entry += " — " + locator
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

// notesSlideXML writes the speaker's own page, and says which links it carries:
// a link written in the notes is a link a presenter clicks while presenting from
// PowerPoint, and the address has to survive the trip to be one.
func notesSlideXML(notes, language string, slides int) (string, []slideLink) {
	var paragraphs []Paragraph
	for _, line := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		paragraphs = append(paragraphs, Paragraph{Text: line})
	}
	links := &linkTable{slides: slides}
	return xmlDeclaration + `<p:notes ` + presentationNamespaces + `><p:cSld><p:spTree>` + emptyGroupHeader +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image Placeholder 1"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg"/></p:nvPr></p:nvSpPr><p:spPr/></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:lstStyle/>` + paragraphsWithLinksXML(paragraphs, language, links) + `</p:txBody></p:sp>` +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:notes>`, links.links
}

func minimalTheme(theme Theme) string {
	color := func(name, fallback string) string {
		if value, ok := theme.Colors[name]; ok && value != "" {
			return value
		}
		return fallback
	}
	fontOr := func(value, fallback string) string {
		if strings.TrimSpace(value) != "" {
			return value
		}
		return fallback
	}
	name := theme.Name
	if strings.TrimSpace(name) == "" {
		name = "Ptium"
	}
	return xmlDeclaration + `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="` + escapeAttribute(name) + `"><a:themeElements><a:clrScheme name="` + escapeAttribute(name) + `">` +
		`<a:dk1><a:srgbClr val="` + color("dk1", "000000") + `"/></a:dk1><a:lt1><a:srgbClr val="` + color("lt1", "FFFFFF") + `"/></a:lt1>` +
		`<a:dk2><a:srgbClr val="` + color("dk2", "44546A") + `"/></a:dk2><a:lt2><a:srgbClr val="` + color("lt2", "E7E6E6") + `"/></a:lt2>` +
		`<a:accent1><a:srgbClr val="` + color("accent1", "4472C4") + `"/></a:accent1><a:accent2><a:srgbClr val="` + color("accent2", "ED7D31") + `"/></a:accent2>` +
		`<a:accent3><a:srgbClr val="` + color("accent3", "A5A5A5") + `"/></a:accent3><a:accent4><a:srgbClr val="` + color("accent4", "FFC000") + `"/></a:accent4>` +
		`<a:accent5><a:srgbClr val="` + color("accent5", "5B9BD5") + `"/></a:accent5><a:accent6><a:srgbClr val="` + color("accent6", "70AD47") + `"/></a:accent6>` +
		`<a:hlink><a:srgbClr val="` + color("hlink", "0563C1") + `"/></a:hlink><a:folHlink><a:srgbClr val="` + color("folHlink", "954F72") + `"/></a:folHlink></a:clrScheme>` +
		`<a:fontScheme name="` + escapeAttribute(name) + `"><a:majorFont><a:latin typeface="` + escapeAttribute(fontOr(theme.MajorLatin, "Aptos Display")) + `"/><a:ea typeface="` + escapeAttribute(fontOr(theme.MajorEA, "")) + `"/><a:cs typeface=""/></a:majorFont>` +
		`<a:minorFont><a:latin typeface="` + escapeAttribute(fontOr(theme.MinorLatin, "Aptos")) + `"/><a:ea typeface="` + escapeAttribute(fontOr(theme.MinorEA, "")) + `"/><a:cs typeface=""/></a:minorFont></a:fontScheme>` +
		`<a:fmtScheme name="` + escapeAttribute(name) + `"><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>` +
		`<a:lnStyleLst><a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst>` +
		`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
		`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst></a:fmtScheme></a:themeElements></a:theme>`
}

func appPropertiesXML(manifest Manifest, deck Deck) string {
	titles := make([]string, 0, len(deck.Slides)+1)
	themeName := manifest.Theme.Name
	if strings.TrimSpace(themeName) == "" {
		themeName = "Office Theme"
	}
	titles = append(titles, themeName)
	notes := 0
	for _, slide := range deck.Slides {
		title := ""
		if paragraphs, ok := slide.Fields[SlotTitle]; ok && len(paragraphs) > 0 {
			title = paragraphs[0].Text
		}
		if strings.TrimSpace(title) == "" {
			title = deck.Title
		}
		titles = append(titles, title)
		if strings.TrimSpace(slide.Notes) != "" {
			notes++
		}
	}
	var vector strings.Builder
	for _, title := range titles {
		vector.WriteString(`<vt:lpstr>` + escapeText(title) + `</vt:lpstr>`)
	}
	return xmlDeclaration + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Application>Ptium</Application><PresentationFormat>` + escapeText(manifest.AspectRatio) + `</PresentationFormat>` +
		`<Slides>` + strconv.Itoa(len(deck.Slides)) + `</Slides><Notes>` + strconv.Itoa(notes) + `</Notes><HiddenSlides>0</HiddenSlides><MMClips>0</MMClips><ScaleCrop>false</ScaleCrop>` +
		`<HeadingPairs><vt:vector size="4" baseType="variant"><vt:variant><vt:lpstr>Theme</vt:lpstr></vt:variant><vt:variant><vt:i4>1</vt:i4></vt:variant>` +
		`<vt:variant><vt:lpstr>Slide Titles</vt:lpstr></vt:variant><vt:variant><vt:i4>` + strconv.Itoa(len(deck.Slides)) + `</vt:i4></vt:variant></vt:vector></HeadingPairs>` +
		`<TitlesOfParts><vt:vector size="` + strconv.Itoa(len(titles)) + `" baseType="lpstr">` + vector.String() + `</vt:vector></TitlesOfParts>` +
		`<Company>` + escapeText(deck.Author) + `</Company><LinksUpToDate>false</LinksUpToDate><SharedDoc>false</SharedDoc><HyperlinksChanged>false</HyperlinksChanged><AppVersion>16.0000</AppVersion></Properties>`
}

func corePropertiesXML(deck Deck) string {
	now := time.Now().UTC().Format(time.RFC3339)
	author := deck.Author
	if strings.TrimSpace(author) == "" {
		author = "Ptium"
	}
	return xmlDeclaration + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + escapeText(deck.Title) + `</dc:title><dc:subject>` + escapeText(deck.Subject) + `</dc:subject><dc:creator>` + escapeText(author) + `</dc:creator>` +
		`<cp:lastModifiedBy>` + escapeText(author) + `</cp:lastModifiedBy><cp:revision>1</cp:revision>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">` + now + `</dcterms:modified></cp:coreProperties>`
}

func relativePath(from, to string) string {
	fromDir := path.Dir(strings.TrimPrefix(from, "/"))
	to = strings.TrimPrefix(to, "/")
	fromParts := strings.Split(fromDir, "/")
	toParts := strings.Split(to, "/")
	if fromDir == "." {
		fromParts = nil
	}
	common := 0
	for common < len(fromParts) && common < len(toParts)-1 && fromParts[common] == toParts[common] {
		common++
	}
	var segments []string
	for i := common; i < len(fromParts); i++ {
		segments = append(segments, "..")
	}
	segments = append(segments, toParts[common:]...)
	return strings.Join(segments, "/")
}

func normalizeLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return "ko-KR"
	}
	if strings.Contains(language, "-") {
		return language
	}
	switch strings.ToLower(language) {
	case "ko":
		return "ko-KR"
	case "en":
		return "en-US"
	case "ja":
		return "ja-JP"
	case "zh":
		return "zh-CN"
	case "es":
		return "es-ES"
	case "fr":
		return "fr-FR"
	case "de":
		return "de-DE"
	}
	return language
}

func escapeText(value string) string { return html.EscapeString(sanitizeXMLText(value)) }

func escapeAttribute(value string) string { return html.EscapeString(sanitizeXMLText(value)) }

// sanitizeXMLText drops control characters that are illegal in XML 1.0.
func sanitizeXMLText(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t', r == '\n', r == '\r':
			return ' '
		case r < 0x20, r == 0xFFFE, r == 0xFFFF:
			return -1
		}
		return r
	}, value)
}

func capitalize(value string) string {
	if value == "" {
		return "Placeholder"
	}
	runes := []rune(value)
	return strings.ToUpper(string(runes[0])) + string(runes[1:])
}

const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
const relationshipNamespace = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
const presentationNamespaces = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
const emptyGroupHeader = `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`
