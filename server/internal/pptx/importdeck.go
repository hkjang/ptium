package pptx

import (
	"encoding/xml"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
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
	// Struck is a line the author had drawn a rule through: it says the line no
	// longer holds, and nothing in this deck's markup can carry that.
	Struck bool
}

// ImportedSlide is one slide as text.
type ImportedSlide struct {
	Title   string
	Lead    string
	Bullets []ImportedLine
	Notes   string
	Role    string
	// Sources are the citations drawn on the slide. They are a line of text like
	// any other on the page, and read as one they became a point: a deck came
	// back arguing "출처: 내부 자료 2026".
	Sources []string
	// Hidden is a slide the author took out of the show without deleting it.
	// Carrying it in as an ordinary slide puts something back in front of a room
	// that somebody decided a room should not see.
	Hidden bool
	// Tables come across whole: a table is words in a grid, and Ptium draws one
	// from exactly that.
	Tables [][][]string
	// Pictures are the photographs on the slide, bytes and all. They are placed
	// into the new design's picture region rather than at the old design's
	// coordinates: a photograph positioned for one layout means nothing in
	// another, but the photograph itself is the author's.
	Pictures []ImportedPicture
	// Charts come across as their numbers. A chart part carries the figures it
	// was plotted from — that is what makes it a chart rather than a picture —
	// and those numbers are the slide's argument.
	Charts []ImportedChart
	// OtherCharts counts the plots whose form Ptium does not draw: a pie, a
	// scatter, a doughnut of a doughnut. They are reported, not invented.
	OtherCharts int
	// Struck counts the table cells the author had drawn a rule through. A deck is
	// redrawn in the design it lands in, so its colours and sizes are the new
	// design's — but a rule through a line is not styling, it is the author
	// saying that line no longer holds. There is no mark to carry it, and the
	// words arrive looking as live as the rest, so the count is reported and
	// the author is told to strike them again. A cell is counted here rather
	// than beside the points because a table's cells are carried as words in a
	// grid, with nowhere on them to hang a mark of their own.
	Struck int
}

// ImportedChart is a plot carried over from a slide, as its numbers.
type ImportedChart struct {
	// Kind is the component the numbers should be drawn as: columns, bars, line.
	Kind       string
	Categories []string
	Series     []Series
}

