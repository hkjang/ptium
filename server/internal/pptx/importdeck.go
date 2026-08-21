package pptx

import (
	"encoding/xml"
	"path"
	"regexp"
	"strings"
)

// Reading a deck someone already has.
//
// Ptium's whole premise is that a company's own template is the design. The
// other half of that premise is the decks already written in it: last quarter's
// report, the pitch that worked, the standard introduction. Reading one back in
// turns it into deck source — text — which can then be recompiled into any
// template, edited as words, or handed to the model to rewrite.
//
// What comes back is the argument, not the artwork: titles, points, speaker
// notes and how many pictures, tables and charts were on each slide. A picture
// cannot be carried into another design at a different aspect ratio and be
// trusted to look right, so the import says what it left behind rather than
// pretending.

// ImportedLine is one point, at the depth it was written.
type ImportedLine struct {
	Text  string
	Level int
}

// ImportedSlide is one slide as text.
type ImportedSlide struct {
	Title   string
	Lead    string
	Bullets []ImportedLine
	Notes   string
	Role    string
	// Tables come across whole: a table is words in a grid, and Ptium draws one
	// from exactly that.
	Tables [][][]string
	// Pictures are the photographs on the slide, bytes and all. They are placed
	// into the new design's picture region rather than at the old design's
	// coordinates: a photograph positioned for one layout means nothing in
	// another, but the photograph itself is the author's.
	Pictures []ImportedPicture
	Charts   int
}

// ImportedPicture is a photograph carried over from a slide.
type ImportedPicture struct {
	Name string
	Data []byte
	// Area is how much of the slide the picture covered, in per-mille. A logo in
	// the corner is not the slide's illustration.
	Area int
}

// ImportedDeck is a whole deck as text.
type ImportedDeck struct {
	Title  string
	Slides []ImportedSlide
}

// slideIDPattern reads the slide order out of the presentation part.
var slideIDPattern = regexp.MustCompile(`<p:sldId[^>]*r:id="([^"]+)"`)

// ReadDeck reads the slides of a stored PowerPoint package.
func ReadDeck(pkg *Package) ImportedDeck {
	deck := ImportedDeck{}
	presentation, ok := pkg.Text("ppt/presentation.xml")
	if !ok {
		return deck
	}
	for _, match := range slideIDPattern.FindAllStringSubmatch(presentation, -1) {
		part, ok := pkg.RelationshipByID("ppt/presentation.xml", match[1])
		if !ok {
			continue
		}
		if slide, ok := readSlide(pkg, part); ok {
			deck.Slides = append(deck.Slides, slide)
		}
	}
	if len(deck.Slides) > 0 {
		deck.Title = deck.Slides[0].Title
	}
	if properties, ok := pkg.Text("docProps/core.xml"); ok {
		if title := betweenTags(properties, "dc:title"); strings.TrimSpace(title) != "" {
			deck.Title = strings.TrimSpace(title)
		}
	}
	return deck
}

// readSlide turns one slide part into text.
func readSlide(pkg *Package, part string) (ImportedSlide, bool) {
	content, ok := pkg.Text(part)
	if !ok {
		return ImportedSlide{}, false
	}
	var parsed struct {
		CSld struct {
			SpTree rawShapeTree `xml:"spTree"`
		} `xml:"cSld"`
	}
	if err := xml.Unmarshal([]byte(content), &parsed); err != nil {
		return ImportedSlide{}, false
	}
	slide := ImportedSlide{Role: slideRoleOf(pkg, part)}
	for _, shape := range parsed.CSld.SpTree.flatten() {
		lines := shapeParagraphs(shape)
		if len(lines) == 0 {
			continue
		}
		reference := shape.placeholder()
		phType := ""
		if reference != nil {
			phType = normalizePlaceholderType(reference.Type)
		}
		switch phType {
		case "title", "ctrTitle":
			if slide.Title == "" {
				slide.Title = joinLines(lines)
			}
			continue
		case "subTitle":
			if slide.Lead == "" {
				slide.Lead = joinLines(lines)
			}
			continue
		case "dt", "ftr", "sldNum", "sldImg", "hdr":
			continue
		}
		slide.Bullets = append(slide.Bullets, lines...)
	}
	// A slide with no title placeholder still has a title: its first line.
	if slide.Title == "" && len(slide.Bullets) > 0 {
		slide.Title = slide.Bullets[0].Text
		slide.Bullets = slide.Bullets[1:]
	}
	// A picture, a table and a chart are counted from the part itself: the shape
	// parser does not descend into a graphic frame, and what matters here is only
	// how much of the slide the words leave behind.
	slide.Charts = strings.Count(content, "/chart\"")
	slide.Tables = readTables(content)
	slide.Pictures = readPictures(pkg, part, parsed.CSld.SpTree, slideArea(pkg))
	slide.Notes = readNotes(pkg, part)
	return slide, true
}

