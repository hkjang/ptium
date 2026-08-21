package pptx

import (
	"encoding/xml"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Analyze inspects a template package and produces the manifest that drives
// generation, preview and export.
func Analyze(pkg *Package) (Manifest, error) {
	manifest := Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000}
	presentation, ok := pkg.Text("ppt/presentation.xml")
	if !ok {
		return manifest, errors.New("the package does not contain a PowerPoint presentation")
	}
	if width, height, found := slideSize(presentation); found {
		manifest.SlideWidth, manifest.SlideHeight = width, height
	}
	manifest.SourceSlides = len(pkg.RelatedParts("ppt/presentation.xml", "slide"))
	_, manifest.HasNotesMaster = pkg.RelatedPart("ppt/presentation.xml", "notesMaster")

	masters := pkg.RelatedParts("ppt/presentation.xml", "slideMaster")
	if len(masters) == 0 {
		return manifest, errors.New("the template does not define a slide master")
	}
	manifest.MasterCount = len(masters)
	manifest.Theme = analyzeTheme(pkg, masters[0])

	used := map[string]bool{}
	for _, masterPart := range masters {
		master := analyzeMaster(pkg, masterPart, manifest.Theme)
		for _, layoutPart := range pkg.RelatedParts(masterPart, "slideLayout") {
			layout, err := analyzeLayout(pkg, layoutPart, masterPart, master, manifest.Theme, manifest.SlideWidth, manifest.SlideHeight)
			if err != nil {
				manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("%s: %v", path.Base(layoutPart), err))
				continue
			}
			layout.ID = uniqueID(layout.ID, used)
			manifest.Layouts = append(manifest.Layouts, layout)
		}
	}
	if len(manifest.Layouts) == 0 {
		return manifest, errors.New("the template does not define any usable slide layout")
	}
	manifest.finalize()
	return manifest, nil
}

// AnalyzeBytes opens and analyzes raw template bytes in one step.
func AnalyzeBytes(data []byte) (*Package, Manifest, error) {
	pkg, err := Open(data)
	if err != nil {
		return nil, Manifest{}, err
	}
	manifest, err := Analyze(pkg)
	if err != nil {
		return nil, Manifest{}, err
	}
	return pkg, manifest, nil
}

type master struct {
	Part         string
	ColorMap     map[string]string
	Background   string
	Placeholders map[string]rawPlaceholder
	TitleSize    int
	BodySizes    []int
	OtherSize    int
	TitleStyle   rawLevelStyle
	BodyStyle    rawLevelStyle
	OtherStyle   rawLevelStyle
	Decorations  []Decoration
	Artwork      []Artwork
	Fill         Background
}

type rawPlaceholder struct {
	X, Y, Width, Height int
	HasGeometry         bool
	FontSize            int
	Style               rawLevelStyle
}