// ImportedPicture is a photograph carried over from a slide.
type ImportedPicture struct {
	Name string
	Data []byte
	// Caption is the alternative text the picture already had, which is the one
	// thing about a picture that cannot be worked out by looking at it.
	Caption string
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
	// The order is read first, because a link to another slide is written as its
	// position — "#3" — and a slide cannot know its own place from the inside.
	var order []string
	for _, match := range slideIDPattern.FindAllStringSubmatch(presentation, -1) {
		if part, ok := pkg.RelationshipByID("ppt/presentation.xml", match[1]); ok {
			order = append(order, part)
		}
	}
	for _, part := range order {
		if slide, ok := readSlide(pkg, part, order); ok {
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
func readSlide(pkg *Package, part string, order []string) (ImportedSlide, bool) {
	content, ok := pkg.Text(part)
	if !ok {
		return ImportedSlide{}, false
	}
	var parsed struct {
		// show="0" on the slide's own root is how the format says "not part of
		// the show"; its absence is the default.
		Show string `xml:"show,attr"`
		CSld struct {
			SpTree rawShapeTree `xml:"spTree"`
		} `xml:"cSld"`
	}
	if err := xml.Unmarshal([]byte(content), &parsed); err != nil {
		return ImportedSlide{}, false
	}
	slide := ImportedSlide{Role: slideRoleOf(pkg, part), Hidden: isOff(parsed.Show)}
	// A slide is read the way a person reads it: down the page, then across.
	// The file stores shapes in drawing order — all the text boxes, then the
	// pictures, then the frames, and within each whatever order they were last
	// touched in — so a deck written by hand came back with its argument out of
	// order, which is the one thing an import is for.
	for _, placed := range downThePage(parsed.CSld.SpTree.placed()) {
		shape := placed.Shape
		lines := shapeParagraphsWithLinks(shape, slideLinks(pkg, part, order))
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
				slide.Title = WithoutInlineMarkup(joinLines(lines))
			}
			continue
		case "subTitle":
			if slide.Lead == "" {
				// A lead is prose: "> …" carries links and emphasis the way a
				// point does, and a single point on a slide is often drawn in
				// this slot. Taking the markup out of it loses the address of a
				// link nobody can type again from looking at the slide.
				slide.Lead = joinLines(lines)
			}
			continue
		case "dt", "ftr", "sldNum", "sldImg", "hdr":
			continue
		}
		if cited := citationsIn(lines); len(cited) > 0 {
			slide.Sources = append(slide.Sources, cited...)
			continue
		}
		slide.Bullets = append(slide.Bullets, lines...)
	}
	// A slide with no title placeholder still has a title: its first line.
	if slide.Title == "" && len(slide.Bullets) > 0 {
		slide.Title = WithoutInlineMarkup(slide.Bullets[0].Text)
		slide.Bullets = slide.Bullets[1:]
	}
	// A picture and a table are read from the part itself: the shape parser does
	// not descend into a graphic frame.
	slide.Charts, slide.OtherCharts = readCharts(pkg, part)
	slide.Tables, slide.Struck = readTables(content, slideLinks(pkg, part, order))
	slide.Pictures = readPictures(pkg, part, parsed.CSld.SpTree, slideArea(pkg))
	slide.Notes = withoutRepeatedCitations(readNotes(pkg, part, order), slide.Sources)
	return slide, true
}

// shapeParagraphs reads a shape's text, one paragraph per line, keeping depth.
// markedUpRun writes a run the way deck source spells it. Emphasis around an
// empty or blank run would be markup with nothing inside it, so it is left off.
func markedUpRun(text, bold, italic, target string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	if target != "" {
		// A link's own text may not carry the brackets that delimit it.
		text = strings.NewReplacer("[", "(", "]", ")").Replace(text)
		text = "[" + text + "](" + target + ")"
		return text
	}
	// A run's own text usually carries the space that separates it from the next
	// one — "이 부분은 굵게 " — and a mark that closes on a space is not
	// emphasis, by the same rule the reader applies. Wrapping the space inside
	// the marks made the reader refuse them, so an imported deck drew
	// "**이 부분은 굵게 **" on the slide and lost the bold: asterisks on the wall
	// and in the exported file. The spaces stay outside.
	lead := text[:len(text)-len(strings.TrimLeft(text, " \t"))]
	tail := text[len(strings.TrimRight(text, " \t")):]
	core := strings.TrimRight(strings.TrimLeft(text, " \t"), " \t")
	if isOn(bold) {
		core = "**" + core + "**"
	}
	if isOn(italic) {
		core = "*" + core + "*"
	}
	return lead + core + tail
}

// isStruck reads the strike attribute, which names the rule rather than
// answering yes: "sngStrike", "dblStrike", and "noStrike" for none.
func isStruck(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return trimmed != "" && trimmed != "nostrike" && trimmed != "0" && trimmed != "false"
}

// isOff reads an attribute that is present and says no.
func isOff(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "off":
		return true
	}
	return false
}

// isOn reads the "1"/"true"/"0" a DrawingML attribute is written with.
func isOn(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on":
		return true
	}
	return false
}