// shapeParagraphs reads a shape's text, one paragraph per line, keeping depth.
func shapeParagraphs(shape rawShape) []ImportedLine {
	if shape.TxBody == nil {
		return nil
	}
	lines := make([]ImportedLine, 0, len(shape.TxBody.Para))
	for _, paragraph := range shape.TxBody.Para {
		var builder strings.Builder
		for _, run := range paragraph.Runs {
			builder.WriteString(run.Text)
		}
		text := strings.TrimSpace(builder.String())
		if text == "" {
			continue
		}
		level := paragraph.PPr.Level
		if level < 0 || level > 4 {
			level = 0
		}
		lines = append(lines, ImportedLine{Text: text, Level: level})
	}
	return lines
}

func joinLines(lines []ImportedLine) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, line.Text)
	}
	return strings.Join(parts, " ")
}

// slideArea is how big a slide is, for judging how much of it a picture covered.
func slideArea(pkg *Package) int {
	presentation, ok := pkg.Text("ppt/presentation.xml")
	if !ok {
		return 0
	}
	size := regexp.MustCompile(`<p:sldSz[^>]*cx="(\d+)"[^>]*cy="(\d+)"`).FindStringSubmatch(presentation)
	if len(size) != 3 {
		return 0
	}
	width, height := atoiDefault(size[1], 0), atoiDefault(size[2], 0)
	return width * height / 1000
}

// readPictures carries a slide's photographs over, bytes and all.
//
// Where they sat is not carried: coordinates chosen for one design mean nothing
// in another. What is carried is the picture itself, into the region the new
// design keeps for one — which is what a person redoing the deck by hand would
// do with it.
func readPictures(pkg *Package, slidePart string, tree rawShapeTree, slideArea int) []ImportedPicture {
	var pictures []ImportedPicture
	for _, shape := range tree.flatten() {
		fill := shape.picture()
		if fill == nil || strings.TrimSpace(fill.Blip.Embed) == "" {
			continue
		}
		target, ok := pkg.RelationshipByID(slidePart, fill.Blip.Embed)
		if !ok {
			continue
		}
		data, ok := pkg.Part(target)
		if !ok || len(data) == 0 {
			continue
		}
		_, _, width, height, hasGeometry := shape.geometry()
		area := 0
		if hasGeometry && slideArea > 0 {
			area = int(int64(width) * int64(height) * 1000 / int64(slideArea))
		}
		pictures = append(pictures, ImportedPicture{Name: path.Base(target), Data: data, Area: area})
	}
	return pictures
}