func analyzeMaster(pkg *Package, part string, theme Theme) master {
	result := master{Part: part, ColorMap: map[string]string{}, Placeholders: map[string]rawPlaceholder{}, TitleSize: 4400, OtherSize: 1800}
	content, ok := pkg.Text(part)
	if !ok {
		return result
	}
	var parsed struct {
		CSld struct {
			Background *rawFillHolder `xml:"bg"`
			SpTree     rawShapeTree   `xml:"spTree"`
		} `xml:"cSld"`
		ClrMap struct {
			Bg1      string `xml:"bg1,attr"`
			Tx1      string `xml:"tx1,attr"`
			Bg2      string `xml:"bg2,attr"`
			Tx2      string `xml:"tx2,attr"`
			Accent1  string `xml:"accent1,attr"`
			Accent2  string `xml:"accent2,attr"`
			Accent3  string `xml:"accent3,attr"`
			Accent4  string `xml:"accent4,attr"`
			Accent5  string `xml:"accent5,attr"`
			Accent6  string `xml:"accent6,attr"`
			Hlink    string `xml:"hlink,attr"`
			FolHlink string `xml:"folHlink,attr"`
		} `xml:"clrMap"`
		TxStyles struct {
			Title rawTextStyle `xml:"titleStyle"`
			Body  rawTextStyle `xml:"bodyStyle"`
			Other rawTextStyle `xml:"otherStyle"`
		} `xml:"txStyles"`
	}
	if xml.Unmarshal([]byte(content), &parsed) != nil {
		return result
	}
	for key, value := range map[string]string{
		"bg1": parsed.ClrMap.Bg1, "tx1": parsed.ClrMap.Tx1, "bg2": parsed.ClrMap.Bg2, "tx2": parsed.ClrMap.Tx2,
		"accent1": parsed.ClrMap.Accent1, "accent2": parsed.ClrMap.Accent2, "accent3": parsed.ClrMap.Accent3,
		"accent4": parsed.ClrMap.Accent4, "accent5": parsed.ClrMap.Accent5, "accent6": parsed.ClrMap.Accent6,
		"hlink": parsed.ClrMap.Hlink, "folHlink": parsed.ClrMap.FolHlink,
	} {
		if value != "" {
			result.ColorMap[key] = value
		}
	}
	if size := parsed.TxStyles.Title.level(1); size > 0 {
		result.TitleSize = size
	}
	for level := 1; level <= 9; level++ {
		result.BodySizes = append(result.BodySizes, parsed.TxStyles.Body.level(level))
	}
	if size := parsed.TxStyles.Other.level(1); size > 0 {
		result.OtherSize = size
	}
	result.TitleStyle = parsed.TxStyles.Title.levelStyle(1)
	result.BodyStyle = parsed.TxStyles.Body.levelStyle(1)
	result.OtherStyle = parsed.TxStyles.Other.levelStyle(1)
	for _, shape := range parsed.CSld.SpTree.flatten() {
		placeholder := shape.placeholder()
		if placeholder == nil {
			continue
		}
		key := placeholderKey(placeholder.Type, placeholder.Idx)
		x, y, width, height, ok := shape.geometry()
		entry := rawPlaceholder{X: x, Y: y, Width: width, Height: height, HasGeometry: ok, FontSize: shape.overrideSize(1), Style: shape.overrideStyle(1)}
		result.Placeholders[key] = entry
	}
	if parsed.CSld.Background != nil {
		result.Background = parsed.CSld.Background.solidColor()
	}
	context := artworkContext{colorMap: result.ColorMap, theme: theme, relations: imageRelations(pkg, part)}
	if fill, ok := context.background(parsed.CSld.Background); ok {
		result.Fill = fill
	}
	result.Decorations = decorations(parsed.CSld.SpTree, result.ColorMap, theme)
	result.Artwork = collectArtwork(parsed.CSld.SpTree, context, nil)
	return result
}