// runLinkTarget resolves a run's link, if it has one this package can follow.
func runLinkTarget(hlink *struct {
	ID string `xml:"id,attr"`
}, resolve linkResolver) string {
	if hlink == nil || resolve == nil {
		return ""
	}
	target, ok := resolve(hlink.ID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(target)
}

// downThePage sorts shapes down the page and then across it.
//
// Tops within a quarter of an inch of each other are the same row: two boxes
// side by side are rarely aligned to the EMU, and sorting them strictly by top
// would put the lower-by-a-hair column first.
//
// A slide built in columns is read column by column instead. Down-the-page is
// how a reader takes an ordinary slide, but a roadmap drawn as three stages
// side by side — and every second deck has one — is read across: a stage, its
// duration, what happens in it, then the next stage. Sorting that by rows
// interleaves the three stages into one another, and a zigzag timeline (the
// middle stage drawn lower than its neighbours) comes out as 1, 3, 2.
func downThePage(shapes []placedShape) []placedShape {
	sorted := byRow(shapes)
	if columns := columnsOf(sorted); len(columns) > 1 {
		return acrossColumns(sorted, columns)
	}
	return sorted
}

// byRow is down the page and then across it.
func byRow(shapes []placedShape) []placedShape {
	const row = 228600 // a quarter inch in EMU
	sorted := make([]placedShape, len(shapes))
	copy(sorted, shapes)
	sort.SliceStable(sorted, func(a, b int) bool {
		bandA, bandB := sorted[a].Top/row, sorted[b].Top/row
		if bandA != bandB {
			return bandA < bandB
		}
		return sorted[a].Left < sorted[b].Left
	})
	return sorted
}

// span is the horizontal reach of a run of shapes.
type span struct{ left, right int }

func (s span) overlaps(other span) bool { return s.left < other.right && other.left < s.right }

// columnsOf finds the columns a slide is built in, if it is built in columns.
//
// Only what carries the argument is considered: the rules, circles and bands a
// design paints between columns span the page and would hide the very structure
// they are drawn to show. A column has to hold more than one thing — two boxes
// side by side are already read left to right — and there have to be at least
// two such columns, or this is an ordinary slide.
func columnsOf(shapes []placedShape) []span {
	reach := span{}
	var all []span
	for _, shape := range shapes {
		if shape.Width <= 0 || !hasWords(shape) {
			continue
		}
		one := span{shape.Left, shape.Left + shape.Width}
		all = append(all, one)
		if reach.right == 0 || one.left < reach.left {
			reach.left = one.left
		}
		if one.right > reach.right {
			reach.right = one.right
		}
	}
	if len(all) < 4 {
		return nil
	}
	// A title, a rule or a footnote reaches across the whole slide. Left in, it
	// bridges every column into one and hides the structure it sits above.
	wide := (reach.right - reach.left) * 55 / 100
	var spans []span
	for _, one := range all {
		if one.right-one.left <= wide {
			spans = append(spans, one)
		}
	}
	if len(spans) < 4 {
		return nil
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a].left < spans[b].left })
	var columns []span
	counts := []int{}
	for _, one := range spans {
		if len(columns) > 0 && one.left < columns[len(columns)-1].right {
			last := &columns[len(columns)-1]
			if one.right > last.right {
				last.right = one.right
			}
			counts[len(counts)-1]++
			continue
		}
		columns = append(columns, one)
		counts = append(counts, 1)
	}
	// Every column has to be a stack of its own. A heading, a list and a caption
	// that happen to fall into three bands are an ordinary slide read down the
	// page; three stacks of three are a slide built in columns.
	if len(columns) < 2 {
		return nil
	}
	for _, count := range counts {
		if count < 2 {
			return nil
		}
	}
	return columns
}

// acrossColumns reads the columns left to right, and what spans them in its own
// place: a title above them comes first, a footnote under them comes last.
func acrossColumns(sorted []placedShape, columns []span) []placedShape {
	columnOf := func(shape placedShape) int {
		if shape.Width <= 0 {
			return -1
		}
		reach := span{shape.Left, shape.Left + shape.Width}
		found := -1
		for index, column := range columns {
			if !reach.overlaps(column) {
				continue
			}
			if found >= 0 {
				return -1 // it spans more than one column
			}
			found = index
		}
		return found
	}
	firstColumnTop := 0
	for index, shape := range sorted {
		if columnOf(shape) >= 0 {
			firstColumnTop = sorted[index].Top
			break
		}
	}
	above, below := []placedShape{}, []placedShape{}
	inColumn := make([][]placedShape, len(columns))
	for _, shape := range sorted {
		switch index := columnOf(shape); {
		case index >= 0:
			inColumn[index] = append(inColumn[index], shape)
		case shape.Top < firstColumnTop:
			above = append(above, shape)
		default:
			below = append(below, shape)
		}
	}
	result := make([]placedShape, 0, len(sorted))
	result = append(result, above...)
	for _, column := range inColumn {
		result = append(result, column...)
	}
	return append(result, below...)
}

