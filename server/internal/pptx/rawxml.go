package pptx

import (
	"strings"
)

// The structures below mirror only the fragments of DrawingML and
// PresentationML that Ptium needs. Everything else in a template is copied
// through byte-for-byte, so unsupported constructs cannot be damaged.

type rawShapeTree struct {
	Shapes   []rawShape     `xml:"sp"`
	Pictures []rawShape     `xml:"pic"`
	Frames   []rawShape     `xml:"graphicFrame"`
	Groups   []rawShapeTree `xml:"grpSp"`
}

func (t rawShapeTree) flatten() []rawShape {
	result := make([]rawShape, 0, len(t.Shapes)+len(t.Pictures)+len(t.Frames))
	result = append(result, t.Shapes...)
	result = append(result, t.Pictures...)
	result = append(result, t.Frames...)
	for _, group := range t.Groups {
		result = append(result, group.flatten()...)
	}
	return result
}

type rawShape struct {
	NvSpPr           *rawNonVisual `xml:"nvSpPr"`
	NvPicPr          *rawNonVisual `xml:"nvPicPr"`
	NvGraphicFramePr *rawNonVisual `xml:"nvGraphicFramePr"`
	SpPr             *rawShapeProp `xml:"spPr"`
	Xfrm             *rawXfrm      `xml:"xfrm"`
	TxBody           *rawTxBody    `xml:"txBody"`
}

type rawNonVisual struct {
	CNvPr struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"name,attr"`
	} `xml:"cNvPr"`
	NvPr struct {
		Ph *rawPlaceholderRef `xml:"ph"`
	} `xml:"nvPr"`
}

type rawPlaceholderRef struct {
	Type   string `xml:"type,attr"`
	Idx    string `xml:"idx,attr"`
	Orient string `xml:"orient,attr"`
	Size   string `xml:"sz,attr"`
}

type rawShapeProp struct {
	Xfrm      *rawXfrm      `xml:"xfrm"`
	SolidFill *rawSolidFill `xml:"solidFill"`
	PrstGeom  *struct {
		Prst string `xml:"prst,attr"`
	} `xml:"prstGeom"`
}

// fill returns a hex value or an unresolved scheme color name for a shape's
// solid fill, or an empty string when the shape is not solidly filled.
func (s rawShape) fill() string {
	if s.SpPr == nil || s.SpPr.SolidFill == nil {
		return ""
	}
	fill := s.SpPr.SolidFill
	switch {
	case fill.SrgbClr != nil && fill.SrgbClr.Val != "":
		return strings.ToUpper(fill.SrgbClr.Val)
	case fill.SchemeClr != nil && fill.SchemeClr.Val != "":
		return fill.SchemeClr.Val
	case fill.SysClr != nil && fill.SysClr.LastClr != "":
		return strings.ToUpper(fill.SysClr.LastClr)
	}
	return ""
}

func (s rawShape) geometryPreset() string {
	if s.SpPr == nil || s.SpPr.PrstGeom == nil {
		return ""
	}
	return s.SpPr.PrstGeom.Prst
}

type rawXfrm struct {
	Off struct {
		X int `xml:"x,attr"`
		Y int `xml:"y,attr"`
	} `xml:"off"`
	Ext struct {
		CX int `xml:"cx,attr"`
		CY int `xml:"cy,attr"`
	} `xml:"ext"`
}

type rawTxBody struct {
	BodyPr struct {
		Vert string `xml:"vert,attr"`
	} `xml:"bodyPr"`
	LstStyle rawTextStyle `xml:"lstStyle"`
	Para     []struct {
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"p"`
}

type rawTextStyle struct {
	Lvl1 rawLevelStyle `xml:"lvl1pPr"`
	Lvl2 rawLevelStyle `xml:"lvl2pPr"`
	Lvl3 rawLevelStyle `xml:"lvl3pPr"`
	Lvl4 rawLevelStyle `xml:"lvl4pPr"`
	Lvl5 rawLevelStyle `xml:"lvl5pPr"`
	Lvl6 rawLevelStyle `xml:"lvl6pPr"`
	Lvl7 rawLevelStyle `xml:"lvl7pPr"`
	Lvl8 rawLevelStyle `xml:"lvl8pPr"`
	Lvl9 rawLevelStyle `xml:"lvl9pPr"`
}

type rawLevelStyle struct {
	DefRPr struct {
		Size      int           `xml:"sz,attr"`
		Bold      string        `xml:"b,attr"`
		Italic    string        `xml:"i,attr"`
		SolidFill *rawSolidFill `xml:"solidFill"`
		Latin     struct {
			Typeface string `xml:"typeface,attr"`
		} `xml:"latin"`
	} `xml:"defRPr"`
}

// color returns a hex value or an unresolved scheme color name.
func (l rawLevelStyle) color() string {
	fill := l.DefRPr.SolidFill
	if fill == nil {
		return ""
	}
	switch {
	case fill.SrgbClr != nil && fill.SrgbClr.Val != "":
		return strings.ToUpper(fill.SrgbClr.Val)
	case fill.SchemeClr != nil && fill.SchemeClr.Val != "":
		return fill.SchemeClr.Val
	case fill.SysClr != nil && fill.SysClr.LastClr != "":
		return strings.ToUpper(fill.SysClr.LastClr)
	}
	return ""
}

func (l rawLevelStyle) bold() bool { return l.DefRPr.Bold == "1" || l.DefRPr.Bold == "true" }