func analyzeLayout(pkg *Package, part, masterPart string, parent master, theme Theme, slideWidth, slideHeight int) (Layout, error) {
	content, ok := pkg.Text(part)
	if !ok {
		return Layout{}, errors.New("layout part is missing")
	}
	var parsed struct {
		Type          string `xml:"type,attr"`
		ShowMasterShp string `xml:"showMasterSp,attr"`
		CSld          struct {
			Name       string         `xml:"name,attr"`
			Background *rawFillHolder `xml:"bg"`
			SpTree     rawShapeTree   `xml:"spTree"`
		} `xml:"cSld"`
	}
	if err := xml.Unmarshal([]byte(content), &parsed); err != nil {
		return Layout{}, fmt.Errorf("layout XML is not readable: %w", err)
	}
	layout := Layout{Part: part, MasterPart: masterPart, Name: strings.TrimSpace(parsed.CSld.Name), Type: parsed.Type, Placeholders: []Placeholder{}}
	if layout.Name == "" {
		layout.Name = strings.TrimSuffix(path.Base(part), ".xml")
	}
	layout.ID = slug(layout.Name)
	background := parent.Background
	if parsed.CSld.Background != nil {
		if resolved := parsed.CSld.Background.solidColor(); resolved != "" {
			background = resolved
		}
	}
	layout.Background = resolveColorReference(background, parent.ColorMap, theme)
	context := artworkContext{colorMap: parent.ColorMap, theme: theme, relations: imageRelations(pkg, part)}
	layout.Fill = parent.Fill
	if fill, ok := context.background(parsed.CSld.Background); ok {
		layout.Fill = fill
	}
	if layout.Fill.Fill == "" && len(layout.Fill.Gradient) == 0 && layout.Fill.Image == "" {
		layout.Fill.Fill = layout.Background
	}
	// The master paints first, and only when the layout does not opt out of it.
	if parsed.ShowMasterShp != "0" {
		layout.Decorations = append(layout.Decorations, parent.Decorations...)
		layout.Artwork = append(layout.Artwork, parent.Artwork...)
	}
	layout.Decorations = append(layout.Decorations, decorations(parsed.CSld.SpTree, parent.ColorMap, theme)...)
	layout.Artwork = collectArtwork(parsed.CSld.SpTree, context, layout.Artwork)
	describePictures(pkg, layout.Artwork)

	bodyIndex := 0
	for _, shape := range parsed.CSld.SpTree.flatten() {
		reference := shape.placeholder()
		if reference == nil {
			continue
		}
		kind := placeholderKind(reference.Type)
		if kind == "" {
			continue
		}
		inherited, hasInherited := inheritedPlaceholder(parent, reference.Type, reference.Idx)
		x, y, width, height, hasGeometry := shape.geometry()
		if !hasGeometry && hasInherited && inherited.HasGeometry {
			x, y, width, height, hasGeometry = inherited.X, inherited.Y, inherited.Width, inherited.Height, true
		}
		if !hasGeometry || width <= 0 || height <= 0 {
			continue
		}
		placeholder := Placeholder{
			Kind:     kind,
			Type:     normalizePlaceholderType(reference.Type),
			Index:    atoiDefault(reference.Idx, 0),
			Name:     strings.TrimSpace(shape.name()),
			X:        x,
			Y:        y,
			Width:    width,
			Height:   height,
			Vertical: strings.HasPrefix(reference.Orient, "vert") || shape.verticalText(),
			Prompt:   shape.sampleText(),
		}
		placeholder.FontSize = layoutFontSize(shape, placeholder.Type, inherited, parent)
		placeholder.Color, placeholder.Bold, placeholder.Font, placeholder.Italic, placeholder.Align =
			textStyle(shape, placeholder.Type, inherited, parent, theme)
		switch placeholder.Type {
		case "title", "ctrTitle":
			placeholder.Slot = SlotTitle
		case "subTitle":
			placeholder.Slot = SlotSubtitle
		default:
			switch kind {
			case "picture":
				placeholder.Slot = SlotPicture
			case "chart":
				placeholder.Slot = SlotChart
			case "table":
				placeholder.Slot = SlotTable
			default:
				bodyIndex++
				placeholder.Slot = SlotBody
				if bodyIndex > 1 {
					placeholder.Slot = fmt.Sprintf("%s%d", SlotBody, bodyIndex)
				}
			}
		}
		placeholder.MaxChars, placeholder.MaxLines, placeholder.LineEm = capacity(placeholder)
		layout.Placeholders = append(layout.Placeholders, placeholder)
	}
	sort.SliceStable(layout.Placeholders, func(i, j int) bool { return readingOrder(layout.Placeholders[i], layout.Placeholders[j]) })
	dedupeSlots(&layout)
	// A layout whose design lives entirely in artwork has no placeholder to write
	// into. Rather than leaving it unusable — which sends every slide to whichever
	// plain layout does have placeholders — derive regions from its free space.
	titleSize, bodySize := parent.TitleSize, parent.OtherSize
	if len(parent.BodySizes) > 0 && parent.BodySizes[0] > 0 {
		bodySize = parent.BodySizes[0]
	}
	if synthetic := synthesizeSlots(layout, theme, slideWidth, slideHeight, titleSize, bodySize); len(synthetic) > 0 {
		layout.Placeholders = append(layout.Placeholders, synthetic...)
		layout.Composed = true
	}
	layout.Role = classify(layout)
	return layout, nil
}

// decorations collects the static solid-filled shapes of a shape tree. Only
// simple rectangles are kept: they are what templates use for accent rules and
// colour blocks, and anything more elaborate is better left out of an
// approximate preview than drawn wrongly.
// imageRelations maps a part's relationship ids to the image parts they point
// at, which is how a blip fill names its picture.
func imageRelations(pkg *Package, part string) map[string]string {
	result := map[string]string{}
	for _, relationship := range pkg.Relationships(part) {
		if relationship.TargetMode == "External" || relationship.ShortType() != "image" {
			continue
		}
		result[relationship.ID] = Resolve(part, relationship.Target)
	}
	return result
}