// hasWords is a shape with something written in it. Columns are found from
// these alone: the photographs and shapes a design lays over a slide sit where
// they look right rather than where the argument is, and one of them crossing
// two columns would merge them into one.
func hasWords(shape placedShape) bool {
	if shape.Shape.TxBody == nil {
		return false
	}
	for _, paragraph := range shape.Shape.TxBody.Para {
		for _, run := range paragraph.Runs {
			if strings.TrimSpace(run.Text) != "" {
				return true
			}
		}
	}
	return false
}

// saysSomething is a shape a reader would look at for what the slide says: it
// has words in it, or it is a picture or a table.
func saysSomething(shape placedShape) bool {
	if shape.Shape.BlipFill != nil || shape.Shape.NvGraphicFramePr != nil || shape.Shape.NvPicPr != nil {
		return true
	}
	if shape.Shape.TxBody == nil {
		return false
	}
	for _, paragraph := range shape.Shape.TxBody.Para {
		for _, run := range paragraph.Runs {
			if strings.TrimSpace(run.Text) != "" {
				return true
			}
		}
	}
	return false
}

// withoutRepeatedCitations drops from the notes what the slide already cites.
//
// A citation is written twice on the way out — drawn on the slide and repeated
// under the notes, where a presenter can read it — so reading both back gives
// the deck the same source twice, once as a citation and once as a sentence in
// the notes.
func withoutRepeatedCitations(notes string, sources []string) string {
	trimmed := strings.TrimSpace(notes)
	if trimmed == "" || len(sources) == 0 {
		return trimmed
	}
	// From the last mention of the word backwards. A note that says "숫자는
	// 출처와 함께 말합니다" — which is what this product's own offline writer
	// writes — has the word in it twice, and stopping at the first left the
	// citation in the notes as well as in the deck, growing by one copy on
	// every trip out and back.
	places := citationTail.FindAllStringIndex(trimmed, -1)
	for index := len(places) - 1; index >= 0; index-- {
		// Only when what follows the heading is the citations themselves: an
		// author who ends a note with the word "출처" and something else keeps it.
		if onlyTheseCitations(trimmed[places[index][1]:], sources) {
			return strings.TrimSpace(trimmed[:places[index][0]])
		}
	}
	return trimmed
}

// onlyTheseCitations says the tail of a note is the works cited and nothing
// else. What separates a title from its page is written one way where it is
// drawn and another where it is stored — "보고서 — p.42" against "보고서, p.42"
// — so the comparison is on the letters and digits alone.
func onlyTheseCitations(rest string, sources []string) bool {
	left := squashed(rest)
	if left == "" {
		return false
	}
	for _, cited := range sources {
		left = strings.Replace(left, squashed(cited), "", 1)
	}
	return left == ""
}

func squashed(text string) string {
	var kept strings.Builder
	for _, character := range text {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			kept.WriteRune(character)
		}
	}
	return kept.String()
}

// citationTail is the heading that introduces the citations repeated under the
// notes, wherever the languages put it.
var citationTail = regexp.MustCompile(`\s*(?:출처|Sources?|出典|来源)\s*[::]?\s*`)

// citationLine is how a citation is drawn: the word for "source", a colon, and
// the works cited. Several are numbered and set two spaces apart.
var citationLine = regexp.MustCompile(`^(?:출처|Sources?|出典|来源)\s*[::]\s*(.+)$`)

// citationsIn reads a shape that is a citation and nothing else. A shape with
// anything else in it is prose, and prose that mentions a source is still
// prose.
func citationsIn(lines []ImportedLine) []string {
	if len(lines) != 1 {
		return nil
	}
	found := citationLine.FindStringSubmatch(strings.TrimSpace(lines[0].Text))
	if found == nil {
		return nil
	}
	var cited []string
	for _, entry := range strings.Split(found[1], "  ") {
		entry = strings.TrimSpace(entry)
		// Several citations are numbered where they are drawn; the number is the
		// drawing's, not the author's.
		if mark := citationMark.FindStringSubmatch(entry); mark != nil {
			entry = strings.TrimSpace(mark[1])
		}
		if entry != "" {
			cited = append(cited, entry)
		}
	}
	return cited
}