// readTables reads every table on a slide as rows of cell text.
//
// A picture cannot be carried into another design, but a table is words in a
// grid and Ptium draws one from exactly that — so a table comes across as a
// table and is redrawn in the design it lands in.
func readTables(content string) [][][]string {
	var parsed struct {
		CSld struct {
			SpTree struct {
				Frames []struct {
					Graphic struct {
						Data struct {
							Table *struct {
								Rows []struct {
									Cells []struct {
										TxBody *rawTxBody `xml:"txBody"`
									} `xml:"tc"`
								} `xml:"tr"`
							} `xml:"tbl"`
						} `xml:"graphicData"`
					} `xml:"graphic"`
				} `xml:"graphicFrame"`
			} `xml:"spTree"`
		} `xml:"cSld"`
	}
	if err := xml.Unmarshal([]byte(content), &parsed); err != nil {
		return nil
	}
	var tables [][][]string
	for _, frame := range parsed.CSld.SpTree.Frames {
		table := frame.Graphic.Data.Table
		if table == nil {
			continue
		}
		var rows [][]string
		for _, row := range table.Rows {
			cells := make([]string, 0, len(row.Cells))
			for _, cell := range row.Cells {
				cells = append(cells, cellText(cell.TxBody))
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
		}
		if len(rows) > 1 {
			tables = append(tables, rows)
		}
	}
	return tables
}

// cellText is one cell's words, joined.
func cellText(body *rawTxBody) string {
	if body == nil {
		return ""
	}
	var parts []string
	for _, paragraph := range body.Para {
		var builder strings.Builder
		for _, run := range paragraph.Runs {
			builder.WriteString(run.Text)
		}
		if text := strings.TrimSpace(builder.String()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

// readNotes reads the speaker notes attached to a slide, without the copy of the
// slide's own text that a notes page carries.
func readNotes(pkg *Package, slidePart string) string {
	notesPart, ok := pkg.RelatedPart(slidePart, "notesSlide")
	if !ok {
		return ""
	}
	content, ok := pkg.Text(notesPart)
	if !ok {
		return ""
	}
	var parsed struct {
		CSld struct {
			SpTree rawShapeTree `xml:"spTree"`
		} `xml:"cSld"`
	}
	if err := xml.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	var notes []string
	for _, shape := range parsed.CSld.SpTree.flatten() {
		reference := shape.placeholder()
		if reference == nil || normalizePlaceholderType(reference.Type) != "body" {
			continue
		}
		for _, line := range shapeParagraphs(shape) {
			notes = append(notes, line.Text)
		}
	}
	return strings.TrimSpace(strings.Join(notes, " "))
}

// slideRoleOf reads the kind of slide from the layout it was built on, so a
// section divider stays a section divider in whatever design it lands in.
func slideRoleOf(pkg *Package, slidePart string) string {
	layoutPart, ok := pkg.RelatedPart(slidePart, "slideLayout")
	if !ok {
		return ""
	}
	content, ok := pkg.Text(layoutPart)
	if !ok {
		return ""
	}
	var parsed struct {
		Type string `xml:"type,attr"`
		CSld struct {
			Name string `xml:"name,attr"`
		} `xml:"cSld"`
	}
	if err := xml.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	return roleForLayoutType(parsed.Type, parsed.CSld.Name)
}

// roleForLayoutType maps a PowerPoint layout type to the kind of slide it is.
func roleForLayoutType(layoutType, name string) string {
	switch layoutType {
	case "title":
		return RoleTitle
	case "secHead":
		return RoleSection
	case "obj", "tx", "objTx", "txAndObj", "objAndTx", "txOverObj", "objOverTx2":
		// The ordinary content layouts. Naming them matters: without a role the
		// deck's position rules decide, and the last slide of an imported deck
		// would become a closing page — losing its points to a layout designed to
		// hold one line.
		return RoleContent
	case "twoObj", "twoTxTwoObj", "twoObjAndTx", "twoObjOverTx":
		return RoleTwoContent
	case "picTx", "txAndPic", "picOnly":
		return RolePicture
	case "blank":
		return RoleBlank
	case "titleOnly":
		// A title-only layout is what people reach for when they are about to draw
		// something themselves. What such a slide is depends on what it holds, not
		// on the empty layout it was built from.
		return ""
	}
	lowered := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(lowered, "title and content") || strings.Contains(lowered, "제목 및 내용"):
		return RoleContent
	case strings.Contains(lowered, "title slide") || strings.Contains(lowered, "표지"):
		return RoleTitle
	case strings.Contains(lowered, "section") || strings.Contains(lowered, "간지"):
		return RoleSection
	case strings.Contains(lowered, "two") || strings.Contains(lowered, "2단"):
		return RoleTwoContent
	case strings.Contains(lowered, "comparison") || strings.Contains(lowered, "비교"):
		return RoleComparison
	case strings.Contains(lowered, "picture") || strings.Contains(lowered, "그림"):
		return RolePicture
	}
	return ""
}

// betweenTags pulls the text out of an XML element, for the few document
// properties worth reading without parsing the whole part.
func betweenTags(document, tag string) string {
	open := "<" + tag + ">"
	start := strings.Index(document, open)
	if start < 0 {
		return ""
	}
	rest := document[start+len(open):]
	end := strings.Index(rest, "</"+tag+">")
	if end < 0 {
		return ""
	}
	return rest[:end]
}