func decorations(tree rawShapeTree, colorMap map[string]string, theme Theme) []Decoration {
	var result []Decoration
	for _, shape := range tree.flatten() {
		if shape.placeholder() != nil {
			continue
		}
		fill := shape.fill()
		if fill == "" {
			continue
		}
		x, y, width, height, ok := shape.geometry()
		if !ok {
			continue
		}
		preset := shape.geometryPreset()
		switch preset {
		case "", "rect", "roundRect", "snip1Rect", "snip2SameRect", "ellipse":
		default:
			continue
		}
		resolved := resolveColorReference(fill, colorMap, theme)
		if resolved == "" {
			continue
		}
		result = append(result, Decoration{X: x, Y: y, Width: width, Height: height, Fill: resolved,
			Round: preset == "roundRect" || preset == "ellipse"})
		if len(result) >= 12 {
			break
		}
	}
	return result
}

// inheritedPlaceholder finds the master placeholder a layout placeholder
// inherits from. Real templates are inconsistent about placeholder indexes, so
// an exact match is tried first, then the type alone, then a family of related
// types — a centred title inherits from the master title, a picture or chart
// placeholder from the master body.
func inheritedPlaceholder(parent master, phType, index string) (rawPlaceholder, bool) {
	if value, ok := parent.Placeholders[placeholderKey(phType, index)]; ok {
		return value, true
	}
	if value, ok := parent.Placeholders[placeholderKey(phType, "")]; ok {
		return value, true
	}
	family := map[string][]string{
		"ctrTitle": {"title"},
		"title":    {"title", "ctrTitle"},
		"subTitle": {"body", "subTitle"},
		"body":     {"body"},
		"obj":      {"body"},
		"pic":      {"body"},
		"chart":    {"body"},
		"tbl":      {"body"},
		"dgm":      {"body"},
		"media":    {"body"},
		"clipArt":  {"body"},
	}
	for _, candidate := range family[normalizePlaceholderType(phType)] {
		// Prefer a candidate that actually carries geometry.
		var fallback rawPlaceholder
		found := false
		for key, value := range parent.Placeholders {
			if !strings.HasPrefix(key, candidate+"/") {
				continue
			}
			if value.HasGeometry {
				return value, true
			}
			if !found {
				fallback, found = value, true
			}
		}
		if found {
			return fallback, true
		}
	}
	return rawPlaceholder{}, false
}

func layoutFontSize(shape rawShape, phType string, inherited rawPlaceholder, parent master) int {
	if size := shape.overrideSize(1); size > 0 {
		return size
	}
	if inherited.FontSize > 0 {
		return inherited.FontSize
	}
	switch phType {
	case "title", "ctrTitle":
		if parent.TitleSize > 0 {
			return parent.TitleSize
		}
		return 4400
	case "subTitle":
		if len(parent.BodySizes) > 0 && parent.BodySizes[0] > 0 {
			return parent.BodySizes[0]
		}
		return 2400
	case "dt", "ftr", "sldNum":
		return 1200
	}
	if len(parent.BodySizes) > 0 && parent.BodySizes[0] > 0 {
		return parent.BodySizes[0]
	}
	return 1800
}

// textStyle resolves the effective color, weight and typeface of a
// placeholder's first outline level by walking layout, master placeholder and
// master text-style overrides in that order.
func textStyle(shape rawShape, phType string, inherited rawPlaceholder, parent master, theme Theme) (color string, bold bool, font string, italic bool, align string) {
	candidates := []rawLevelStyle{shape.overrideStyle(1), inherited.Style}
	switch phType {
	case "title", "ctrTitle":
		candidates = append(candidates, parent.TitleStyle)
	case "dt", "ftr", "sldNum":
		candidates = append(candidates, parent.OtherStyle)
	default:
		candidates = append(candidates, parent.BodyStyle)
	}
	typeface := ""
	for _, candidate := range candidates {
		if color == "" {
			if value := candidate.color(); value != "" {
				color = resolveColorReference(value, parent.ColorMap, theme)
			}
		}
		if !bold {
			bold = candidate.bold()
		}
		if typeface == "" {
			typeface = strings.TrimSpace(candidate.DefRPr.Latin.Typeface)
		}
		if !italic {
			italic = candidate.italic()
		}
		if align == "" {
			align = candidate.align()
		}
	}
	if color == "" {
		color = resolveColorReference("tx1", parent.ColorMap, theme)
	}
	return color, bold, resolveTypeface(typeface, phType, theme), italic, align
}

