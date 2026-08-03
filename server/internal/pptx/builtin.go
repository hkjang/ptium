package pptx

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// BuiltinPalette describes one of the designs Ptium ships with so the product
// produces a polished deck before a customer uploads their own template.
type BuiltinPalette struct {
	Key        string
	Name       string
	Background string
	Surface    string
	Text       string
	Muted      string
	Accent     string
	Accent2    string
	Accent3    string
	MajorLatin string
	MinorLatin string
	EastAsian  string
}

var builtinPalettes = map[string]BuiltinPalette{
	"aurora": {
		Key: "aurora", Name: "Aurora", Background: "12102A", Surface: "1E1B3A", Text: "F6F4FF", Muted: "C7C0F0",
		Accent: "8B7BFF", Accent2: "38BDF8", Accent3: "F472B6",
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
	},
	"modern": {
		Key: "modern", Name: "Modern", Background: "FFFFFF", Surface: "F1F5F9", Text: "0F172A", Muted: "475569",
		Accent: "4F46E5", Accent2: "0EA5E9", Accent3: "10B981",
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
	},
	"paper": {
		Key: "paper", Name: "Paper", Background: "F5F1E8", Surface: "EAE3D5", Text: "241F1A", Muted: "5D554A",
		Accent: "C2410C", Accent2: "0F766E", Accent3: "A16207",
		MajorLatin: "Georgia", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
	},
	"mint": {
		Key: "mint", Name: "Mint", Background: "F2FBF7", Surface: "DDF3E9", Text: "0F2E24", Muted: "3D6B5B",
		Accent: "0F9D74", Accent2: "0284C7", Accent3: "CA8A04",
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
	},
	"graphite": {
		Key: "graphite", Name: "Graphite", Background: "1C1F24", Surface: "282C33", Text: "F4F5F7", Muted: "B7BDC6",
		Accent: "9CA3AF", Accent2: "60A5FA", Accent3: "FBBF24",
		MajorLatin: "Aptos Display", MinorLatin: "Aptos", EastAsian: "맑은 고딕",
	},
}