func (s rawTextStyle) levelStyle(index int) rawLevelStyle {
	levels := []rawLevelStyle{s.Lvl1, s.Lvl2, s.Lvl3, s.Lvl4, s.Lvl5, s.Lvl6, s.Lvl7, s.Lvl8, s.Lvl9}
	if index < 1 || index > len(levels) {
		return rawLevelStyle{}
	}
	return levels[index-1]
}

func (s rawTextStyle) level(index int) int {
	levels := []rawLevelStyle{s.Lvl1, s.Lvl2, s.Lvl3, s.Lvl4, s.Lvl5, s.Lvl6, s.Lvl7, s.Lvl8, s.Lvl9}
	if index < 1 || index > len(levels) {
		return 0
	}
	return levels[index-1].DefRPr.Size
}

func (s rawShape) nonVisual() *rawNonVisual {
	switch {
	case s.NvSpPr != nil:
		return s.NvSpPr
	case s.NvPicPr != nil:
		return s.NvPicPr
	case s.NvGraphicFramePr != nil:
		return s.NvGraphicFramePr
	}
	return nil
}

func (s rawShape) placeholder() *rawPlaceholderRef {
	if nonVisual := s.nonVisual(); nonVisual != nil {
		return nonVisual.NvPr.Ph
	}
	return nil
}

func (s rawShape) name() string {
	if nonVisual := s.nonVisual(); nonVisual != nil {
		return nonVisual.CNvPr.Name
	}
	return ""
}

func (s rawShape) geometry() (x, y, width, height int, ok bool) {
	transform := s.Xfrm
	if s.SpPr != nil && s.SpPr.Xfrm != nil {
		transform = s.SpPr.Xfrm
	}
	if transform == nil || transform.Ext.CX <= 0 || transform.Ext.CY <= 0 {
		return 0, 0, 0, 0, false
	}
	return transform.Off.X, transform.Off.Y, transform.Ext.CX, transform.Ext.CY, true
}

func (s rawShape) overrideSize(level int) int {
	if s.TxBody == nil {
		return 0
	}
	return s.TxBody.LstStyle.level(level)
}

func (s rawShape) overrideStyle(level int) rawLevelStyle {
	if s.TxBody == nil {
		return rawLevelStyle{}
	}
	return s.TxBody.LstStyle.levelStyle(level)
}

func (s rawShape) verticalText() bool {
	if s.TxBody == nil {
		return false
	}
	return strings.HasPrefix(s.TxBody.BodyPr.Vert, "vert") || s.TxBody.BodyPr.Vert == "eaVert"
}

func (s rawShape) sampleText() string {
	if s.TxBody == nil || len(s.TxBody.Para) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, run := range s.TxBody.Para[0].Runs {
		builder.WriteString(run.Text)
	}
	text := strings.TrimSpace(builder.String())
	if len([]rune(text)) > 120 {
		text = string([]rune(text)[:120])
	}
	return text
}

type rawFillHolder struct {
	BgPr struct {
		SolidFill *rawSolidFill `xml:"solidFill"`
	} `xml:"bgPr"`
	BgRef *struct {
		SchemeClr *struct {
			Val string `xml:"val,attr"`
		} `xml:"schemeClr"`
		SrgbClr *struct {
			Val string `xml:"val,attr"`
		} `xml:"srgbClr"`
	} `xml:"bgRef"`
}

type rawSolidFill struct {
	SchemeClr *struct {
		Val string `xml:"val,attr"`
	} `xml:"schemeClr"`
	SrgbClr *struct {
		Val string `xml:"val,attr"`
	} `xml:"srgbClr"`
	SysClr *struct {
		LastClr string `xml:"lastClr,attr"`
	} `xml:"sysClr"`
}

// solidColor returns either a hex value or an unresolved scheme color name.
func (f rawFillHolder) solidColor() string {
	if fill := f.BgPr.SolidFill; fill != nil {
		switch {
		case fill.SrgbClr != nil && fill.SrgbClr.Val != "":
			return strings.ToUpper(fill.SrgbClr.Val)
		case fill.SchemeClr != nil && fill.SchemeClr.Val != "":
			return fill.SchemeClr.Val
		case fill.SysClr != nil && fill.SysClr.LastClr != "":
			return strings.ToUpper(fill.SysClr.LastClr)
		}
	}
	if reference := f.BgRef; reference != nil {
		switch {
		case reference.SrgbClr != nil && reference.SrgbClr.Val != "":
			return strings.ToUpper(reference.SrgbClr.Val)
		case reference.SchemeClr != nil && reference.SchemeClr.Val != "":
			return reference.SchemeClr.Val
		}
	}
	return ""
}

type rawColorRef struct {
	SrgbClr *struct {
		Val string `xml:"val,attr"`
	} `xml:"srgbClr"`
	SysClr *struct {
		Val     string `xml:"val,attr"`
		LastClr string `xml:"lastClr,attr"`
	} `xml:"sysClr"`
}

func (c rawColorRef) value() string {
	if c.SrgbClr != nil && c.SrgbClr.Val != "" {
		return strings.ToUpper(c.SrgbClr.Val)
	}
	if c.SysClr != nil {
		if c.SysClr.LastClr != "" {
			return strings.ToUpper(c.SysClr.LastClr)
		}
		switch c.SysClr.Val {
		case "window":
			return "FFFFFF"
		case "windowText":
			return "000000"
		}
	}
	return ""
}

type rawFontRef struct {
	Latin struct {
		Typeface string `xml:"typeface,attr"`
	} `xml:"latin"`
	EastAsian struct {
		Typeface string `xml:"typeface,attr"`
	} `xml:"ea"`
}