func resolveTypeface(typeface, phType string, theme Theme) string {
	switch typeface {
	case "+mj-lt":
		return theme.MajorLatin
	case "+mn-lt":
		return theme.MinorLatin
	case "+mj-ea":
		return theme.MajorEA
	case "+mn-ea":
		return theme.MinorEA
	case "":
		if phType == "title" || phType == "ctrTitle" {
			return theme.MajorLatin
		}
		return theme.MinorLatin
	}
	return typeface
}

// capacity measures how much text fits in a placeholder. The line width is
// kept in em units so fitting stays script-aware; maxChars is derived from it
// with a mixed-script average purely as a number a writer can reason about.
func capacity(placeholder Placeholder) (maxChars, maxLines int, lineEm float64) {
	size := placeholder.FontSize
	if size <= 0 {
		size = 1800
	}
	const insetX = 2 * 91440
	const insetY = 2 * 45720
	width := placeholder.Width - insetX
	height := placeholder.Height - insetY
	if placeholder.Vertical {
		width, height = height, width
	}
	if width <= 0 || height <= 0 {
		return 0, 0, 0
	}
	em := float64(size) / 100 * EMUPerPoint
	lineHeight := em * 1.22
	lineEm = float64(width) / em
	lines := int(float64(height) / lineHeight)
	if lineEm < 1 {
		lineEm = 1
	}
	if lines < 1 {
		lines = 1
	}
	switch placeholder.Slot {
	case SlotTitle:
		// Title boxes are usually one line tall but carry autofit, and every
		// real deck has titles that wrap. Budgeting two lines keeps headlines
		// from being written as fragments.
		if lines < 2 {
			lines = 2
		}
		if lines > 3 {
			lines = 3
		}
	case SlotSubtitle:
		if lines < 2 {
			lines = 2
		}
		if lines > 4 {
			lines = 4
		}
	}
	return int(lineEm/referenceAdvance) * lines, lines, lineEm
}

func readingOrder(a, b Placeholder) bool {
	rank := func(p Placeholder) int {
		switch p.Slot {
		case SlotTitle:
			return 0
		case SlotSubtitle:
			return 1
		}
		return 2
	}
	if rank(a) != rank(b) {
		return rank(a) < rank(b)
	}
	// Treat rows as bands so a two-column layout keeps left-to-right order.
	const band = 457200
	if a.Y/band != b.Y/band {
		return a.Y < b.Y
	}
	return a.X < b.X
}

// dedupeSlots renumbers body slots after sorting so slot names follow reading
// order rather than XML order.
func dedupeSlots(layout *Layout) {
	bodyIndex := 0
	for index := range layout.Placeholders {
		if layout.Placeholders[index].Kind != "text" {
			continue
		}
		switch layout.Placeholders[index].Slot {
		case SlotTitle, SlotSubtitle:
			continue
		}
		bodyIndex++
		if bodyIndex == 1 {
			layout.Placeholders[index].Slot = SlotBody
			continue
		}
		layout.Placeholders[index].Slot = fmt.Sprintf("%s%d", SlotBody, bodyIndex)
	}
	pictureIndex, chartIndex, tableIndex := 0, 0, 0
	for index := range layout.Placeholders {
		switch layout.Placeholders[index].Kind {
		case "picture":
			pictureIndex++
			if pictureIndex > 1 {
				layout.Placeholders[index].Slot = fmt.Sprintf("%s%d", SlotPicture, pictureIndex)
			}
		case "chart":
			chartIndex++
			if chartIndex > 1 {
				layout.Placeholders[index].Slot = fmt.Sprintf("%s%d", SlotChart, chartIndex)
			}
		case "table":
			tableIndex++
			if tableIndex > 1 {
				layout.Placeholders[index].Slot = fmt.Sprintf("%s%d", SlotTable, tableIndex)
			}
		}
	}
}