// BuiltinPaletteKeys lists the shipped designs in a stable order.
func BuiltinPaletteKeys() []string {
	keys := make([]string, 0, len(builtinPalettes))
	for key := range builtinPalettes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// LookupBuiltinPalette resolves a palette name, falling back to Aurora.
func LookupBuiltinPalette(key string) BuiltinPalette {
	if palette, ok := builtinPalettes[strings.ToLower(strings.TrimSpace(key))]; ok {
		return palette
	}
	return builtinPalettes["aurora"]
}

// Slide geometry shared by every built-in layout, in EMU on a 16:9 canvas.
const (
	builtinWidth   = 12192000
	builtinHeight  = 6858000
	builtinMargin  = 838200
	builtinContent = builtinWidth - 2*builtinMargin
)

// BuiltinTemplate assembles a complete, professionally proportioned PowerPoint
// template for the requested palette. The result is a real .pptx package, so
// it flows through exactly the same analysis and rendering path as a template
// a customer uploads.
func BuiltinTemplate(paletteKey string) ([]byte, error) {
	palette := LookupBuiltinPalette(paletteKey)
	layouts := builtinLayouts(palette)

	pkg := &Package{parts: map[string][]byte{}}
	pkg.SetText("_rels/.rels", relationshipsDocument(
		`<Relationship Id="rId1" Type="`+relationshipNamespace+`/officeDocument" Target="ppt/presentation.xml"/>`+
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>`+
			`<Relationship Id="rId3" Type="`+relationshipNamespace+`/extended-properties" Target="docProps/app.xml"/>`))
	pkg.SetText("docProps/core.xml", corePropertiesXML(Deck{Title: "Ptium " + palette.Name, Author: "Ptium"}))
	pkg.SetText("docProps/app.xml", xmlDeclaration+`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>Ptium</Application><PresentationFormat>16:9</PresentationFormat><Slides>0</Slides><Notes>0</Notes><HiddenSlides>0</HiddenSlides><AppVersion>16.0000</AppVersion></Properties>`)
	pkg.SetText("ppt/presProps.xml", xmlDeclaration+`<p:presentationPr `+presentationNamespaces+`/>`)
	pkg.SetText("ppt/viewProps.xml", xmlDeclaration+`<p:viewPr `+presentationNamespaces+`><p:normalViewPr/><p:slideViewPr><p:cSldViewPr snapToGrid="0"><p:cViewPr varScale="1"><p:scale><a:sx n="80" d="100"/><a:sy n="80" d="100"/></p:scale><p:origin x="0" y="0"/></p:cViewPr><p:guideLst/></p:cSldViewPr></p:slideViewPr><p:gridSpacing cx="76200" cy="76200"/></p:viewPr>`)
	pkg.SetText("ppt/tableStyles.xml", xmlDeclaration+`<a:tblStyleLst xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"/>`)
	pkg.SetText("ppt/theme/theme1.xml", builtinTheme(palette))
	pkg.SetText("ppt/slideMasters/slideMaster1.xml", builtinMaster(palette, len(layouts)))

	masterRels := `<Relationship Id="rId100" Type="` + relationshipNamespace + `/theme" Target="../theme/theme1.xml"/>`
	for index := range layouts {
		masterRels += fmt.Sprintf(`<Relationship Id="rId%d" Type="%s/slideLayout" Target="../slideLayouts/slideLayout%d.xml"/>`, index+1, relationshipNamespace, index+1)
	}
	pkg.SetText("ppt/slideMasters/_rels/slideMaster1.xml.rels", relationshipsDocument(masterRels))

	for index, layout := range layouts {
		part := fmt.Sprintf("ppt/slideLayouts/slideLayout%d.xml", index+1)
		pkg.SetText(part, layout.xml(palette))
		pkg.SetText(RelationshipsPath(part), relationshipsDocument(
			`<Relationship Id="rId1" Type="`+relationshipNamespace+`/slideMaster" Target="../slideMasters/slideMaster1.xml"/>`))
	}

	pkg.SetText("ppt/presentation.xml", xmlDeclaration+`<p:presentation `+presentationNamespaces+` saveSubsetFonts="1">`+
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`+
		`<p:sldIdLst/>`+
		fmt.Sprintf(`<p:sldSz cx="%d" cy="%d"/><p:notesSz cx="6858000" cy="9144000"/>`, builtinWidth, builtinHeight)+
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

func builtinTheme(palette BuiltinPalette) string {
	return xmlDeclaration + `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Ptium ` + escapeAttribute(palette.Name) + `"><a:themeElements>` +
		`<a:clrScheme name="Ptium ` + escapeAttribute(palette.Name) + `">` +
		`<a:dk1><a:srgbClr val="` + palette.Text + `"/></a:dk1><a:lt1><a:srgbClr val="` + palette.Background + `"/></a:lt1>` +
		`<a:dk2><a:srgbClr val="` + palette.Muted + `"/></a:dk2><a:lt2><a:srgbClr val="` + palette.Surface + `"/></a:lt2>` +
		`<a:accent1><a:srgbClr val="` + palette.Accent + `"/></a:accent1><a:accent2><a:srgbClr val="` + palette.Accent2 + `"/></a:accent2>` +
		`<a:accent3><a:srgbClr val="` + palette.Accent3 + `"/></a:accent3><a:accent4><a:srgbClr val="` + palette.Muted + `"/></a:accent4>` +
		`<a:accent5><a:srgbClr val="` + palette.Surface + `"/></a:accent5><a:accent6><a:srgbClr val="` + palette.Text + `"/></a:accent6>` +
		`<a:hlink><a:srgbClr val="` + palette.Accent2 + `"/></a:hlink><a:folHlink><a:srgbClr val="` + palette.Accent3 + `"/></a:folHlink></a:clrScheme>` +
		`<a:fontScheme name="Ptium ` + escapeAttribute(palette.Name) + `">` +
		`<a:majorFont><a:latin typeface="` + escapeAttribute(palette.MajorLatin) + `"/><a:ea typeface="` + escapeAttribute(palette.EastAsian) + `"/><a:cs typeface=""/></a:majorFont>` +
		`<a:minorFont><a:latin typeface="` + escapeAttribute(palette.MinorLatin) + `"/><a:ea typeface="` + escapeAttribute(palette.EastAsian) + `"/><a:cs typeface=""/></a:minorFont></a:fontScheme>` +
		`<a:fmtScheme name="Ptium ` + escapeAttribute(palette.Name) + `">` +
		`<a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:tint val="80000"/></a:schemeClr></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:shade val="90000"/></a:schemeClr></a:solidFill></a:fillStyleLst>` +
		`<a:lnStyleLst><a:ln w="9525" cap="flat"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln><a:ln w="19050" cap="flat"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln><a:ln w="28575" cap="flat"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln></a:lnStyleLst>` +
		`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
		`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst>` +
		`</a:fmtScheme></a:themeElements><a:objectDefaults/><a:extraClrSchemeLst/></a:theme>`
}

func builtinMaster(palette BuiltinPalette, layoutCount int) string {
	var layoutIDs strings.Builder
	for index := 1; index <= layoutCount; index++ {
		fmt.Fprintf(&layoutIDs, `<p:sldLayoutId id="%d" r:id="rId%d"/>`, 2147483649+index, index)
	}
	// A thin accent rule under the title area gives every slide a consistent
	// visual anchor without competing with the content.
	decoration := shapeRect(9, "Accent Rule", builtinMargin, 1866900, 1143000, 45720, palette.Accent)
	footer := placeholderShape(10, "Slide Number Placeholder 3", "sldNum", 12,
		builtinWidth-builtinMargin-1143000, 6172200, 1143000, 365760,
		`<a:bodyPr vert="horz" lIns="0" tIns="45720" rIns="0" bIns="45720" rtlCol="0" anchor="ctr"/><a:lstStyle><a:lvl1pPr algn="r"><a:defRPr sz="1100"><a:solidFill><a:srgbClr val="`+palette.Muted+`"/></a:solidFill></a:defRPr></a:lvl1pPr></a:lstStyle>`+
			`<a:p><a:fld id="{B7B5A0C4-2C2F-4E28-9E86-9E0F0F5B0E11}" type="slidenum"><a:rPr lang="ko-KR" smtClean="0"/><a:t>‹#›</a:t></a:fld><a:endParaRPr lang="ko-KR"/></a:p>`)

	return xmlDeclaration + `<p:sldMaster ` + presentationNamespaces + `><p:cSld>` +
		`<p:bg><p:bgPr><a:solidFill><a:schemeClr val="bg1"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>` +
		`<p:spTree>` + emptyGroupHeader +
		placeholderShape(2, "Title Placeholder 1", "title", -1, builtinMargin, 685800, builtinContent, 1097280,
			`<a:bodyPr vert="horz" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0" anchor="b"><a:normAutofit/></a:bodyPr><a:lstStyle/><a:p><a:r><a:rPr lang="ko-KR"/><a:t>마스터 제목 스타일 편집</a:t></a:r></a:p>`) +
		placeholderShape(3, "Text Placeholder 2", "body", 1, builtinMargin, 2103120, builtinContent, 3840480,
			`<a:bodyPr vert="horz" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0"><a:normAutofit/></a:bodyPr><a:lstStyle/>`+
				`<a:p><a:r><a:rPr lang="ko-KR"/><a:t>마스터 텍스트 스타일 편집</a:t></a:r></a:p>`) +
		decoration + footer +
		`</p:spTree></p:cSld>` +
		`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
		`<p:sldLayoutIdLst>` + layoutIDs.String() + `</p:sldLayoutIdLst>` +
		`<p:txStyles>` +
		`<p:titleStyle><a:lvl1pPr algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:lnSpc><a:spcPct val="95000"/></a:lnSpc><a:spcBef><a:spcPct val="0"/></a:spcBef><a:defRPr sz="4000" b="1" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mj-lt"/><a:ea typeface="+mj-ea"/><a:cs typeface="+mj-cs"/></a:defRPr></a:lvl1pPr></p:titleStyle>` +
		`<p:bodyStyle>` +
		builtinBodyLevel(1, 0, 342900, 1800, palette.Text, "•", palette.Accent) +
		builtinBodyLevel(2, 457200, 285750, 1600, palette.Muted, "–", palette.Accent2) +
		builtinBodyLevel(3, 914400, 228600, 1400, palette.Muted, "•", palette.Accent3) +
		builtinBodyLevel(4, 1371600, 228600, 1300, palette.Muted, "–", palette.Accent) +
		builtinBodyLevel(5, 1828800, 228600, 1200, palette.Muted, "•", palette.Accent2) +
		`</p:bodyStyle>` +
		`<p:otherStyle><a:lvl1pPr marL="0" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1200" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:otherStyle>` +
		`</p:txStyles></p:sldMaster>`
}

func builtinBodyLevel(level, marginLeft, indent, size int, color, bullet, bulletColor string) string {
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
	Shapes     func(palette BuiltinPalette) string
}

func (l builtinLayout) xml(palette BuiltinPalette) string {
	showMaster := ""
	if !l.ShowMaster {
		showMaster = ` showMasterSp="0"`
	}
	return xmlDeclaration + `<p:sldLayout ` + presentationNamespaces + ` type="` + l.Type + `" preserve="1"` + showMaster + `>` +
		`<p:cSld name="` + escapeAttribute(l.Name) + `"><p:spTree>` + emptyGroupHeader + l.Shapes(palette) +
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

func builtinLayouts(palette BuiltinPalette) []builtinLayout {
	const titleY = 685800
	const titleH = 1097280
	const bodyY = 2103120
	const bodyH = 3840480
	halfWidth := (builtinContent - 457200) / 2
	rightX := builtinMargin + halfWidth + 457200

	return []builtinLayout{
		{
			Name: "제목 슬라이드", Type: "title", ShowMaster: false,
			Shapes: func(p BuiltinPalette) string {
				return shapeRect(8, "Cover Accent", builtinMargin, 2286000, 1600200, 68580, p.Accent) +
					placeholderShape(2, "Title 1", "ctrTitle", -1, builtinMargin, 2560320, builtinContent, 1600200,
						textBody("l", "b", 5400, true, p.Text, "프레젠테이션 제목")) +
					placeholderShape(3, "Subtitle 2", "subTitle", 1, builtinMargin, 4297680, builtinContent, 914400,
						textBody("l", "t", 2000, false, p.Muted, "부제목 또는 한 줄 요약"))
			},
		},
		{
			Name: "구역 머리글", Type: "secHead", ShowMaster: false,
			Shapes: func(p BuiltinPalette) string {
				return shapeRect(8, "Section Accent", builtinMargin, 2377440, 685800, 68580, p.Accent) +
					placeholderShape(2, "Title 1", "title", -1, builtinMargin, 2651760, builtinContent, 1097280,
						textBody("l", "b", 4400, true, p.Text, "구역 제목")) +
					placeholderShape(3, "Text Placeholder 2", "body", 1, builtinMargin, 3840480, builtinContent, 731520,
						textBody("l", "t", 1800, false, p.Muted, "이 구역에서 다룰 내용을 한 문장으로 소개합니다"))
			},
		},
		{
			Name: "제목 및 내용", Type: "obj", ShowMaster: true,
			Shapes: func(p BuiltinPalette) string {
				return placeholderShape(2, "Title 1", "title", -1, builtinMargin, titleY, builtinContent, titleH,
					textBody("l", "b", 4000, true, p.Text, "제목을 입력하세요")) +
					placeholderShape(3, "Content Placeholder 2", "body", 1, builtinMargin, bodyY, builtinContent, bodyH,
						bulletBody(1800, p.Text))
			},
		},
		{
			Name: "콘텐츠 2개", Type: "twoObj", ShowMaster: true,
			Shapes: func(p BuiltinPalette) string {
				return placeholderShape(2, "Title 1", "title", -1, builtinMargin, titleY, builtinContent, titleH,
					textBody("l", "b", 4000, true, p.Text, "제목을 입력하세요")) +
					placeholderShape(3, "Content Placeholder 2", "body", 1, builtinMargin, bodyY, halfWidth, bodyH,
						bulletBody(1700, p.Text)) +
					placeholderShape(4, "Content Placeholder 3", "body", 2, rightX, bodyY, halfWidth, bodyH,
						bulletBody(1700, p.Text))
			},
		},
		{
			Name: "비교", Type: "twoTxTwoObj", ShowMaster: true,
			Shapes: func(p BuiltinPalette) string {
				const headerH = 502920
				headerY := bodyY
				subBodyY := bodyY + headerH + 137160
				subBodyH := bodyH - headerH - 137160
				return placeholderShape(2, "Title 1", "title", -1, builtinMargin, titleY, builtinContent, titleH,
					textBody("l", "b", 4000, true, p.Text, "제목을 입력하세요")) +
					placeholderShape(3, "Text Placeholder 2", "body", 1, builtinMargin, headerY, halfWidth, headerH,
						textBody("l", "ctr", 2000, true, p.Accent, "왼쪽 항목")) +
					placeholderShape(4, "Content Placeholder 3", "body", 2, builtinMargin, subBodyY, halfWidth, subBodyH,
						bulletBody(1600, p.Text)) +
					placeholderShape(5, "Text Placeholder 4", "body", 3, rightX, headerY, halfWidth, headerH,
						textBody("l", "ctr", 2000, true, p.Accent2, "오른쪽 항목")) +
					placeholderShape(6, "Content Placeholder 5", "body", 4, rightX, subBodyY, halfWidth, subBodyH,
						bulletBody(1600, p.Text))
			},
		},
		{
			Name: "핵심 인용", Type: "obj", ShowMaster: false,
			Shapes: func(p BuiltinPalette) string {
				return shapeRect(8, "Quote Accent", builtinMargin, 1828800, 274320, 274320, p.Accent) +
					placeholderShape(2, "Title 1", "title", -1, builtinMargin, 2377440, builtinContent, 1828800,
						textBody("l", "ctr", 3200, false, p.Text, "기억에 남길 한 문장을 입력하세요")) +
					placeholderShape(3, "Text Placeholder 2", "body", 1, builtinMargin, 4389120, builtinContent, 457200,
						textBody("l", "t", 1600, false, p.Muted, "출처 또는 화자"))
			},
		},
		{
			Name: "캡션 있는 그림", Type: "picTx", ShowMaster: true,
			Shapes: func(p BuiltinPalette) string {
				pictureWidth := builtinContent*3/5 - 228600
				captionX := builtinMargin + pictureWidth + 457200
				captionWidth := builtinContent - pictureWidth - 457200
				return placeholderShape(2, "Title 1", "title", -1, builtinMargin, titleY, builtinContent, titleH,
					textBody("l", "b", 4000, true, p.Text, "제목을 입력하세요")) +
					placeholderShape(3, "Picture Placeholder 2", "pic", 1, builtinMargin, bodyY, pictureWidth, bodyH,
						`<a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p>`) +
					placeholderShape(4, "Text Placeholder 3", "body", 2, captionX, bodyY, captionWidth, bodyH,
						bulletBody(1600, p.Text))
			},
		},
		{
			Name: "제목만", Type: "titleOnly", ShowMaster: true,
			Shapes: func(p BuiltinPalette) string {
				return placeholderShape(2, "Title 1", "title", -1, builtinMargin, titleY, builtinContent, titleH,
					textBody("l", "b", 4000, true, p.Text, "제목을 입력하세요"))
			},
		},
		{
			Name: "마무리", Type: "secHead", ShowMaster: false,
			Shapes: func(p BuiltinPalette) string {
				return shapeRect(8, "Closing Accent", builtinMargin, 2377440, 1600200, 68580, p.Accent) +
					placeholderShape(2, "Title 1", "ctrTitle", -1, builtinMargin, 2651760, builtinContent, 1097280,
						textBody("l", "b", 4400, true, p.Text, "감사합니다")) +
					placeholderShape(3, "Subtitle 2", "subTitle", 1, builtinMargin, 3840480, builtinContent, 1097280,
						textBody("l", "t", 1800, false, p.Muted, "다음 단계와 연락처를 남기세요"))
			},
		},
		{
			Name: "빈 화면", Type: "blank", ShowMaster: true,
			Shapes: func(p BuiltinPalette) string { return "" },
		},
	}
}
