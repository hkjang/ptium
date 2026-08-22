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
	// Use is the kind of meeting the palette was chosen for, in two or three
	// words. It is what someone scanning a library actually filters on.
	Use string
}

// builtinPalettes is the validated palette library.
var builtinPalettes = []BuiltinPalette{
	{
		Key: "slate", Name: "Slate", Dark: false,
		Surface: "FFFFFF", Ink: "15181D",
		Accents:    [6]string{"2563EB", "EB6834", "1BAF7A", "EDA100", "E87BA4", "008300"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "중립적인 밝은 배경에 선명한 블루. 사내 업무 보고에 가장 안전한 선택입니다.",
		Use:  "사내 보고",
	},
	{
		Key: "azure", Name: "Azure", Dark: false,
		Surface: "F5F9FF", Ink: "10233D",
		Accents:    [6]string{"0B6BCB", "EB6834", "1BAF7A", "EDA100", "E87BA4", "008300"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "차분한 블루 틴트 배경. 금융·공공 부문 보고에 어울립니다.",
		Use:  "금융·공공",
	},
	{
		Key: "crimson", Name: "Crimson", Dark: false,
		Surface: "FFFBFB", Ink: "1B1112",
		Accents:    [6]string{"C81E33", "2A78D6", "EB6834", "1BAF7A", "EDA100", "E87BA4"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "밝은 배경에 크림슨 액센트. 리스크와 의사결정 자료에 씁니다.",
		Use:  "리스크·의사결정",
	},
	{
		Key: "coral", Name: "Coral", Dark: false,
		Surface: "FFFBF8", Ink: "241A16",
		Accents:    [6]string{"DE5B3B", "2A78D6", "1BAF7A", "EDA100", "E87BA4", "008300"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "부드러운 웜 배경에 코랄 액센트. 브랜드·마케팅 발표용입니다.",
		Use:  "브랜드·마케팅",
	},
	{
		Key: "ivory", Name: "Ivory", Dark: false,
		Surface: "FBF8F1", Ink: "1E1B16",
		Accents:    [6]string{"A8551E", "2A78D6", "EB6834", "1BAF7A", "EDA100", "E87BA4"},
		MajorLatin: "Georgia", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "따뜻한 아이보리와 세리프 제목. 제안서와 연차 보고서 톤입니다.",
		Use:  "제안서",
	},
	{
		Key: "sand", Name: "Sand", Dark: false,
		Surface: "F7F3EA", Ink: "241F17",
		Accents:    [6]string{"B26A12", "2A78D6", "EB6834", "1BAF7A", "EDA100", "E87BA4"},
		MajorLatin: "Georgia", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "모래빛 배경과 세리프 제목. 리서치와 정책 문서에 맞습니다.",
		Use:  "리서치·정책",
	},
	{
		Key: "midnight", Name: "Midnight", Dark: true,
		Surface: "0E1526", Ink: "F2F5FB",
		Accents:    [6]string{"5B8DEF", "D95926", "199E70", "6B7B93", "8B98AC", "AEB8C6"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "깊은 네이비 다크 테마. 대형 화면 발표와 임원 브리핑에 강한 인상을 줍니다.",
		Use:  "임원 브리핑",
	},
	{
		Key: "graphite", Name: "Graphite", Dark: true,
		Surface: "1B1D21", Ink: "F3F4F6",
		Accents:    [6]string{"C98500", "3987E5", "D95926", "199E70", "9085E9", "E66767"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "중립 그레이 다크 테마에 앰버 액센트. 기술 리뷰와 아키텍처 설명용입니다.",
		Use:  "기술 리뷰",
	},
	{
		Key: "forest", Name: "Forest", Dark: true,
		Surface: "0D1F19", Ink: "EAF3EE",
		Accents:    [6]string{"17A574", "3987E5", "D95926", "9085E9", "E66767", "B05FC4"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "딥 그린 다크 테마. 지속가능성, ESG, 인프라 주제에 씁니다.",
		Use:  "ESG·인프라",
	},
	{
		Key: "plum", Name: "Plum", Dark: true,
		Surface: "1A1024", Ink: "F6F1FC",
		Accents:    [6]string{"8B5CF6", "D95926", "199E70", "9085E9", "E66767", "B05FC4"},
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
		Note: "딥 퍼플 다크 테마. 제품 출시와 비전 발표에 어울립니다.",
		Use:  "제품 출시",
	},
}

// layoutFamily is the composition half of a design: where the title sits, how
// wide the margins are, and what marks the slide as belonging to this family.
type layoutFamily struct {
	Key  string
	Name string
	Note string
	// Label is the one or two words a person browsing the library reads.
	Label string

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
	// Cover is how the opening slide is composed. Recolouring one composition ten
	// times produces ten templates that look like the same template, so the
	// families differ in where the title sits before they differ in hue.
	Cover string
	// Body is how a content slide divides its page.
	Body string
	// Motif is the figure a metaphor family draws: a circle, an arc, a diagonal.
	Motif string
	// Serif sets the display face in a serif whatever the palette prefers.
	Serif bool
	// Footer draws a quiet rule along the bottom margin.
	Footer bool
}

// Cover compositions.
const (
	coverStack  = ""       // an accent bar over the title, the library's default
	coverPanel  = "panel"  // a full-height accent panel carrying the title
	coverBand   = "band"   // an accent band filling the lower half
	coverPlate  = "plate"  // a centred plate the title sits inside
	coverCorner = "corner" // a large accent block in the upper corner
	coverColumn = "column" // the title and the subtitle in facing columns
	coverMotif  = "motif"  // a drawn metaphor: a circle, an arc, a diagonal
)

// Motifs. A metaphor family draws one figure and repeats a quiet echo of it on
// every content slide, which is what makes a deck look designed rather than
// decorated: the same idea, said once loudly and then softly.
const (
	motifOrbit    = "orbit"    // a circle rising off the right edge
	motifArc      = "arc"      // a wide arc across the foot of the slide
	motifDiagonal = "diagonal" // a diagonal field cutting the page
	motifDots     = "dots"     // a scatter of dots, densest at one corner
	motifLayers   = "layers"   // translucent panels overlapping
	motifWash     = "wash"     // a gradient the title is reversed out of
)

// Body compositions.
const (
	bodyWide   = ""        // one column across the page
	bodyIndent = "indent"  // the title in a narrow left column, the body beside it
	bodySide   = "sidebar" // a tinted column carries the title, the body sits right
)

const (
	slideWidth  = 12192000
	slideHeight = 6858000
)

var layoutFamilies = []layoutFamily{
	{
		Key: "classic", Name: "Classic", Label: "기본", Note: "제목 위 액센트 룰, 좌측 정렬, 넉넉한 본문",
		Margin: 838200, TitleSize: 4000, BodySize: 1800, CoverSize: 5400,
		TitleAlign: "l", CoverAlign: "l", Rule: true,
	},
	{
		Key: "rail", Name: "Rail", Label: "레일", Note: "좌측 액센트 레일이 모든 슬라이드를 묶어 주는 구성",
		Margin: 1097280, TitleSize: 3800, BodySize: 1800, CoverSize: 5000,
		TitleAlign: "l", CoverAlign: "l", Rail: 182880,
	},
	{
		Key: "centered", Name: "Centered", Label: "중앙", Note: "표지와 구역을 중앙 정렬한 키노트형 구성",
		Margin: 1143000, TitleSize: 3800, BodySize: 1800, CoverSize: 5000,
		TitleAlign: "l", CoverAlign: "ctr",
	},
	{
		Key: "panel", Name: "Panel", Label: "패널", Note: "제목이 상단 색 패널에 놓이는 강한 위계",
		Margin: 838200, TitleSize: 3400, BodySize: 1800, CoverSize: 5000,
		TitleAlign: "l", CoverAlign: "l", Panel: true,
	},
	{
		Key: "editorial", Name: "Editorial", Label: "편집", Note: "얇은 헤어라인과 아이브로우를 쓰는 편집 디자인",
		Margin: 1188720, TitleSize: 3400, BodySize: 1700, CoverSize: 4800,
		TitleAlign: "l", CoverAlign: "l", Hairline: true, Eyebrow: true,
	},
	{
		Key: "minimal", Name: "Minimal", Label: "미니멀", Note: "장식을 걷어내고 여백과 타이포만 남긴 구성",
		Margin: 1371600, TitleSize: 3200, BodySize: 1700, CoverSize: 4400,
		TitleAlign: "l", CoverAlign: "l",
	},
	{
		Key: "column", Name: "Column", Label: "2단", Note: "제목은 왼쪽 단, 본문은 오른쪽 단에 두는 보고서 구성",
		Margin: 914400, TitleSize: 2800, BodySize: 1700, CoverSize: 4000,
		TitleAlign: "l", CoverAlign: "l", Cover: coverColumn, Body: bodyIndent, Serif: true, Footer: true,
	},
	{
		Key: "sidebar", Name: "Sidebar", Label: "사이드바", Note: "제목이 왼쪽 색 기둥에 서고 본문이 그 옆에 오는 구성",
		Margin: 685800, TitleSize: 2800, BodySize: 1700, CoverSize: 4000,
		TitleAlign: "l", CoverAlign: "l", Cover: coverPanel, Body: bodySide,
	},
	{
		Key: "split", Name: "Split", Label: "분할", Note: "표지를 세로로 가르는 액센트 패널, 본문은 헤어라인",
		Margin: 838200, TitleSize: 3600, BodySize: 1800, CoverSize: 4000,
		TitleAlign: "l", CoverAlign: "l", Cover: coverPanel, Hairline: true,
	},
	{
		Key: "band", Name: "Band", Label: "밴드", Note: "표지 아래를 가득 채우는 액센트 띠와 아이브로우",
		Margin: 914400, TitleSize: 3600, BodySize: 1800, CoverSize: 4400,
		TitleAlign: "l", CoverAlign: "l", Cover: coverBand, Eyebrow: true,
	},
	{
		Key: "plate", Name: "Plate", Label: "플레이트", Note: "표지 제목을 가운데 판 위에 얹은 초대장 같은 구성",
		Margin: 1143000, TitleSize: 3200, BodySize: 1800, CoverSize: 4000,
		TitleAlign: "ctr", CoverAlign: "ctr", Cover: coverPlate, Serif: true,
	},
	{
		Key: "orbit", Name: "Orbit", Label: "원", Note: "오른쪽에서 떠오르는 큰 원. 성장과 확장을 말하는 덱에",
		Margin: 914400, TitleSize: 3400, BodySize: 1800, CoverSize: 4400,
		TitleAlign: "l", CoverAlign: "l", Cover: coverMotif, Motif: motifOrbit, Rule: true,
	},
	{
		Key: "arc", Name: "Arc", Label: "아치", Note: "슬라이드 아래를 가로지르는 아치. 여정과 단계를 말하는 덱에",
		Margin: 914400, TitleSize: 3400, BodySize: 1800, CoverSize: 4200,
		TitleAlign: "l", CoverAlign: "l", Cover: coverMotif, Motif: motifArc, Hairline: true,
	},
	{
		Key: "diagonal", Name: "Diagonal", Label: "사선", Note: "화면을 가르는 사선. 전환과 대비를 말하는 덱에",
		Margin: 914400, TitleSize: 3400, BodySize: 1800, CoverSize: 4200,
		TitleAlign: "l", CoverAlign: "l", Cover: coverMotif, Motif: motifDiagonal,
	},
	{
		Key: "dots", Name: "Dots", Label: "도트", Note: "모서리에 모인 점들. 데이터와 규모를 말하는 덱에",
		Margin: 914400, TitleSize: 3400, BodySize: 1800, CoverSize: 4200,
		TitleAlign: "l", CoverAlign: "l", Cover: coverMotif, Motif: motifDots, Eyebrow: true,
	},
	{
		Key: "layers", Name: "Layers", Label: "레이어", Note: "겹쳐진 반투명 판. 구조와 계층을 말하는 덱에",
		Margin: 1005840, TitleSize: 3200, BodySize: 1700, CoverSize: 4000,
		TitleAlign: "l", CoverAlign: "l", Cover: coverMotif, Motif: motifLayers,
	},
	{
		Key: "wash", Name: "Wash", Label: "그라데이션", Note: "표지를 가득 채운 그라데이션. 비전과 출시를 말하는 덱에",
		Margin: 914400, TitleSize: 3400, BodySize: 1800, CoverSize: 4600,
		TitleAlign: "l", CoverAlign: "l", Cover: coverMotif, Motif: motifWash, Rule: true,
	},
	{
		Key: "corner", Name: "Corner", Label: "코너", Note: "위쪽 모서리 색 블록과 아래에 앉은 제목",
		Margin: 838200, TitleSize: 3600, BodySize: 1800, CoverSize: 4600,
		TitleAlign: "l", CoverAlign: "l", Cover: coverCorner, Rule: true, Footer: true,
	},
}

// BuiltinDesign is one shipped template: a palette in a layout family.
type BuiltinDesign struct {
	Key     string
	Name    string
	Palette BuiltinPalette
	Family  layoutFamily
}

// MajorLatin is the display face this design sets titles in. A family that asks
// for a serif gets one whatever the palette prefers, because a serif title and a
// grotesque title are two different designs even in the same colours.
func (d BuiltinDesign) MajorLatin() string {
	if d.Family.Serif {
		return "Georgia"
	}
	return d.Palette.MajorLatin
}

// Tags describe a design in the terms someone choosing one thinks in: how dark
// it is, how it is composed, what it was drawn for. A library is browsable when
// it can be narrowed, and these are what it narrows by.
func (d BuiltinDesign) Tags() []string {
	tags := make([]string, 0, 4)
	if d.Palette.Dark {
		tags = append(tags, "어두운")
	} else {
		tags = append(tags, "밝은")
	}
	if label := strings.TrimSpace(d.Family.Label); label != "" {
		tags = append(tags, label)
	}
	if d.MajorLatin() == "Georgia" {
		tags = append(tags, "세리프")
	}
	if use := strings.TrimSpace(d.Palette.Use); use != "" {
		tags = append(tags, use)
	}
	return tags
}

// Description is the sentence shown in the template library.
func (d BuiltinDesign) Description() string {
	return fmt.Sprintf("%s · %s. %s", d.Family.Name, d.Family.Note, d.Palette.Note)
}

// builtinPairs assigns five layout families to every palette. Rows are appended
// to, never rewritten: a deck built on any pairing keeps its design. The first
// three are the original compositions, the fourth is a structurally different
// one, and the fifth draws a metaphor — a circle, an arc, a diagonal — because a
// deck about growth and a deck about risk should not open with the same picture.
var builtinPairs = map[string][5]string{
	"slate":    {"classic", "panel", "minimal", "column", "orbit"},
	"azure":    {"classic", "rail", "centered", "sidebar", "arc"},
	"crimson":  {"classic", "editorial", "panel", "corner", "diagonal"},
	"coral":    {"rail", "centered", "editorial", "band", "dots"},
	"ivory":    {"editorial", "centered", "classic", "plate", "layers"},
	"sand":     {"editorial", "minimal", "panel", "column", "arc"},
	"midnight": {"panel", "rail", "minimal", "band", "wash"},
	"graphite": {"minimal", "classic", "rail", "sidebar", "diagonal"},
	"forest":   {"panel", "centered", "minimal", "split", "orbit"},
	"plum":     {"rail", "centered", "editorial", "corner", "wash"},
}

// BuiltinDesigns returns the shipped library in a stable order.
func BuiltinDesigns() []BuiltinDesign {
	families := map[string]layoutFamily{}
	for _, family := range layoutFamilies {
		families[family.Key] = family
	}
	result := make([]BuiltinDesign, 0, len(builtinPalettes)*5)
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
		`<a:majorFont><a:latin typeface="` + escapeAttribute(design.MajorLatin()) + `"/><a:ea typeface="` + escapeAttribute(palette.EastAsian) + `"/><a:cs typeface=""/></a:majorFont>` +
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

// sidebarWidth is the tinted column a sidebar family stands its titles in.
func (f layoutFamily) sidebarWidth() int { return slideWidth * 30 / 100 }

// titleFrame and bodyFrame divide a content slide. Most families run both across
// the page; a column family sets the title in a narrow measure with the body
// beside it, and a sidebar family stands the title in a tinted column.
func (f layoutFamily) titleFrame() Frame {
	area := f.contentArea()
	switch f.Body {
	case bodyIndent:
		return Frame{X: area.X, Y: f.titleTop(), Width: area.Width * 34 / 100, Height: f.titleHeight() * 3}
	case bodySide:
		return Frame{X: f.Margin, Y: f.titleTop(), Width: f.sidebarWidth() - f.Margin*2, Height: f.titleHeight() * 3}
	}
	return Frame{X: area.X, Y: f.titleTop(), Width: area.Width, Height: f.titleHeight()}
}

func (f layoutFamily) bodyFrame() Frame {
	area := f.contentArea()
	switch f.Body {
	case bodyIndent:
		column := area.Width * 34 / 100
		gap := 457200
		return Frame{X: area.X + column + gap, Y: f.titleTop(), Width: area.Width - column - gap,
			Height: slideHeight - f.titleTop() - 914400}
	case bodySide:
		left := f.sidebarWidth() + f.Margin
		return Frame{X: left, Y: f.titleTop(), Width: slideWidth - left - f.Margin,
			Height: slideHeight - f.titleTop() - 914400}
	}
	return Frame{X: area.X, Y: f.bodyTop(), Width: area.Width, Height: f.bodyHeight()}
}

// columnFill is the tint a sidebar column is painted in: a neutral lift off the
// page, never a wash of the brand hue. An amber accent at sixteen percent over a
// dark grey is mud, and it covers a third of the slide — the colour belongs in
// the stripe at the column's edge, where a little of it goes a long way.
func (f layoutFamily) columnFill(palette BuiltinPalette) string {
	// The blend is in linear light, where a little goes much further than an sRGB
	// average would suggest: twelve percent of near-white over near-black is a
	// mid grey, not a lift.
	return mixColor(palette.Surface, palette.Ink, ifElse(palette.Dark, 0.03, 0.06))
}

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

// slideNumberPlaceholder is the page number. It lives on the master, and again
// on every layout that hides the master's shapes — otherwise whether a page is
// numbered depends on whether its design happens to have a rail, and one deck
// numbers its section pages while the same deck in another design does not.
// Which pages show a number is a decision about the deck; a rail is decoration.
func slideNumberPlaceholder(id int, family layoutFamily, palette BuiltinPalette) string {
	muted := mixColor(palette.Ink, palette.Surface, 0.45)
	return placeholderShape(id, "Slide Number Placeholder 3", "sldNum", 12,
		slideWidth-family.Margin-1143000, slideHeight-685800, 1143000, 365760,
		`<a:bodyPr vert="horz" lIns="0" tIns="45720" rIns="0" bIns="45720" rtlCol="0" anchor="ctr"/><a:lstStyle><a:lvl1pPr algn="r"><a:defRPr sz="1100"><a:solidFill><a:srgbClr val="`+muted+`"/></a:solidFill></a:defRPr></a:lvl1pPr></a:lstStyle>`+
			`<a:p><a:fld id="{B7B5A0C4-2C2F-4E28-9E86-9E0F0F5B0E11}" type="slidenum"><a:rPr lang="ko-KR" smtClean="0"/><a:t>‹#›</a:t></a:fld><a:endParaRPr lang="ko-KR"/></a:p>`)
}

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
	footer := slideNumberPlaceholder(10, family, palette)

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

// shapeGeom draws one decorative shape: any preset geometry, an optional
// transparency, an optional rotation. The metaphor families are built out of
// these — a circle, an arc, a diagonal, a scatter of dots.
func shapeGeom(id int, name, preset string, x, y, width, height int, fill string, alpha, rotation int) string {
	transparency := ""
	if alpha > 0 && alpha < 100 {
		transparency = fmt.Sprintf(`<a:alpha val="%d"/>`, alpha*1000)
	}
	spin := ""
	if rotation != 0 {
		spin = fmt.Sprintf(` rot="%d"`, rotation*60000)
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr><a:xfrm%s><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="%s"><a:avLst/></a:prstGeom>`+
		`<a:solidFill><a:srgbClr val="%s">%s</a:srgbClr></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr>`+
		`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p></p:txBody></p:sp>`,
		id, escapeAttribute(name), spin, x, y, max(width, 1), max(height, 1), escapeAttribute(preset),
		escapeAttribute(fill), transparency)
}

// shapeOutline draws a preset shape as a stroked outline rather than a fill,
// which is how an arc or a ring reads as a mark instead of a block.
func shapeOutline(id int, name, preset string, x, y, width, height int, stroke string, weight, alpha int) string {
	transparency := ""
	if alpha > 0 && alpha < 100 {
		transparency = fmt.Sprintf(`<a:alpha val="%d"/>`, alpha*1000)
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="%s"><a:avLst/></a:prstGeom><a:noFill/>`+
		`<a:ln w="%d"><a:solidFill><a:srgbClr val="%s">%s</a:srgbClr></a:solidFill></a:ln></p:spPr>`+
		`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p></p:txBody></p:sp>`,
		id, escapeAttribute(name), x, y, max(width, 1), max(height, 1), escapeAttribute(preset),
		weight, escapeAttribute(stroke), transparency)
}

// shapeWash fills a frame with a gradient between two colours.
func shapeWash(id int, name string, x, y, width, height int, from, to string, angle int) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`+
		`<a:gradFill rotWithShape="1"><a:gsLst>`+
		`<a:gs pos="0"><a:srgbClr val="%s"/></a:gs><a:gs pos="100000"><a:srgbClr val="%s"/></a:gs>`+
		`</a:gsLst><a:lin ang="%d" scaled="0"/></a:gradFill><a:ln><a:noFill/></a:ln></p:spPr>`+
		`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p></p:txBody></p:sp>`,
		id, escapeAttribute(name), x, y, max(width, 1), max(height, 1),
		escapeAttribute(from), escapeAttribute(to), angle*60000)
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

// ifInt is the integer companion of ifElse, for geometry that is present in one
// composition and absent in another.
func ifInt(condition bool, whenTrue, whenFalse int) int {
	if condition {
		return whenTrue
	}
	return whenFalse
}

// panelInk is the text colour a title panel needs to stay readable.
func panelInk(palette BuiltinPalette) string {
	return readableInk(palette.Accents[0], palette.Surface, palette.Ink)
}

// motifCover draws a metaphor family's opening slide: the figure, then the
// title standing clear of it. Every figure is built from preset geometry, so the
// browser preview and PowerPoint draw the same picture.
func (f layoutFamily) motifCover(palette BuiltinPalette, area Frame) string {
	accent := palette.Accents[0]
	subtitleInk := mixColor(palette.Ink, palette.Surface, 0.42)
	ink := palette.Ink
	art := ""
	titleTop, titleWidth := slideHeight*44/100, area.Width
	switch f.Motif {
	case motifOrbit:
		// A circle rising off the right edge, with a smaller one in orbit.
		size := slideHeight * 118 / 100
		art = shapeGeom(8, "Orbit", "ellipse", slideWidth-size*62/100, slideHeight-size*72/100, size, size, accent, 16, 0) +
			shapeGeom(9, "Orbit Core", "ellipse", slideWidth-slideWidth*17/100, slideHeight*17/100,
				slideWidth*9/100, slideWidth*9/100, accent, 100, 0) +
			shapeOutline(10, "Orbit Ring", "ellipse", slideWidth-slideWidth*30/100, slideHeight*6/100,
				slideWidth*22/100, slideWidth*22/100, accent, 12700, 45)
		titleWidth = area.Width * 62 / 100
	case motifArc:
		// A wide arc across the foot of the slide, and its echo above.
		width, height := slideWidth*150/100, slideHeight*120/100
		art = shapeGeom(8, "Arc", "ellipse", (slideWidth-width)/2, slideHeight-height*32/100, width, height, accent, 14, 0) +
			shapeOutline(9, "Arc Line", "ellipse", (slideWidth-width)/2, slideHeight-height*26/100, width, height, accent, 19050, 55)
		titleTop = slideHeight * 30 / 100
	case motifDiagonal:
		// A diagonal field cutting the page, with a thin second cut beside it.
		art = shapeGeom(8, "Diagonal", "rtTriangle", 0, 0, slideWidth*72/100, slideHeight, accent, 12, 0) +
			shapeGeom(9, "Diagonal Edge", "rtTriangle", 0, 0, slideWidth*30/100, slideHeight*42/100, accent, 100, 0)
		titleTop = slideHeight * 52 / 100
		titleWidth = area.Width * 78 / 100
	case motifDots:
		// A scatter of dots, densest at the corner it starts from.
		dot := slideWidth * 13 / 1000
		gap := dot * 22 / 10
		for row := 0; row < 5; row++ {
			for column := 0; column < 7; column++ {
				fade := 100 - (row+column)*11
				if fade < 12 {
					continue
				}
				art += shapeGeom(20+row*10+column, fmt.Sprintf("Dot %d-%d", row, column), "ellipse",
					slideWidth-area.X-gap*(6-column), slideHeight*10/100+gap*row, dot, dot, accent, fade, 0)
			}
		}
		art += shapeGeom(9, "Dot Mark", "ellipse", area.X, slideHeight*36/100, dot*3, dot*3, accent, 100, 0)
		titleTop = slideHeight * 46 / 100
		titleWidth = area.Width * 70 / 100
	case motifLayers:
		// Panels overlapping, the way a structure is drawn in section.
		width, height := slideWidth*46/100, slideHeight*62/100
		art = shapeGeom(8, "Layer Back", "roundRect", slideWidth-width-area.X/2, slideHeight*10/100, width, height, accent, 10, 0) +
			shapeGeom(9, "Layer Middle", "roundRect", slideWidth-width-area.X/2-slideWidth*7/100, slideHeight*20/100,
				width, height, accent, 17, 0) +
			shapeGeom(10, "Layer Front", "roundRect", slideWidth-width-area.X/2-slideWidth*14/100, slideHeight*30/100,
				width, height, accent, 26, 0)
		titleTop = slideHeight * 34 / 100
		titleWidth = area.Width * 52 / 100
	case motifWash:
		// The whole page washed in the brand hue, the title reversed out of it.
		// A wash deepens toward the page's own colour. Mixing toward the ink turns
		// a midnight palette's wash pale, because on a dark theme the ink is white.
		deep := mixColor(accent, palette.Surface, 0.55)
		if !palette.Dark {
			deep = mixColor(accent, palette.Ink, 0.42)
		}
		middle := mixColor(accent, deep, 0.5)
		ink = readableInk(middle, palette.Surface, palette.Ink)
		art = shapeWash(8, "Wash", 0, 0, slideWidth, slideHeight, accent, deep, 45) +
			shapeOutline(9, "Wash Ring", "ellipse", slideWidth-slideWidth*26/100, -slideHeight*18/100,
				slideWidth*34/100, slideWidth*34/100, ink, 12700, 30)
		subtitleInk = mixColor(ink, middle, 0.30)
		titleTop = slideHeight * 46 / 100
	}
	return art +
		placeholderShape(2, "Title 1", "ctrTitle", -1, area.X, titleTop, titleWidth, 1737360,
			textBody("l", "b", f.CoverSize, true, ink, "프레젠테이션 제목")) +
		placeholderShape(3, "Subtitle 2", "subTitle", 1, area.X, titleTop+1874520, titleWidth, 731520,
			textBody("l", "t", 2000, false, subtitleInk, "부제목 또는 한 줄 요약"))
}

// motifEcho is the quiet repeat a metaphor family puts on its content slides.
// The figure is stated once on the cover; after that it only has to be present.
func (f layoutFamily) motifEcho(id int, palette BuiltinPalette, area Frame) string {
	accent := palette.Accents[0]
	switch f.Motif {
	case motifOrbit:
		size := slideWidth * 14 / 100
		return shapeGeom(id, "Orbit Echo", "ellipse", slideWidth-size*55/100, slideHeight-size*55/100, size, size, accent, 12, 0)
	case motifArc:
		width, height := slideWidth*90/100, slideHeight*70/100
		return shapeOutline(id, "Arc Echo", "ellipse", slideWidth-width*55/100, slideHeight-height*22/100, width, height, accent, 12700, 24)
	case motifDiagonal:
		return shapeGeom(id, "Diagonal Echo", "rtTriangle", 0, slideHeight*62/100, slideWidth*18/100, slideHeight*38/100, accent, 10, 0)
	case motifDots:
		dot := slideWidth * 9 / 1000
		gap := dot * 24 / 10
		art := ""
		for index := 0; index < 3; index++ {
			art += shapeGeom(id+index, fmt.Sprintf("Dot Echo %d", index), "ellipse",
				slideWidth-area.X-gap*index, slideHeight-slideHeight*9/100, dot, dot, accent, 100-index*28, 0)
		}
		return art
	case motifLayers:
		return shapeGeom(id, "Layer Echo", "roundRect", slideWidth-slideWidth*30/100, slideHeight*62/100,
			slideWidth*34/100, slideHeight*46/100, accent, 9, 0)
	case motifWash:
		return shapeWash(id, "Wash Edge", 0, slideHeight-91440, slideWidth, 91440, accent,
			mixColor(accent, palette.Ink, 0.45), 0)
	}
	return ""
}

// coverShapes composes the opening slide. This is the picture a gallery shows
// and the first thing an audience sees, so it is where the families differ
// most: the same palette on two covers should not look like the same template.
func (f layoutFamily) coverShapes(palette BuiltinPalette, area Frame) string {
	subtitleInk := mixColor(palette.Ink, palette.Surface, 0.42)
	switch f.Cover {
	case coverPanel:
		// A full-height panel down the left, with the title standing on it. A
		// sidebar family tints the panel instead of filling it, so its cover and
		// its content slides are recognisably the same design.
		panelWidth := slideWidth * 42 / 100
		fill := palette.Accents[0]
		ink := readableInk(fill, palette.Surface, palette.Ink)
		if f.Body == bodySide {
			panelWidth = f.sidebarWidth() + slideWidth*8/100
			fill, ink = f.columnFill(palette), palette.Ink
		}
		inset := 731520
		return shapeRect(8, "Cover Panel", 0, 0, panelWidth, slideHeight, fill) +
			shapeRect(9, "Cover Panel Edge", 0, 0, ifInt(f.Body == bodySide, 68580, 0), slideHeight, palette.Accents[0]) +
			placeholderShape(2, "Title 1", "ctrTitle", -1, inset, 2011680, panelWidth-inset*2, 2011680,
				textBody("l", "b", f.CoverSize, true, ink, "프레젠테이션 제목")) +
			placeholderShape(3, "Subtitle 2", "subTitle", 1, panelWidth+inset, 2011680,
				slideWidth-panelWidth-inset-f.Margin, 2011680,
				textBody("l", "b", 2000, false, subtitleInk, "부제목 또는 한 줄 요약"))
	case coverBand:
		// An accent band across the lower half, the title reversed out of it.
		bandTop := slideHeight * 47 / 100
		ink := readableInk(palette.Accents[0], palette.Surface, palette.Ink)
		return shapeRect(8, "Cover Band", 0, bandTop, slideWidth, slideHeight-bandTop, palette.Accents[0]) +
			placeholderShape(2, "Title 1", "ctrTitle", -1, area.X, bandTop+594360, area.Width, 1554480,
				textBody("l", "t", f.CoverSize, true, ink, "프레젠테이션 제목")) +
			placeholderShape(3, "Subtitle 2", "subTitle", 1, area.X, bandTop-822960, area.Width, 640080,
				textBody("l", "b", 2000, false, subtitleInk, "부제목 또는 한 줄 요약"))
	case coverPlate:
		// A plate the title sits inside, centred on the page.
		plateX, plateY := slideWidth*12/100, slideHeight*24/100
		plateWidth, plateHeight := slideWidth-plateX*2, slideHeight*52/100
		inset := 594360
		ruleX := plateX + inset
		if f.CoverAlign == "ctr" {
			ruleX = plateX + plateWidth/2 - 342900
		}
		return shapeRect(8, "Cover Plate", plateX, plateY, plateWidth, plateHeight,
			mixColor(palette.Surface, palette.Accents[0], ifElse(palette.Dark, 0.20, 0.10))) +
			shapeRect(9, "Plate Rule", ruleX, plateY+inset, 685800, 45720, palette.Accents[0]) +
			placeholderShape(2, "Title 1", "ctrTitle", -1, plateX+inset, plateY+inset+320040,
				plateWidth-inset*2, 1600200,
				textBody(f.CoverAlign, "b", f.CoverSize, true, palette.Ink, "프레젠테이션 제목")) +
			placeholderShape(3, "Subtitle 2", "subTitle", 1, plateX+inset, plateY+plateHeight-inset-457200,
				plateWidth-inset*2, 457200,
				textBody(f.CoverAlign, "b", 1800, false, subtitleInk, "부제목 또는 한 줄 요약"))
	case coverColumn:
		// The title and the subtitle in facing columns, divided by a hairline —
		// the same two-column measure the content slides are set in.
		column := slideWidth*34/100 - area.X
		gap := 457200
		rightX := area.X + column + gap
		return shapeRect(8, "Cover Rule", area.X, slideHeight*30/100, column, 45720, palette.Accents[0]) +
			shapeRect(9, "Cover Divider", rightX-gap/2, slideHeight*30/100, 9525, slideHeight*40/100,
				mixColor(palette.Surface, palette.Ink, ifElse(palette.Dark, 0.26, 0.16))) +
			placeholderShape(2, "Title 1", "ctrTitle", -1, area.X, slideHeight*30/100+320040, column, 2377440,
				textBody("l", "t", f.CoverSize, true, palette.Ink, "프레젠테이션 제목")) +
			placeholderShape(3, "Subtitle 2", "subTitle", 1, rightX, slideHeight*30/100+320040,
				slideWidth-rightX-f.Margin, 1828800,
				textBody("l", "t", 1800, false, subtitleInk, "부제목 또는 한 줄 요약"))
	case coverCorner:
		// A block in the upper corner, the title sitting under it.
		blockWidth, blockHeight := slideWidth*34/100, slideHeight*46/100
		return shapeRect(8, "Cover Block", slideWidth-blockWidth, 0, blockWidth, blockHeight, palette.Accents[0]) +
			shapeRect(9, "Cover Rule", area.X, slideHeight*58/100, 1371600, 68580, palette.Accents[0]) +
			placeholderShape(2, "Title 1", "ctrTitle", -1, area.X, slideHeight*58/100+274320,
				area.Width-blockWidth/2, 1600200,
				textBody("l", "t", f.CoverSize, true, palette.Ink, "프레젠테이션 제목")) +
			placeholderShape(3, "Subtitle 2", "subTitle", 1, area.X, slideHeight*58/100+1965960,
				area.Width-blockWidth/2, 685800,
				textBody("l", "t", 2000, false, subtitleInk, "부제목 또는 한 줄 요약"))
	case coverMotif:
		return f.motifCover(palette, area)
	}
	// The library's default: an accent bar, the title, the subtitle under it.
	coverTop := 2560320
	accentWidth := 1600200
	accentX := area.X
	if f.CoverAlign == "ctr" {
		accentX = (slideWidth - accentWidth) / 2
	}
	return shapeRect(8, "Cover Accent", accentX, coverTop-274320, accentWidth, 68580, palette.Accents[0]) +
		placeholderShape(2, "Title 1", "ctrTitle", -1, area.X, coverTop, area.Width, 1600200,
			textBody(f.CoverAlign, "b", f.CoverSize, true, palette.Ink, "프레젠테이션 제목")) +
		placeholderShape(3, "Subtitle 2", "subTitle", 1, area.X, coverTop+1737360, area.Width, 914400,
			textBody(f.CoverAlign, "t", 2000, false, subtitleInk, "부제목 또는 한 줄 요약"))
}

func builtinLayouts(design BuiltinDesign) []builtinLayout {
	family := design.Family
	area := family.contentArea()
	titleTop := family.titleTop()
	title, body := family.titleFrame(), family.bodyFrame()
	// Two-column layouts divide whatever the family gives the body, so a column or
	// sidebar family splits its own measure rather than the whole page.
	halfWidth := (body.Width - 457200) / 2
	rightX := body.X + halfWidth + 457200

	// A content slide's title and body, shared by most layouts.
	contentTitle := func(design BuiltinDesign) string {
		palette := design.Palette
		color := palette.Ink
		frame := title
		if family.Panel {
			color = panelInk(palette)
			frame = Frame{X: area.X, Y: 640080, Width: area.Width, Height: lineHeightFor(family.TitleSize) + 91440}
		}
		furniture := family.titleFurniture(8, palette, area)
		// A sidebar family paints its column instead of drawing furniture; the
		// column is the mark that ties the deck together.
		if family.Body == bodySide {
			furniture = shapeRect(8, "Sidebar", 0, 0, family.sidebarWidth(), slideHeight, family.columnFill(palette)) +
				shapeRect(9, "Sidebar Accent", 0, 0, 68580, slideHeight, palette.Accents[0])
		}
		if family.Footer {
			furniture += shapeRect(10, "Footer Rule", area.X, slideHeight-548640, area.Width, 9525,
				mixColor(palette.Surface, palette.Ink, ifElse(palette.Dark, 0.22, 0.12)))
		}
		// A metaphor family repeats its figure quietly, behind everything else.
		furniture = family.motifEcho(40, palette, area) + furniture
		eyebrow := ""
		if family.Eyebrow {
			eyebrow = placeholderShape(7, "Text Placeholder 6", "body", 9, frame.X, titleTop-320040, frame.Width, 274320,
				textBody(family.TitleAlign, "b", 1100, true, palette.Accents[0], "구역 이름"))
		}
		anchor := "b"
		if family.Body == bodyIndent || family.Body == bodySide {
			// A title set in its own column reads from the top of the column, not
			// from the baseline the body happens to start at.
			anchor = "t"
		}
		return furniture + eyebrow +
			placeholderShape(2, "Title 1", "title", -1, frame.X, frame.Y, frame.Width, frame.Height,
				textBody(family.TitleAlign, anchor, family.TitleSize, true, color, "제목을 입력하세요"))
	}

	return []builtinLayout{
		{
			Name: "제목 슬라이드", Type: "title", ShowMaster: family.Rail > 0 && family.Cover == coverStack,
			Shapes: func(d BuiltinDesign) string { return family.coverShapes(d.Palette, area) },
		},
		{
			Name: "구역 머리글", Type: "secHead", ShowMaster: family.Rail > 0,
			Shapes: func(d BuiltinDesign) string {
				palette := d.Palette
				return slideNumberPlaceholder(7, family, palette) +
					shapeRect(8, "Section Accent", area.X, 2377440, 685800, 68580, palette.Accents[0]) +
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
					placeholderShape(3, "Content Placeholder 2", "body", 1, body.X, body.Y, body.Width, body.Height,
						bulletBody(family.BodySize, d.Palette.Ink))
			},
		},
		{
			Name: "콘텐츠 2개", Type: "twoObj", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string {
				return contentTitle(d) +
					placeholderShape(3, "Content Placeholder 2", "body", 1, body.X, body.Y, halfWidth, body.Height,
						bulletBody(family.BodySize-100, d.Palette.Ink)) +
					placeholderShape(4, "Content Placeholder 3", "body", 2, rightX, body.Y, halfWidth, body.Height,
						bulletBody(family.BodySize-100, d.Palette.Ink))
			},
		},
		{
			Name: "비교", Type: "twoTxTwoObj", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string {
				palette := d.Palette
				const headerHeight = 502920
				subBodyTop := body.Y + headerHeight + 137160
				subBodyHeight := body.Height - headerHeight - 137160
				return contentTitle(d) +
					placeholderShape(3, "Text Placeholder 2", "body", 1, body.X, body.Y, halfWidth, headerHeight,
						textBody("l", "ctr", 2000, true, palette.Accents[0], "왼쪽 항목")) +
					placeholderShape(4, "Content Placeholder 3", "body", 2, body.X, subBodyTop, halfWidth, subBodyHeight,
						bulletBody(family.BodySize-200, palette.Ink)) +
					placeholderShape(5, "Text Placeholder 4", "body", 3, rightX, body.Y, halfWidth, headerHeight,
						textBody("l", "ctr", 2000, true, palette.Accents[1], "오른쪽 항목")) +
					placeholderShape(6, "Content Placeholder 5", "body", 4, rightX, subBodyTop, halfWidth, subBodyHeight,
						bulletBody(family.BodySize-200, palette.Ink))
			},
		},
		{
			Name: "핵심 인용", Type: "obj", ShowMaster: family.Rail > 0,
			Shapes: func(d BuiltinDesign) string {
				palette := d.Palette
				return slideNumberPlaceholder(7, family, palette) +
					shapeRect(8, "Quote Accent", area.X, 1828800, 274320, 274320, palette.Accents[0]) +
					placeholderShape(2, "Title 1", "title", -1, area.X, 2377440, area.Width, 1828800,
						textBody("l", "ctr", family.TitleSize-200, false, palette.Ink, "기억에 남길 한 문장을 입력하세요")) +
					placeholderShape(3, "Text Placeholder 2", "body", 1, area.X, 4389120, area.Width, 457200,
						textBody("l", "t", 1600, false, mixColor(palette.Ink, palette.Surface, 0.45), "출처 또는 화자"))
			},
		},
		{
			Name: "캡션 있는 그림", Type: "picTx", ShowMaster: true,
			Shapes: func(d BuiltinDesign) string {
				pictureWidth := body.Width*3/5 - 228600
				captionX := body.X + pictureWidth + 457200
				captionWidth := body.Width - pictureWidth - 457200
				return contentTitle(d) +
					placeholderShape(3, "Picture Placeholder 2", "pic", 1, body.X, body.Y, pictureWidth, body.Height,
						`<a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p>`) +
					placeholderShape(4, "Text Placeholder 3", "body", 2, captionX, body.Y, captionWidth, body.Height,
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
				return slideNumberPlaceholder(7, family, palette) +
					shapeRect(8, "Closing Accent", area.X, 2377440, 1600200, 68580, palette.Accents[0]) +
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