var citationMark = regexp.MustCompile(`^[\p{L}\d]{1,3}[.)]\s+(.+)$`)

// linkResolver turns the relationship id a run carries into the address it
// points at. It is nil when the part has no relationships to read.
type linkResolver func(id string) (string, bool)

// slideLinks resolves a slide part's own relationships, including the external
// ones: a hyperlink points outside the package almost every time.
func slideLinks(pkg *Package, part string, order []string) linkResolver {
	return func(id string) (string, bool) {
		if strings.TrimSpace(id) == "" {
			return "", false
		}
		return pkg.LinkByID(part, id, order)
	}
}

func shapeParagraphs(shape rawShape) []ImportedLine {
	return shapeParagraphsWithLinks(shape, nil)
}

// shapeParagraphsWithLinks reads a shape's text and keeps what the runs say
// about it: a link's address, and bold and italic, written as the deck source
// spells them. Concatenating the runs and dropping their properties is how a
// deck imported from PowerPoint came back with the words of a link and no
// address at all — the one part of a link that cannot be typed again from
// looking at the slide.
func shapeParagraphsWithLinks(shape rawShape, link linkResolver) []ImportedLine {
	if shape.TxBody == nil {
		return nil
	}
	lines := make([]ImportedLine, 0, len(shape.TxBody.Para))
	for _, paragraph := range shape.TxBody.Para {
		var builder strings.Builder
		struck := false
		for _, run := range paragraph.Runs {
			if isStruck(run.RPr.Strike) && strings.TrimSpace(run.Text) != "" {
				struck = true
			}
			builder.WriteString(markedUpRun(run.Text, run.RPr.Bold, run.RPr.Italic, runLinkTarget(run.RPr.HlinkClick, link)))
		}
		text := strings.TrimSpace(builder.String())
		if text == "" || saysNothing(text) {
			// A line whose whole text is the bullet glyph its own design drew —
			// "•", "▪", "-" — is decoration that a text reader sees as a
			// paragraph. Carried across it becomes a point that says nothing,
			// and three of them arrived in a deck imported from a real file.
			continue
		}
		level := paragraph.PPr.Level
		if level < 0 || level > 4 {
			level = 0
		}
		lines = append(lines, ImportedLine{Text: text, Level: level, Struck: struck})
	}
	return lines
}

// WithoutInlineMarkup takes the inline markup back out of a line.
//
// A heading and a subtitle are slots, not prose: the deck source has no way to
// bold part of a heading, so "**목 차**" is drawn with its asterisks and shown
// with them in the deck list. Most real decks bold their title.
func WithoutInlineMarkup(text string) string {
	text = importedLinkPattern.ReplaceAllString(text, "$1")
	text = importedBoldPattern.ReplaceAllString(text, "$1")
	text = importedItalicPattern.ReplaceAllString(text, "$1")
	return strings.TrimSpace(text)
}

var (
	importedLinkPattern   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	importedBoldPattern   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	importedItalicPattern = regexp.MustCompile(`\*([^*]+)\*`)
)

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
		caption := ""
		if nonVisual := shape.nonVisual(); nonVisual != nil {
			caption = describedAs(nonVisual.CNvPr.Descr, path.Base(target))
		}
		pictures = append(pictures, ImportedPicture{Name: path.Base(target), Data: data, Area: area, Caption: caption})
	}
	return pictures
}

// describedAs is the alternative text a picture carries, if it says anything.
//
// PowerPoint fills the field with the picture's own file name when nobody
// writes anything — "image.png", "다운로드.jpg" — and so does every tool that
// writes a deck. Carrying that across would put a file name where the
// description belongs and, worse, stop this product asking for the real one.
func describedAs(descr, file string) string {
	described := strings.TrimSpace(descr)
	if described == "" || strings.EqualFold(described, strings.TrimSpace(file)) {
		return ""
	}
	// A bare file name with no words in it is a file name wherever it came from.
	if !strings.ContainsAny(described, " \t") && imageFileName.MatchString(described) {
		return ""
	}
	return described
}

