package pptx

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Ptium ships a library of ready designs so a deck looks considered before any
// customer template is uploaded. A design is a palette crossed with a layout
// family: the palette decides colour and typography, the family decides
// composition. Both are expressed as data, so the same tested layout code
// produces every one of them.

// BuiltinPalette is the colour and type identity of a design.
type BuiltinPalette struct {
	Key     string
	Name    string
	Dark    bool
	Surface string
	Ink     string
	// Accents are the theme's six accent slots. The first is the brand hue used
	// for rules, emphasis and figures; the rest extend the categorical order.
	// Every set below was checked for lightness band, chroma floor, adjacent
	// separation under normal and dichromatic vision, and surface contrast.
	Accents    [6]string
	MajorLatin string
	MinorLatin string
	EastAsian  string
	Note       string
}

// builtinPalettes is the validated palette library.
var builtinPalettes = []BuiltinPalette{
	{
		Key: "slate", Name: "Slate", Dark: false,
		Surface: "FFFFFF", Ink: "15181D",
		Accents:    [6]string{"2563EB", "EB6834", "1BAF7A", "EDA100", "E87BA4", "008300"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "중립적인 밝은 배경에 선명한 블루. 사내 업무 보고에 가장 안전한 선택입니다.",
	},
	{
		Key: "azure", Name: "Azure", Dark: false,
		Surface: "F5F9FF", Ink: "10233D",
		Accents:    [6]string{"0B6BCB", "EB6834", "1BAF7A", "EDA100", "E87BA4", "008300"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "차분한 블루 틴트 배경. 금융·공공 부문 보고에 어울립니다.",
	},
	{
		Key: "crimson", Name: "Crimson", Dark: false,
		Surface: "FFFBFB", Ink: "1B1112",
		Accents:    [6]string{"C81E33", "2A78D6", "EB6834", "1BAF7A", "EDA100", "E87BA4"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "밝은 배경에 크림슨 액센트. 리스크와 의사결정 자료에 씁니다.",
	},
	{
		Key: "coral", Name: "Coral", Dark: false,
		Surface: "FFFBF8", Ink: "241A16",
		Accents:    [6]string{"DE5B3B", "2A78D6", "1BAF7A", "EDA100", "E87BA4", "008300"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "부드러운 웜 배경에 코랄 액센트. 브랜드·마케팅 발표용입니다.",
	},
	{
		Key: "ivory", Name: "Ivory", Dark: false,
		Surface: "FBF8F1", Ink: "1E1B16",
		Accents:    [6]string{"A8551E", "2A78D6", "EB6834", "1BAF7A", "EDA100", "E87BA4"},
		MajorLatin: "Georgia", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "따뜻한 아이보리와 세리프 제목. 제안서와 연차 보고서 톤입니다.",
	},
	{
		Key: "sand", Name: "Sand", Dark: false,
		Surface: "F7F3EA", Ink: "241F17",
		Accents:    [6]string{"B26A12", "2A78D6", "EB6834", "1BAF7A", "EDA100", "E87BA4"},
		MajorLatin: "Georgia", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "모래빛 배경과 세리프 제목. 리서치와 정책 문서에 맞습니다.",
	},
	{
		Key: "midnight", Name: "Midnight", Dark: true,
		Surface: "0E1526", Ink: "F2F5FB",
		Accents:    [6]string{"5B8DEF", "D95926", "199E70", "6B7B93", "8B98AC", "AEB8C6"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "깊은 네이비 다크 테마. 대형 화면 발표와 임원 브리핑에 강한 인상을 줍니다.",
	},
	{
		Key: "graphite", Name: "Graphite", Dark: true,
		Surface: "1B1D21", Ink: "F3F4F6",
		Accents:    [6]string{"C98500", "3987E5", "D95926", "199E70", "9085E9", "E66767"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "중립 그레이 다크 테마에 앰버 액센트. 기술 리뷰와 아키텍처 설명용입니다.",
	},
	{
		Key: "forest", Name: "Forest", Dark: true,
		Surface: "0D1F19", Ink: "EAF3EE",
		Accents:    [6]string{"17A574", "3987E5", "D95926", "9085E9", "E66767", "B05FC4"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "딥 그린 다크 테마. 지속가능성, ESG, 인프라 주제에 씁니다.",
	},
	{
		Key: "plum", Name: "Plum", Dark: true,
		Surface: "1A1024", Ink: "F6F1FC",
		Accents:    [6]string{"8B5CF6", "D95926", "199E70", "9085E9", "E66767", "B05FC4"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "딥 퍼플 다크 테마. 제품 출시와 비전 발표에 어울립니다.",
	},
}

// layoutFamily is the composition half of a design: where the title sits, how
// wide the margins are, and what marks the slide as belonging to this family.
type layoutFamily struct {
	Key  string
	Name string
	Note string

	Margin     int
	TitleSize  int
	BodySize   int
	CoverSize  int
	TitleAlign string // l or ctr on content slides
	CoverAlign string
	// Rule draws a short accent rule above the title on content slides.
	Rule bool
	// Hairline draws a full-width hairline under the title instead of a rule.
	Hairline bool
	// Rail fills a narrow band down the left edge of every slide.
	Rail int
	// Panel puts the title on a filled accent band across the top.
	Panel bool
	// Eyebrow reserves a small line above the title for a section label.
	Eyebrow bool
}

const (
	slideWidth  = 12192000
	slideHeight = 6858000
)

var layoutFamilies = []layoutFamily{
	{
		Key: "classic", Name: "Classic", Note: "제목 위 액센트 룰, 좌측 정렬, 넉넉한 본문",
		Margin: 838200, TitleSize: 4000, BodySize: 1800, CoverSize: 5400,
		TitleAlign: "l", CoverAlign: "l", Rule: true,
	},
	{
		Key: "rail", Name: "Rail", Note: "좌측 액센트 레일이 모든 슬라이드를 묶어 주는 구성",
		Margin: 1097280, TitleSize: 3800, BodySize: 1800, CoverSize: 5000,
		TitleAlign: "l", CoverAlign: "l", Rail: 182880,
	},
	{
		Key: "centered", Name: "Centered", Note: "표지와 구역을 중앙 정렬한 키노트형 구성",
		Margin: 1143000, TitleSize: 3800, BodySize: 1800, CoverSize: 5000,
		TitleAlign: "l", CoverAlign: "ctr",
	},
	{
		Key: "panel", Name: "Panel", Note: "제목이 상단 색 패널에 놓이는 강한 위계",
		Margin: 838200, TitleSize: 3400, BodySize: 1800, CoverSize: 5000,
		TitleAlign: "l", CoverAlign: "l", Panel: true,
	},
	{
		Key: "editorial", Name: "Editorial", Note: "얇은 헤어라인과 아이브로우를 쓰는 편집 디자인",
		Margin: 1188720, TitleSize: 3400, BodySize: 1700, CoverSize: 4800,
		TitleAlign: "l", CoverAlign: "l", Hairline: true, Eyebrow: true,
	},
	{
		Key: "minimal", Name: "Minimal", Note: "장식을 걷어내고 여백과 타이포만 남긴 구성",
		Margin: 1371600, TitleSize: 3200, BodySize: 1700, CoverSize: 4400,
		TitleAlign: "l", CoverAlign: "l",
	},
}

// BuiltinDesign is one shipped template: a palette in a layout family.
type BuiltinDesign struct {
	Key     string
	Name    string
	Palette BuiltinPalette
	Family  layoutFamily
}

// Description is the sentence shown in the template library.
func (d BuiltinDesign) Description() string {
	return fmt.Sprintf("%s · %s. %s", d.Family.Name, d.Family.Note, d.Palette.Note)
}

// builtinPairs assigns three layout families to every palette, so the library
// covers each family five times and each palette three times without repeating
// a combination.
var builtinPairs = map[string][3]string{
	"slate":    {"classic", "panel", "minimal"},
	"azure":    {"classic", "rail", "centered"},
	"crimson":  {"classic", "editorial", "panel"},
	"coral":    {"rail", "centered", "editorial"},
	"ivory":    {"editorial", "centered", "classic"},
	"sand":     {"editorial", "minimal", "panel"},
	"midnight": {"panel", "rail", "minimal"},
	"graphite": {"minimal", "classic", "rail"},
	"forest":   {"panel", "centered", "minimal"},
	"plum":     {"rail", "centered", "editorial"},
}

// BuiltinDesigns returns the shipped library in a stable order.
func BuiltinDesigns() []BuiltinDesign {
	families := map[string]layoutFamily{}
	for _, family := range layoutFamilies {
		families[family.Key] = family
	}
	result := make([]BuiltinDesign, 0, len(builtinPalettes)*3)
	for _, palette := range builtinPalettes {
		for _, familyKey := range builtinPairs[palette.Key] {
			family, ok := families[familyKey]
			if !ok {
				continue
			}
			result = append(result, BuiltinDesign{
				Key:     palette.Key + "-" + family.Key,
				Name:    "Ptium " + palette.Name + " " + family.Name,
				Palette: palette,
				Family:  family,
			})
		}
	}
	return result
}

// BuiltinDesignKeys lists every shipped design key.
func BuiltinDesignKeys() []string {
	designs := BuiltinDesigns()
	keys := make([]string, 0, len(designs))
	for _, design := range designs {
		keys = append(keys, design.Key)
	}
	return keys
}

// legacyDesignAliases keep theme values stored by earlier versions working.
var legacyDesignAliases = map[string]string{
	"aurora": "plum-rail",
	"modern": "slate-classic",
	"paper":  "ivory-editorial",
	"mint":   "forest-centered",
	"dark":   "midnight-panel",
}

// LookupBuiltinDesign resolves a design key, a legacy theme name or a bare
// palette name, falling back to the library's first design.
func LookupBuiltinDesign(key string) BuiltinDesign {
	key = strings.ToLower(strings.TrimSpace(key))
	designs := BuiltinDesigns()
	if alias, ok := legacyDesignAliases[key]; ok {
		key = alias
	}
	for _, design := range designs {
		if design.Key == key {
			return design
		}
	}
	// A bare palette name selects that palette's first design.
	for _, design := range designs {
		if design.Palette.Key == key {
			return design
		}
	}
	return designs[0]
}

// BuiltinPaletteKeys lists the palette identities, sorted.
func BuiltinPaletteKeys() []string {
	keys := make([]string, 0, len(builtinPalettes))
	for _, palette := range builtinPalettes {
		keys = append(keys, palette.Key)
	}
	sort.Strings(keys)
	return keys
}

// BuiltinTemplate assembles a complete, professionally proportioned PowerPoint
// template for a design key. The result is a real .pptx package, so it flows
// through exactly the same analysis and rendering path as a customer upload.
func BuiltinTemplate(designKey string) ([]byte, error) {
	design := LookupBuiltinDesign(designKey)
	layouts := builtinLayouts(design)

	pkg := &Package{parts: map[string][]byte{}}
	pkg.SetText("_rels/.rels", relationshipsDocument(
		`<Relationship Id="rId1" Type="`+relationshipNamespace+`/officeDocument" Target="ppt/presentation.xml"/>`+
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>`+
			`<Relationship Id="rId3" Type="`+relationshipNamespace+`/extended-properties" Target="docProps/app.xml"/>`))
	pkg.SetText("docProps/core.xml", corePropertiesXML(Deck{Title: design.Name, Author: "Ptium"}))
	pkg.SetText("docProps/app.xml", xmlDeclaration+`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>Ptium</Application><PresentationFormat>16:9</PresentationFormat><Slides>0</Slides><Notes>0</Notes><HiddenSlides>0</HiddenSlides><AppVersion>16.0000</AppVersion></Properties>`)
	pkg.SetText("ppt/presProps.xml", xmlDeclaration+`<p:presentationPr `+presentationNamespaces+`/>`)
	pkg.SetText("ppt/viewProps.xml", xmlDeclaration+`<p:viewPr `+presentationNamespaces+`><p:normalViewPr/><p:slideViewPr><p:cSldViewPr snapToGrid="0"><p:cViewPr varScale="1"><p:scale><a:sx n="80" d="100"/><a:sy n="80" d="100"/></p:scale><p:origin x="0" y="0"/></p:cViewPr><p:guideLst/></p:cSldViewPr></p:slideViewPr><p:gridSpacing cx="76200" cy="76200"/></p:viewPr>`)
	pkg.SetText("ppt/tableStyles.xml", xmlDeclaration+`<a:tblStyleLst xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"/>`)
	pkg.SetText("ppt/theme/theme1.xml", builtinTheme(design))
	pkg.SetText("ppt/slideMasters/slideMaster1.xml", builtinMaster(design, len(layouts)))

	masterRels := `<Relationship Id="rId100" Type="` + relationshipNamespace + `/theme" Target="../theme/theme1.xml"/>`
	for index := range layouts {
		masterRels += fmt.Sprintf(`<Relationship Id="rId%d" Type="%s/slideLayout" Target="../slideLayouts/slideLayout%d.xml"/>`, index+1, relationshipNamespace, index+1)
	}
	pkg.SetText("ppt/slideMasters/_rels/slideMaster1.xml.rels", relationshipsDocument(masterRels))

	for index, layout := range layouts {
		part := fmt.Sprintf("ppt/slideLayouts/slideLayout%d.xml", index+1)
		pkg.SetText(part, layout.xml(design))
		pkg.SetText(RelationshipsPath(part), relationshipsDocument(
			`<Relationship Id="rId1" Type="`+relationshipNamespace+`/slideMaster" Target="../slideMasters/slideMaster1.xml"/>`))
	}

	pkg.SetText("ppt/presentation.xml", xmlDeclaration+`<p:presentation `+presentationNamespaces+` saveSubsetFonts="1">`+
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`+
		`<p:sldIdLst/>`+
		fmt.Sprintf(`<p:sldSz cx="%d" cy="%d"/><p:notesSz cx="6858000" cy="9144000"/>`, slideWidth, slideHeight)+
		`<p:defaultTextStyle><a:defPPr><a:defRPr lang="ko-KR"/></a:defPPr></p:defaultTextStyle></p:presentation>`)
	pkg.SetText("ppt/_rels/presentation.xml.rels", relationshipsDocument(
		`<Relationship Id="rId1" Type="`+relationshipNamespace+`/slideMaster" Target="slideMasters/slideMaster1.xml"/>`+
			`<Relationship Id="rId2" Type="`+relationshipNamespace+`/presProps" Target="presProps.xml"/>`+
			`<Relationship Id="rId3" Type="`+relationshipNamespace+`/viewProps" Target="viewProps.xml"/>`+
			`<Relationship Id="rId4" Type="`+relationshipNamespace+`/theme" Target="theme/theme1.xml"/>`+
			`<Relationship Id="rId5" Type="`+relationshipNamespace+`/tableStyles" Target="tableStyles.xml"/>`))

	pkg.SetText("[Content_Types].xml", builtinContentTypes(len(layouts)))
	return pkg.Bytes()
}

func builtinContentTypes(layoutCount int) string {
	var builder strings.Builder
	builder.WriteString(xmlDeclaration + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	builder.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>`)
	builder.WriteString(`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`)
	builder.WriteString(`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>`)
	for index := 1; index <= layoutCount; index++ {
		builder.WriteString(`<Override PartName="/ppt/slideLayouts/slideLayout` + strconv.Itoa(index) + `.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>`)
	}
	builder.WriteString(`<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`)
	builder.WriteString(`<Override PartName="/ppt/presProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presProps+xml"/>`)
	builder.WriteString(`<Override PartName="/ppt/viewProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.viewProps+xml"/>`)
	builder.WriteString(`<Override PartName="/ppt/tableStyles.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml"/>`)
	builder.WriteString(`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`)
	builder.WriteString(`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>`)
	builder.WriteString(`</Types>`)
	return builder.String()
}

func builtinTheme(design BuiltinDesign) string {
	palette := design.Palette
	name := "Ptium " + palette.Name
	accent := func(index int) string { return palette.Accents[index] }
	muted := mixColor(palette.Ink, palette.Surface, 0.45)
	surfaceRaised := mixColor(palette.Surface, palette.Ink, ifElse(palette.Dark, 0.10, 0.045))
	return xmlDeclaration + `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="` + escapeAttribute(name) + `"><a:themeElements>` +
		`<a:clrScheme name="` + escapeAttribute(name) + `">` +
		`<a:dk1><a:srgbClr val="` + palette.Ink + `"/></a:dk1><a:lt1><a:srgbClr val="` + palette.Surface + `"/></a:lt1>` +
		`<a:dk2><a:srgbClr val="` + muted + `"/></a:dk2><a:lt2><a:srgbClr val="` + surfaceRaised + `"/></a:lt2>` +
		`<a:accent1><a:srgbClr val="` + accent(0) + `"/></a:accent1><a:accent2><a:srgbClr val="` + accent(1) + `"/></a:accent2>` +
		`<a:accent3><a:srgbClr val="` + accent(2) + `"/></a:accent3><a:accent4><a:srgbClr val="` + accent(3) + `"/></a:accent4>` +
		`<a:accent5><a:srgbClr val="` + accent(4) + `"/></a:accent5><a:accent6><a:srgbClr val="` + accent(5) + `"/></a:accent6>` +
		`<a:hlink><a:srgbClr val="` + accent(1) + `"/></a:hlink><a:folHlink><a:srgbClr val="` + accent(2) + `"/></a:folHlink></a:clrScheme>` +
		`<a:fontScheme name="` + escapeAttribute(name) + `">` +
		`<a:majorFont><a:latin typeface="` + escapeAttribute(palette.MajorLatin) + `"/><a:ea typeface="` + escapeAttribute(palette.EastAsian) + `"/><a:cs typeface=""/></a:majorFont>` +
		`<a:minorFont><a:latin typeface="` + escapeAttribute(palette.MinorLatin) + `"/><a:ea typeface="` + escapeAttribute(palette.EastAsian) + `"/><a:cs typeface=""/></a:minorFont></a:fontScheme>` +
		`<a:fmtScheme name="` + escapeAttribute(name) + `">` +
		`<a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:tint val="80000"/></a:schemeClr></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:shade val="90000"/></a:schemeClr></a:solidFill></a:fillStyleLst>` +
		`<a:lnStyleLst><a:ln w="9525" cap="flat"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln><a:ln w="19050" cap="flat"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln><a:ln w="28575" cap="flat"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln></a:lnStyleLst>` +
		`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
		`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst>` +
		`</a:fmtScheme></a:themeElements><a:objectDefaults/><a:extraClrSchemeLst/></a:theme>`
}

// contentArea is the writable region of a content slide for a family.
func (f layoutFamily) contentArea() Frame {
	left := f.Margin
	if f.Rail > 0 {
		left = f.Rail + f.Margin
	}
	return Frame{X: left, Y: f.titleTop(), Width: slideWidth - left - f.Margin, Height: 0}
}

func (f layoutFamily) titleTop() int {
	switch {
	case f.Panel:
		return 548640
	case f.Eyebrow:
		return 731520
	}
	return 685800
}

func (f layoutFamily) titleHeight() int { return lineHeightFor(f.TitleSize) + 91440 }

// bodyTop is where a content slide's body begins, leaving room for whatever
// mark the family draws between title and body.
func (f layoutFamily) bodyTop() int {
	top := f.titleTop() + f.titleHeight() + 274320
	if f.Panel {
		top = 1737360
	}
	if f.Hairline || f.Rule {
		top += 137160
	}
	return top
}

func (f layoutFamily) bodyHeight() int { return slideHeight - f.bodyTop() - 914400 }

func builtinMaster(design BuiltinDesign, layoutCount int) string {
	palette, family := design.Palette, design.Family
	var layoutIDs strings.Builder
	for index := 1; index <= layoutCount; index++ {
		fmt.Fprintf(&layoutIDs, `<p:sldLayoutId id="%d" r:id="rId%d"/>`, 2147483649+index, index)
	}
	area := family.contentArea()
	muted := mixColor(palette.Ink, palette.Surface, 0.45)
	decoration := ""
	if family.Rail > 0 {
		decoration += shapeRect(9, "Rail", 0, 0, family.Rail, slideHeight, palette.Accents[0])
	}
	footer := placeholderShape(10, "Slide Number Placeholder 3", "sldNum", 12,
		slideWidth-family.Margin-1143000, slideHeight-685800, 1143000, 365760,
		`<a:bodyPr vert="horz" lIns="0" tIns="45720" rIns="0" bIns="45720" rtlCol="0" anchor="ctr"/><a:lstStyle><a:lvl1pPr algn="r"><a:defRPr sz="1100"><a:solidFill><a:srgbClr val="`+muted+`"/></a:solidFill></a:defRPr></a:lvl1pPr></a:lstStyle>`+
			`<a:p><a:fld id="{B7B5A0C4-2C2F-4E28-9E86-9E0F0F5B0E11}" type="slidenum"><a:rPr lang="ko-KR" smtClean="0"/><a:t>‹#›</a:t></a:fld><a:endParaRPr lang="ko-KR"/></a:p>`)

	return xmlDeclaration + `<p:sldMaster ` + presentationNamespaces + `><p:cSld>` +
		`<p:bg><p:bgPr><a:solidFill><a:schemeClr val="bg1"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>` +
		`<p:spTree>` + emptyGroupHeader +
		placeholderShape(2, "Title Placeholder 1", "title", -1, area.X, family.titleTop(), area.Width, family.titleHeight(),
			`<a:bodyPr vert="horz" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0" anchor="b"><a:normAutofit/></a:bodyPr><a:lstStyle/><a:p><a:r><a:rPr lang="ko-KR"/><a:t>마스터 제목 스타일 편집</a:t></a:r></a:p>`) +
		placeholderShape(3, "Text Placeholder 2", "body", 1, area.X, family.bodyTop(), area.Width, family.bodyHeight(),
			`<a:bodyPr vert="horz" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0"><a:normAutofit/></a:bodyPr><a:lstStyle/>`+
				`<a:p><a:r><a:rPr lang="ko-KR"/><a:t>마스터 텍스트 스타일 편집</a:t></a:r></a:p>`) +
		decoration + footer +
		`</p:spTree></p:cSld>` +
		`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
		`<p:sldLayoutIdLst>` + layoutIDs.String() + `</p:sldLayoutIdLst>` +
		`<p:txStyles>` +
		`<p:titleStyle><a:lvl1pPr algn="` + family.TitleAlign + `" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:lnSpc><a:spcPct val="95000"/></a:lnSpc><a:spcBef><a:spcPct val="0"/></a:spcBef><a:defRPr sz="` + strconv.Itoa(family.TitleSize) + `" b="1" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mj-lt"/><a:ea typeface="+mj-ea"/><a:cs typeface="+mj-cs"/></a:defRPr></a:lvl1pPr></p:titleStyle>` +
		`<p:bodyStyle>` +
		builtinBodyLevel(1, 0, 342900, family.BodySize, palette.Ink, "•", palette.Accents[0]) +
		builtinBodyLevel(2, 457200, 285750, family.BodySize-200, muted, "–", palette.Accents[1]) +
		builtinBodyLevel(3, 914400, 228600, family.BodySize-400, muted, "•", palette.Accents[2]) +
		builtinBodyLevel(4, 1371600, 228600, family.BodySize-500, muted, "–", palette.Accents[0]) +
		builtinBodyLevel(5, 1828800, 228600, family.BodySize-600, muted, "•", palette.Accents[1]) +
		`</p:bodyStyle>` +
		`<p:otherStyle><a:lvl1pPr marL="0" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1200" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:otherStyle>` +
		`</p:txStyles></p:sldMaster>`
}

func builtinBodyLevel(level, marginLeft, indent, size int, color, bullet, bulletColor string) string {
	if size < 1000 {
		size = 1000
	}
	return fmt.Sprintf(`<a:lvl%dpPr marL="%d" indent="-%d" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1">`+
		`<a:lnSpc><a:spcPct val="105000"/></a:lnSpc><a:spcBef><a:spcPts val="%d"/></a:spcBef>`+
		`<a:buClr><a:srgbClr val="%s"/></a:buClr><a:buSzPct val="90000"/><a:buFont typeface="Arial" panose="020B0604020202020204" pitchFamily="34" charset="0"/><a:buChar char="%s"/>`+
		`<a:defRPr sz="%d" kern="1200"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl%dpPr>`,
		level, marginLeft+indent, indent, 600-level*60, bulletColor, bullet, size, color, level)
}

// placeholderShape emits a placeholder shape. A negative index omits the idx
// attribute, which is what a title placeholder requires.
func placeholderShape(id int, name, phType string, index, x, y, width, height int, body string) string {
	reference := `<p:ph type="` + phType + `"`
	if index >= 0 {
		reference += ` idx="` + strconv.Itoa(index) + `"`
	}
	reference += `/>`
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr>%s</p:nvPr></p:nvSpPr>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`+
		`<p:txBody>%s</p:txBody></p:sp>`, id, escapeAttribute(name), reference, x, y, width, height, body)
}

func shapeRect(id int, name string, x, y, width, height int, fill string) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`+
		`<a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr>`+
		`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p></p:txBody></p:sp>`, id, escapeAttribute(name), x, y, width, height, fill)
}

type builtinLayout struct {
	Name       string
	Type       string
	ShowMaster bool
	Shapes     func(design BuiltinDesign) string
}

func (l builtinLayout) xml(design BuiltinDesign) string {
	showMaster := ""
	if !l.ShowMaster {
		showMaster = ` showMasterSp="0"`
	}
	return xmlDeclaration + `<p:sldLayout ` + presentationNamespaces + ` type="` + l.Type + `" preserve="1"` + showMaster + `>` +
		`<p:cSld name="` + escapeAttribute(l.Name) + `"><p:spTree>` + emptyGroupHeader + l.Shapes(design) +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`
}

func textBody(align, anchor string, size int, bold bool, color string, prompt string) string {
	weight := ""
	if bold {
		weight = ` b="1"`
	}
	anchorAttribute := ""
	if anchor != "" {
		anchorAttribute = ` anchor="` + anchor + `"`
	}
	return `<a:bodyPr vert="horz" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0"` + anchorAttribute + `><a:normAutofit/></a:bodyPr>` +
		`<a:lstStyle><a:lvl1pPr algn="` + align + `" marL="0" indent="0"><a:buNone/>` +
		`<a:defRPr sz="` + strconv.Itoa(size) + `"` + weight + `><a:solidFill><a:srgbClr val="` + color + `"/></a:solidFill></a:defRPr></a:lvl1pPr></a:lstStyle>` +
		`<a:p><a:r><a:rPr lang="ko-KR"/><a:t>` + escapeText(prompt) + `</a:t></a:r></a:p>`
}

func bulletBody(size int, color string) string {
	return `<a:bodyPr vert="horz" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0"><a:normAutofit/></a:bodyPr>` +
		`<a:lstStyle><a:lvl1pPr><a:defRPr sz="` + strconv.Itoa(size) + `"><a:solidFill><a:srgbClr val="` + color + `"/></a:solidFill></a:defRPr></a:lvl1pPr></a:lstStyle>` +
		`<a:p><a:r><a:rPr lang="ko-KR"/><a:t>본문 텍스트를 입력하세요</a:t></a:r></a:p>`
}

// titleFurniture draws whatever a family puts between the title and the body:
// an accent rule, a hairline, a filled panel or nothing at all.
func (f layoutFamily) titleFurniture(id int, palette BuiltinPalette, area Frame) string {
	switch {
	case f.Panel:
		return shapeRect(id, "Title Panel", 0, 0, slideWidth, 1508760, palette.Accents[0])
	case f.Rule:
		return shapeRect(id, "Accent Rule", area.X, f.titleTop()-182880, 914400, 45720, palette.Accents[0])
	case f.Hairline:
		return shapeRect(id, "Hairline", area.X, f.titleTop()+f.titleHeight()+137160, area.Width, 9525,
			mixColor(palette.Surface, palette.Ink, ifElse(palette.Dark, 0.24, 0.14)))
	}
	return ""
}

// panelInk is the text colour a title panel needs to stay readable.
func panelInk(palette BuiltinPalette) string {
	return readableInk(palette.Accents[0], palette.Surface, palette.Ink)
}

func builtinLayouts(design BuiltinDesign) []builtinLayout {
	family := design.Family
	area := family.contentArea()
	titleTop, titleHeight := family.titleTop(), family.titleHeight()
	bodyTop, bodyHeight := family.bodyTop(), family.bodyHeight()
	halfWidth := (area.Width - 457200) / 2
	rightX := area.X + halfWidth + 457200

	// A content slide's title and body, shared by most layouts.
	contentTitle := func(design BuiltinDesign) string {
		palette := design.Palette
		color := palette.Ink
		top, height := titleTop, titleHeight
		if family.Panel {
			color, top, height = panelInk(palette), 640080, lineHeightFor(family.TitleSize)+91440
		}
		furniture := family.titleFurniture(8, palette, area)
		eyebrow := ""
		if family.Eyebrow {
			eyebrow = placeholderShape(7, "Text Placeholder 6", "body", 9, area.X, titleTop-320040, area.Width, 274320,
				textBody(family.TitleAlign, "b", 1100, true, palette.Accents[0], "구역 이름"))
		}
		return furniture + eyebrow +
			placeholderShape(2, "Title 1", "title", -1, area.X, top, area.Width, height,
				textBody(family.TitleAlign, "b", family.TitleSize, true, color, "제목을 입력하세요"))
	}

	return []builtinLayout{
		{
			Name: "제목 슬라이드", Type: "title", ShowMaster: family.Rail > 0,
			Shapes: func(d BuiltinDesign) string {
				palette := d.Palette
				coverTop := 2560320
				accentWidth := 1600200
				accentX := area.X
				if family.CoverAlign == "ctr" {
					accentX = (slideWidth - accentWidth) / 2
				}
				return shapeRect(8, "Cover Accent", accentX, coverTop-274320, accentWidth, 68580, palette.Accents[0]) +
					placeholderShape(2, "Title 1", "ctrTitle", -1, area.X, coverTop, area.Width, 1600200,
						textBody(family.CoverAlign, "b", family.CoverSize, true, palette.Ink, "프레젠테이션 제목")) +
					placeholderShape(3, "Subtitle 2", "subTitle", 1, area.X, coverTop+1737360, area.Width, 914400,
						textBody(family.CoverAlign, "t", 2000, false, mixColor(palette.Ink, palette.Surface, 0.42), "부제목 또는 한 줄 요약"))
			},
		},
		{
			Name: "구역 머리글", Type: "secHead", ShowMaster: family.Rail > 0,
			Shapes: func(d BuiltinDesign) string {
				palette := d.Palette
				return shapeRect(8, "Section Accent", area.X, 2377440, 685800, 68580, palette.Accents[0]) +
					placeholderShape(2, "Title 1", "title", -1, area.X, 2651760, area.Width, 1097280,
						textBody(family.CoverAlign, "b", family.TitleSize+400, true, palette.Ink, "구역 제목")) +
					placeholderShape(3, "Text Placeholder 2", "body", 1, area.X, 3840480, area.Width, 731520,
						textBody(family.CoverAlign, "t", 1800, false, mixColor(palette.Ink, palette.Surface, 0.42),
							"이 구역에서 다룰 내용을 한 문장으로 소개합니다"))
			},
		},
		{
			Name: "제목 및 내용", Type: "obj", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string {
				return contentTitle(d) +
					placeholderShape(3, "Content Placeholder 2", "body", 1, area.X, bodyTop, area.Width, bodyHeight,
						bulletBody(family.BodySize, d.Palette.Ink))
			},
		},
		{
			Name: "콘텐츠 2개", Type: "twoObj", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string {
				return contentTitle(d) +
					placeholderShape(3, "Content Placeholder 2", "body", 1, area.X, bodyTop, halfWidth, bodyHeight,
						bulletBody(family.BodySize-100, d.Palette.Ink)) +
					placeholderShape(4, "Content Placeholder 3", "body", 2, rightX, bodyTop, halfWidth, bodyHeight,
						bulletBody(family.BodySize-100, d.Palette.Ink))
			},
		},
		{
			Name: "비교", Type: "twoTxTwoObj", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string {
				palette := d.Palette
				const headerHeight = 502920
				subBodyTop := bodyTop + headerHeight + 137160
				subBodyHeight := bodyHeight - headerHeight - 137160
				return contentTitle(d) +
					placeholderShape(3, "Text Placeholder 2", "body", 1, area.X, bodyTop, halfWidth, headerHeight,
						textBody("l", "ctr", 2000, true, palette.Accents[0], "왼쪽 항목")) +
					placeholderShape(4, "Content Placeholder 3", "body", 2, area.X, subBodyTop, halfWidth, subBodyHeight,
						bulletBody(family.BodySize-200, palette.Ink)) +
					placeholderShape(5, "Text Placeholder 4", "body", 3, rightX, bodyTop, halfWidth, headerHeight,
						textBody("l", "ctr", 2000, true, palette.Accents[1], "오른쪽 항목")) +
					placeholderShape(6, "Content Placeholder 5", "body", 4, rightX, subBodyTop, halfWidth, subBodyHeight,
						bulletBody(family.BodySize-200, palette.Ink))
			},
		},
		{
			Name: "핵심 인용", Type: "obj", ShowMaster: family.Rail > 0,
			Shapes: func(d BuiltinDesign) string {
				palette := d.Palette
				return shapeRect(8, "Quote Accent", area.X, 1828800, 274320, 274320, palette.Accents[0]) +
					placeholderShape(2, "Title 1", "title", -1, area.X, 2377440, area.Width, 1828800,
						textBody("l", "ctr", family.TitleSize-200, false, palette.Ink, "기억에 남길 한 문장을 입력하세요")) +
					placeholderShape(3, "Text Placeholder 2", "body", 1, area.X, 4389120, area.Width, 457200,
						textBody("l", "t", 1600, false, mixColor(palette.Ink, palette.Surface, 0.45), "출처 또는 화자"))
			},
		},
		{
			Name: "캡션 있는 그림", Type: "picTx", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string {
				pictureWidth := area.Width*3/5 - 228600
				captionX := area.X + pictureWidth + 457200
				captionWidth := area.Width - pictureWidth - 457200
				return contentTitle(d) +
					placeholderShape(3, "Picture Placeholder 2", "pic", 1, area.X, bodyTop, pictureWidth, bodyHeight,
						`<a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p>`) +
					placeholderShape(4, "Text Placeholder 3", "body", 2, captionX, bodyTop, captionWidth, bodyHeight,
						bulletBody(family.BodySize-200, d.Palette.Ink))
			},
		},
		{
			Name: "제목만", Type: "titleOnly", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string { return contentTitle(d) },
		},
		{
			Name: "마무리", Type: "secHead", ShowMaster: family.Rail > 0,
			Shapes: func(d BuiltinDesign) string {
				palette := d.Palette
				return shapeRect(8, "Closing Accent", area.X, 2377440, 1600200, 68580, palette.Accents[0]) +
					placeholderShape(2, "Title 1", "ctrTitle", -1, area.X, 2651760, area.Width, 1097280,
						textBody(family.CoverAlign, "b", family.TitleSize+400, true, palette.Ink, "감사합니다")) +
					placeholderShape(3, "Subtitle 2", "subTitle", 1, area.X, 3840480, area.Width, 1097280,
						textBody(family.CoverAlign, "t", 1800, false, mixColor(palette.Ink, palette.Surface, 0.42),
							"다음 단계와 연락처를 남기세요"))
			},
		},
		{
			Name: "빈 화면", Type: "blank", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string { return "" },
		},
	}
}