var layoutKeywords = []struct {
	Role     string
	Patterns []string
}{
	{RoleQuote, []string{"quote", "인용", "명언", "citation"}},
	{RoleClosing, []string{"thank", "closing", "감사", "마무리", "맺음", "끝", "wrap"}},
	{RoleComparison, []string{"comparison", "비교", "vs", "대비"}},
	{RoleSection, []string{"section", "구역", "섹션", "divider", "간지", "chapter", "목차 구분"}},
	{RoleTitle, []string{"title slide", "제목 슬라이드", "표지", "cover", "커버", "타이틀"}},
	{RolePicture, []string{"picture", "그림", "이미지", "photo", "image"}},
	{RoleTable, []string{"table", "표 ", "테이블"}},
	{RoleChart, []string{"chart", "차트", "graph", "그래프"}},
}

func classify(layout Layout) string {
	role := classifyByName(layout)
	// A layout with nowhere to write more than a line cannot carry content,
	// whatever its name suggests. Several templates put a one-line eyebrow above
	// the title and nothing else; choosing that for a bulleted slide crowds every
	// point into unreadable type.
	switch role {
	case RoleContent, RoleTwoContent, RoleComparison:
		if !hasRoomForContent(layout) {
			return RoleSection
		}
	}
	return role
}

// hasRoomForContent reports whether a layout has a writable region that holds
// more than a single line.
func hasRoomForContent(layout Layout) bool {
	for _, placeholder := range layout.BodySlots() {
		if placeholder.MaxLines >= 2 {
			return true
		}
	}
	// A layout whose only region is the subtitle can still carry a lead line.
	if placeholder, ok := layout.Slot(SlotSubtitle); ok && placeholder.MaxLines >= 2 {
		return true
	}
	return false
}

func classifyByName(layout Layout) string {
	lowered := strings.ToLower(layout.Name)
	for _, entry := range layoutKeywords {
		for _, pattern := range entry.Patterns {
			if strings.Contains(lowered, pattern) {
				return entry.Role
			}
		}
	}
	switch layout.Type {
	case "title":
		return RoleTitle
	case "secHead":
		return RoleSection
	case "blank":
		return RoleBlank
	case "picTx", "clipArtAndTx", "txAndClipArt":
		return RolePicture
	case "tbl":
		return RoleTable
	case "chart", "txAndChart", "chartAndTx":
		return RoleChart
	case "twoObj", "twoColTx", "twoTxTwoObj", "objAndTx", "txAndObj", "twoObjAndTx", "txAndTwoObj":
		return RoleTwoContent
	}
	var hasTitle, hasSubtitle, hasPicture bool
	// Only a region with room for more than a line counts as a column. Templates
	// routinely put a one-line eyebrow or kicker above the title, and counting it
	// as content makes an ordinary slide look like a two-column layout.
	bodies := 0
	for _, placeholder := range layout.Placeholders {
		switch {
		case placeholder.Slot == SlotTitle:
			hasTitle = true
		case placeholder.Slot == SlotSubtitle:
			hasSubtitle = true
		case placeholder.Kind == "picture":
			hasPicture = true
		case placeholder.AcceptsText() && placeholder.MaxLines >= 2:
			bodies++
		}
	}
	switch {
	case hasTitle && hasSubtitle && bodies == 0:
		return RoleTitle
	case hasPicture && bodies <= 1:
		return RolePicture
	case bodies >= 2:
		return RoleTwoContent
	case hasTitle && bodies == 1:
		return RoleContent
	case hasTitle && bodies == 0:
		return RoleSection
	case !hasTitle && bodies == 0:
		return RoleBlank
	}
	return RoleContent
}

func placeholderKind(phType string) string {
	switch normalizePlaceholderType(phType) {
	case "title", "ctrTitle", "subTitle", "body", "obj":
		return "text"
	case "pic":
		return "picture"
	case "chart":
		return "chart"
	case "tbl":
		return "table"
	case "dgm", "clipArt", "media":
		return "graphic"
	case "dt", "ftr", "sldNum", "sldImg", "hdr":
		return ""
	}
	return "text"
}

func normalizePlaceholderType(phType string) string {
	phType = strings.TrimSpace(phType)
	if phType == "" {
		return "body"
	}
	return phType
}

func placeholderKey(phType, index string) string {
	return normalizePlaceholderType(phType) + "/" + strings.TrimSpace(index)
}