// imageFileName is a name with a picture's extension and nothing else.
var imageFileName = regexp.MustCompile(`(?i)^[^\s]+\.(png|jpe?g|gif|bmp|tiff?|webp|emf|wmf|svg)$`)

// readTables reads every table on a slide as rows of cell text, and says how
// many of those cells had a rule drawn through them.
//
// A picture cannot be carried into another design, but a table is words in a
// grid and Ptium draws one from exactly that — so a table comes across as a
// table and is redrawn in the design it lands in.
func readTables(content string, link linkResolver) ([][][]string, int) {
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
		return nil, 0
	}
	var tables [][][]string
	struck := 0
	for _, frame := range parsed.CSld.SpTree.Frames {
		table := frame.Graphic.Data.Table
		if table == nil {
			continue
		}
		var rows [][]string
		for _, row := range table.Rows {
			cells := make([]string, 0, len(row.Cells))
			for _, cell := range row.Cells {
				text, ruled := cellText(cell.TxBody, link)
				if ruled {
					struck++
				}
				cells = append(cells, text)
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
		}
		if len(rows) > 1 {
			tables = append(tables, rows)
		}
	}
	return tables, struck
}

// cellText is one cell's words, joined, keeping any address they link to.
//
// A cell was read as its words alone, so the address behind "근거 문서" was
// gone — the one part of a link nobody can type again from looking at the
// slide. The emphasis is deliberately not carried: a table's header row and
// label column are bold in almost every deck because the table style made them
// so, and carrying that across marks every heading **like this** in a design
// that already sets its own. What the drawing owns stays with the drawing; what
// the author knows and the slide cannot show comes with the words.
func cellText(body *rawTxBody, link linkResolver) (string, bool) {
	if body == nil {
		return "", false
	}
	struck := false
	var parts []string
	for _, paragraph := range body.Para {
		var builder strings.Builder
		for _, run := range paragraph.Runs {
			if isStruck(run.RPr.Strike) && strings.TrimSpace(run.Text) != "" {
				struck = true
			}
			builder.WriteString(markedUpRun(run.Text, "", "", runLinkTarget(run.RPr.HlinkClick, link)))
		}
		if text := strings.TrimSpace(builder.String()); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " "), struck
}

