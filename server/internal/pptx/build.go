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
	Notes    string             `json:"notes,omitempty"`
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
func blockFrame(layout Layout, placeholder Placeholder, block Block) Frame {
	frame := Frame{X: placeholder.X, Y: placeholder.Y, Width: placeholder.Width, Height: placeholder.Height}
	for _, slot := range block.Span {
		other, ok := layout.Slot(slot)
		if !ok || other.Slot == placeholder.Slot {
			continue
		}
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
	Title    string  `json:"title"`
	Subject  string  `json:"subject,omitempty"`
	Author   string  `json:"author,omitempty"`
	Language string  `json:"language,omitempty"`
	Slides   []Slide `json:"slides"`
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
		if strings.TrimSpace(slide.Notes) != "" {
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
	notesIndex := 0
	for index, slide := range deck.Slides {
		layout, ok := manifest.Layout(slide.LayoutID)
		if !ok {
			if layout, ok = manifest.LayoutForRole(RoleContent); !ok {
				return nil, fmt.Errorf("slide %d references unknown layout %q", index+1, slide.LayoutID)
			}
		}
		slidePart := fmt.Sprintf("ppt/slides/slide%d.xml", index+1)
		var notesPart string
		if strings.TrimSpace(slide.Notes) != "" && notesMasterPart != "" {
			notesIndex++
			notesPart = fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", notesIndex)
		}
		// Pictures become package parts and slide relationships before the slide
		// itself is written, because the shape refers to them by relationship id.
		pictures := addSlidePictures(pkg, slidePart, index+1, slide, layout)
		pkg.SetText(slidePart, slideXML(layout, slide, language, design, pictures))
		pkg.SetText(RelationshipsPath(slidePart), slideRelationshipsXML(slidePart, layout.Part, notesPart, pictures))
		if notesPart != "" {
			pkg.SetText(notesPart, notesSlideXML(slide.Notes, language))
			pkg.SetText(RelationshipsPath(notesPart), notesRelationshipsXML(notesPart, notesMasterPart, slidePart))
		}
		slideRelIDs = append(slideRelIDs, fmt.Sprintf("rId%d", nextRelationshipID))
		nextRelationshipID++
	}

	notesMasterRelID := ""
	if createdNotesMaster {
		notesMasterRelID = fmt.Sprintf("rId%d", nextRelationshipID)
		nextRelationshipID++
	}
	pkg.SetText(RelationshipsPath("ppt/presentation.xml"), presentationRelationshipsXML(pkg, slideRelIDs, notesMasterRelID, notesMasterPart))

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
	RelationshipID string
	// Part is the package part the image was written to.
	Part    string
	Picture Picture
}

// addSlidePictures writes each of a slide's images into the package and returns
// them with the relationship ids the slide will use.
func addSlidePictures(pkg *Package, slidePart string, position int, slide Slide, layout Layout) []placedPicture {
	if len(slide.Pictures) == 0 {
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

	placed := make([]placedPicture, 0, len(slots))
	// Slide relationship ids start after the layout and any notes slide.
	next := 3
	for index, slot := range slots {
		picture := slide.Pictures[slot]
		if len(picture.Data) == 0 {
			continue
		}
		part := fmt.Sprintf("ppt/media/ptium%d-%d.%s", position, index+1, picture.extension())
		pkg.Set(part, picture.Data)
		placed = append(placed, placedPicture{
			Slot: slot, RelationshipID: fmt.Sprintf("rId%d", next+index), Part: part, Picture: picture,
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

func slideXML(layout Layout, slide Slide, language string, design Design, pictures []placedPicture) string {
	var shapes, components strings.Builder
	shapeID := 2
	placedBySlot := map[string]placedPicture{}
	for _, placed := range pictures {
		placedBySlot[placed.Slot] = placed
	}
	spanned := slide.spannedSlots()
	for _, placeholder := range layout.Placeholders {
		if spanned[placeholder.Slot] {
			// A component placed elsewhere covers this region.
			continue
		}
		// An image replaces whatever the slot would otherwise hold.
		if placed, ok := placedBySlot[placeholder.Slot]; ok {
			components.WriteString(pictureXML(shapeID, placeholder, placed))
			shapeID++
			continue
		}
		// A component replaces its placeholder rather than sitting on top of
		// it, so the exported slide has no empty text box behind the drawing.
		if block, ok := slide.Blocks[placeholder.Slot]; ok && placeholder.AcceptsText() {
			frame := blockFrame(layout, placeholder, block)
			if component := RenderBlock(design, frame, block); len(component.Primitives) > 0 {
				markup, next := component.DrawingML(shapeID)
				components.WriteString(markup)
				shapeID = next
				continue
			}
		}
		paragraphs := slide.Fields[placeholder.Slot]
		if len(paragraphs) == 0 && placeholder.AcceptsText() {
			continue
		}
		shapes.WriteString(placeholderShapeXML(shapeID, placeholder, paragraphs, language))
		shapeID++
	}
	return xmlDeclaration + `<p:sld ` + presentationNamespaces + `><p:cSld><p:spTree>` + emptyGroupHeader +
		shapes.String() + components.String() + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`
}

func placeholderShapeXML(shapeID int, placeholder Placeholder, paragraphs []Paragraph, language string) string {
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
		body = `<p:txBody>` + bodyPropertiesXML(placeholder, paragraphs) + `<a:lstStyle/>` + paragraphsXML(paragraphs, language) + `</p:txBody>`
	}
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + strconv.Itoa(shapeID) + `" name="` + escapeAttribute(name) + `"/>` +
		`<p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>` + reference + `</p:nvPr></p:nvSpPr><p:spPr/>` + body + `</p:sp>`
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
			properties += `<a:latin typeface="` + escapeAttribute(font) + `"/><a:ea typeface="` + escapeAttribute(font) + `"/>`
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
		if placeholder.Slot == SlotTitle || placeholder.Slot == SlotSubtitle {
			properties += `><a:buNone/></a:pPr>`
		} else {
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
	if len(paragraphs) == 0 {
		return `<a:p><a:endParaRPr lang="` + language + `"/></a:p>`
	}
	var builder strings.Builder
	for _, paragraph := range paragraphs {
		properties := ""
		if paragraph.Level > 0 {
			level := paragraph.Level
			if level > 8 {
				level = 8
			}
			properties = `<a:pPr lvl="` + strconv.Itoa(level) + `"/>`
		}
		text := strings.TrimSpace(paragraph.Text)
		if text == "" {
			builder.WriteString(`<a:p>` + properties + `<a:endParaRPr lang="` + language + `"/></a:p>`)
			continue
		}
		builder.WriteString(`<a:p>` + properties + `<a:r><a:rPr lang="` + language + `" dirty="0"/><a:t>` +
			escapeText(text) + `</a:t></a:r><a:endParaRPr lang="` + language + `" dirty="0"/></a:p>`)
	}
	return builder.String()
}

func slideRelationshipsXML(slidePart, layoutPart, notesPart string, pictures []placedPicture) string {
	relationships := `<Relationship Id="rId1" Type="` + relationshipNamespace + `/slideLayout" Target="` + escapeAttribute(relativePath(slidePart, layoutPart)) + `"/>`
	if notesPart != "" {
		relationships += `<Relationship Id="rId2" Type="` + relationshipNamespace + `/notesSlide" Target="` + escapeAttribute(relativePath(slidePart, notesPart)) + `"/>`
	}
	for _, placed := range pictures {
		relationships += `<Relationship Id="` + placed.RelationshipID + `" Type="` + relationshipNamespace +
			`/image" Target="` + escapeAttribute(relativePath(slidePart, placed.Part)) + `"/>`
	}
	return relationshipsDocument(relationships)
}

func notesRelationshipsXML(notesPart, notesMasterPart, slidePart string) string {
	return relationshipsDocument(
		`<Relationship Id="rId1" Type="` + relationshipNamespace + `/notesMaster" Target="` + escapeAttribute(relativePath(notesPart, notesMasterPart)) + `"/>` +
			`<Relationship Id="rId2" Type="` + relationshipNamespace + `/slide" Target="` + escapeAttribute(relativePath(notesPart, slidePart)) + `"/>`)
}

func presentationRelationshipsXML(pkg *Package, slideRelIDs []string, notesMasterRelID, notesMasterPart string) string {
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

func notesSlideXML(notes, language string) string {
	var paragraphs []Paragraph
	for _, line := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		paragraphs = append(paragraphs, Paragraph{Text: line})
	}
	return xmlDeclaration + `<p:notes ` + presentationNamespaces + `><p:cSld><p:spTree>` + emptyGroupHeader +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image Placeholder 1"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg"/></p:nvPr></p:nvSpPr><p:spPr/></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:lstStyle/>` + paragraphsXML(paragraphs, language) + `</p:txBody></p:sp>` +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:notes>`
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