func analyzeTheme(pkg *Package, masterPart string) Theme {
	result := Theme{Colors: map[string]string{}}
	themePart, ok := pkg.RelatedPart(masterPart, "theme")
	if !ok {
		return result
	}
	content, ok := pkg.Text(themePart)
	if !ok {
		return result
	}
	var parsed struct {
		Name          string `xml:"name,attr"`
		ThemeElements struct {
			ClrScheme struct {
				Name     string      `xml:"name,attr"`
				Dk1      rawColorRef `xml:"dk1"`
				Lt1      rawColorRef `xml:"lt1"`
				Dk2      rawColorRef `xml:"dk2"`
				Lt2      rawColorRef `xml:"lt2"`
				Accent1  rawColorRef `xml:"accent1"`
				Accent2  rawColorRef `xml:"accent2"`
				Accent3  rawColorRef `xml:"accent3"`
				Accent4  rawColorRef `xml:"accent4"`
				Accent5  rawColorRef `xml:"accent5"`
				Accent6  rawColorRef `xml:"accent6"`
				Hlink    rawColorRef `xml:"hlink"`
				FolHlink rawColorRef `xml:"folHlink"`
			} `xml:"clrScheme"`
			FontScheme struct {
				Major rawFontRef `xml:"majorFont"`
				Minor rawFontRef `xml:"minorFont"`
			} `xml:"fontScheme"`
		} `xml:"themeElements"`
	}
	if xml.Unmarshal([]byte(content), &parsed) != nil {
		return result
	}
	result.Name = parsed.Name
	scheme := parsed.ThemeElements.ClrScheme
	for name, reference := range map[string]rawColorRef{
		"dk1": scheme.Dk1, "lt1": scheme.Lt1, "dk2": scheme.Dk2, "lt2": scheme.Lt2,
		"accent1": scheme.Accent1, "accent2": scheme.Accent2, "accent3": scheme.Accent3,
		"accent4": scheme.Accent4, "accent5": scheme.Accent5, "accent6": scheme.Accent6,
		"hlink": scheme.Hlink, "folHlink": scheme.FolHlink,
	} {
		if value := reference.value(); value != "" {
			result.Colors[name] = value
		}
	}
	result.MajorLatin = parsed.ThemeElements.FontScheme.Major.Latin.Typeface
	result.MinorLatin = parsed.ThemeElements.FontScheme.Minor.Latin.Typeface
	result.MajorEA = parsed.ThemeElements.FontScheme.Major.EastAsian.Typeface
	result.MinorEA = parsed.ThemeElements.FontScheme.Minor.EastAsian.Typeface
	return result
}

// resolveColorReference maps a scheme color name through the master color map
// into a concrete RRGGBB value.
func resolveColorReference(reference string, colorMap map[string]string, theme Theme) string {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return ""
	}
	if hexColorPattern.MatchString(reference) {
		return strings.ToUpper(reference)
	}
	name := reference
	if mapped, ok := colorMap[name]; ok {
		name = mapped
	}
	switch name {
	case "bg1", "lt1":
		name = "lt1"
	case "tx1", "dk1":
		name = "dk1"
	case "bg2", "lt2":
		name = "lt2"
	case "tx2", "dk2":
		name = "dk2"
	}
	return theme.Color(name)
}

var hexColorPattern = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

func slideSize(presentation string) (int, int, bool) {
	var parsed struct {
		SlideSize struct {
			CX int `xml:"cx,attr"`
			CY int `xml:"cy,attr"`
		} `xml:"sldSz"`
	}
	if xml.Unmarshal([]byte(presentation), &parsed) != nil {
		return 0, 0, false
	}
	if parsed.SlideSize.CX <= 0 || parsed.SlideSize.CY <= 0 {
		return 0, 0, false
	}
	return parsed.SlideSize.CX, parsed.SlideSize.CY, true
}

var slugPattern = regexp.MustCompile(`[^a-z0-9가-힣]+`)

func slug(value string) string {
	lowered := strings.ToLower(strings.TrimSpace(value))
	cleaned := strings.Trim(slugPattern.ReplaceAllString(lowered, "-"), "-")
	if cleaned == "" {
		return "layout"
	}
	if len([]rune(cleaned)) > 48 {
		cleaned = string([]rune(cleaned)[:48])
	}
	return cleaned
}

func uniqueID(candidate string, used map[string]bool) string {
	id := candidate
	for counter := 2; used[id]; counter++ {
		id = fmt.Sprintf("%s-%d", candidate, counter)
	}
	used[id] = true
	return id
}

func atoiDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