// readNotes reads the speaker notes attached to a slide, without the copy of the
// slide's own text that a notes page carries.
func readNotes(pkg *Package, slidePart string, order []string) string {
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
		// The notes part keeps its own relationships, and a note is where an
		// author puts the address of the thing they will be asked about.
		for _, line := range shapeParagraphsWithLinks(shape, slideLinks(pkg, notesPart, order)) {
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
	// Two kinds of slide have no type of their own in PowerPoint: a closing page
	// is built on a section layout and a quotation on an ordinary content one.
	// The type is read first everywhere else, and reading it first here turned a
	// deck's own closing page into a section divider and its quotation into a
	// bullet — on the way back in from a file Ptium had written.
	if named := roleForLayoutName(name); named == RoleClosing || named == RoleQuote {
		return named
	}
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
	return roleForLayoutName(name)
}

// roleForLayoutName reads what a designer called a layout.
func roleForLayoutName(name string) string {
	lowered := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(lowered, "마무리"), strings.Contains(lowered, "맺음"),
		strings.Contains(lowered, "closing"), strings.Contains(lowered, "thank you"),
		strings.Contains(lowered, "wrap-up"):
		return RoleClosing
	case strings.Contains(lowered, "인용"), strings.Contains(lowered, "quote"):
		return RoleQuote
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

// --- charts -----------------------------------------------------------------

type rawChartSpace struct {
	Chart struct {
		PlotArea struct {
			Bar  []rawChartPlot `xml:"barChart"`
			Line []rawChartPlot `xml:"lineChart"`
			Pie  []rawChartPlot `xml:"pieChart"`
			Ring []rawChartPlot `xml:"doughnutChart"`
		} `xml:"plotArea"`
	} `xml:"chart"`
}

type rawChartPlot struct {
	Direction struct {
		Val string `xml:"val,attr"`
	} `xml:"barDir"`
	Series []rawChartSeries `xml:"ser"`
}

type rawChartSeries struct {
	Name struct {
		Ref struct {
			Cache rawChartCache `xml:"strCache"`
		} `xml:"strRef"`
	} `xml:"tx"`
	Categories struct {
		Strings struct {
			Cache rawChartCache `xml:"strCache"`
		} `xml:"strRef"`
		Numbers struct {
			Cache rawChartCache `xml:"numCache"`
		} `xml:"numRef"`
	} `xml:"cat"`
	Values struct {
		Numbers struct {
			Cache rawChartCache `xml:"numCache"`
		} `xml:"numRef"`
	} `xml:"val"`
}

type rawChartCache struct {
	Count struct {
		Val int `xml:"val,attr"`
	} `xml:"ptCount"`
	Points []struct {
		Index int    `xml:"idx,attr"`
		Value string `xml:"v"`
	} `xml:"pt"`
}

// values returns the cache in point order, with holes kept as empty strings so
// a series and its categories stay aligned.
func (c rawChartCache) values() []string {
	size := c.Count.Val
	for _, point := range c.Points {
		size = max(size, point.Index+1)
	}
	if size <= 0 {
		return nil
	}
	values := make([]string, size)
	for _, point := range c.Points {
		if point.Index >= 0 && point.Index < size {
			values[point.Index] = strings.TrimSpace(point.Value)
		}
	}
	return values
}

// readCharts reads the numbers behind a slide's charts, and counts the plots
// whose form has no component to come back as.
func readCharts(pkg *Package, slidePart string) ([]ImportedChart, int) {
	var charts []ImportedChart
	other := 0
	for _, chartPart := range pkg.RelatedParts(slidePart, "chart") {
		content, ok := pkg.Text(chartPart)
		if !ok {
			continue
		}
		var parsed rawChartSpace
		if err := xml.Unmarshal([]byte(content), &parsed); err != nil {
			other++
			continue
		}
		found := false
		for _, plot := range parsed.Chart.PlotArea.Bar {
			kind := BlockColumns
			if plot.Direction.Val == "bar" {
				kind = BlockBars
			}
			if chart, ok := importedChart(kind, plot); ok {
				charts = append(charts, chart)
				found = true
			}
		}
		for _, plot := range parsed.Chart.PlotArea.Line {
			if chart, ok := importedChart(BlockLine, plot); ok {
				charts = append(charts, chart)
				found = true
			}
		}
		// A pie is a division of one whole, which Ptium draws as a share bar —
		// the same statement, in the form that reads at slide distance.
		for _, plot := range append(parsed.Chart.PlotArea.Pie, parsed.Chart.PlotArea.Ring...) {
			if chart, ok := importedChart(BlockShare, plot); ok {
				charts = append(charts, chart)
				found = true
			}
		}
		if !found {
			other++
		}
	}
	return charts, other
}

func importedChart(kind string, plot rawChartPlot) (ImportedChart, bool) {
	chart := ImportedChart{Kind: kind}
	for _, series := range plot.Series {
		values := series.Values.Numbers.Cache.values()
		if len(values) == 0 {
			continue
		}
		points := make([]float64, 0, len(values))
		for _, value := range values {
			number, err := strconv.ParseFloat(value, 64)
			if err != nil {
				number = 0
			}
			points = append(points, number)
		}
		categories := series.Categories.Strings.Cache.values()
		if len(categories) == 0 {
			categories = series.Categories.Numbers.Cache.values()
		}
		if len(categories) > len(chart.Categories) {
			chart.Categories = categories
		}
		name := ""
		if named := series.Name.Ref.Cache.values(); len(named) > 0 {
			name = named[0]
		}
		chart.Series = append(chart.Series, Series{Name: name, Points: points})
	}
	if len(chart.Series) == 0 {
		return ImportedChart{}, false
	}
	return chart, true
}

// saysNothing reports a line that is only the marks around words: bullet
// glyphs, dashes, and the emphasis markup this importer writes around them.
func saysNothing(text string) bool {
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
		case strings.ContainsRune("•·▪◦‣∙※-–—*_~`", r):
		default:
			return false
		}
	}
	return true
}
