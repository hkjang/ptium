package pptx

import (
	"encoding/xml"
	"errors"
	"io"
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
	// Children keeps the shapes in document order, which is paint order. The
	// typed slices above are what most of the analyzer wants; drawing a
	// template's artwork needs to know what sits on top of what.
	Children []rawTreeChild
	// Transform is a group's own frame, needed to place its children.
	Transform *rawGroupXfrm
}

type rawTreeChild struct {
	Kind  string // "sp" | "pic" | "graphicFrame" | "grpSp"
	Shape rawShape
	Group *rawShapeTree
}

// rawGroupXfrm is a group shape's transform. A group maps its child coordinate
// space onto its own frame, so children have to be projected through it.
type rawGroupXfrm struct {
	Off struct {
		X int `xml:"x,attr"`
		Y int `xml:"y,attr"`
	} `xml:"off"`
	Ext struct {
		CX int `xml:"cx,attr"`
		CY int `xml:"cy,attr"`
	} `xml:"ext"`
	ChOff struct {
		X int `xml:"x,attr"`
		Y int `xml:"y,attr"`
	} `xml:"chOff"`
	ChExt struct {
		CX int `xml:"cx,attr"`
		CY int `xml:"cy,attr"`
	} `xml:"chExt"`
}

// UnmarshalXML decodes a shape tree while remembering the order of its
// children. encoding/xml would otherwise sort them into per-type slices and
// lose paint order, which is the one thing artwork extraction cannot guess.
func (t *rawShapeTree) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "sp", "pic", "graphicFrame":
				var shape rawShape
				if err := decoder.DecodeElement(&shape, &element); err != nil {
					return err
				}
				switch element.Name.Local {
				case "sp":
					t.Shapes = append(t.Shapes, shape)
				case "pic":
					t.Pictures = append(t.Pictures, shape)
				default:
					t.Frames = append(t.Frames, shape)
				}
				t.Children = append(t.Children, rawTreeChild{Kind: element.Name.Local, Shape: shape})
			case "grpSp":
				var group rawShapeTree
				if err := decoder.DecodeElement(&group, &element); err != nil {
					return err
				}
				t.Groups = append(t.Groups, group)
				nested := group
				t.Children = append(t.Children, rawTreeChild{Kind: "grpSp", Group: &nested})
			case "grpSpPr":
				var properties struct {
					Xfrm *rawGroupXfrm `xml:"xfrm"`
				}
				if err := decoder.DecodeElement(&properties, &element); err != nil {
					return err
				}
				t.Transform = properties.Xfrm
			default:
				if err := decoder.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if element.Name.Local == start.Name.Local {
				return nil
			}
		}
	}
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

// placedShape is a shape with where it sits, so a reader can take the slide in
// the order a person reads it rather than the order the file happens to store
// it in.
type placedShape struct {
	Shape rawShape
	Top   int
	Left  int
	// Width and Height are how much of the slide the shape covers, which is what
	// tells a column of a design apart from a box that spans the page. Zero
	// means the file did not say.
	Width  int
	Height int
}

// placed flattens the tree and says where each shape is. A shape inside a group
// is placed at its group, because a group is one thing on the page and its
// children keep the order they were written in.
func (t rawShapeTree) placed() []placedShape {
	result := make([]placedShape, 0, len(t.Shapes)+len(t.Pictures)+len(t.Frames))
	for _, kind := range [][]rawShape{t.Shapes, t.Pictures, t.Frames} {
		for _, shape := range kind {
			top, left := shape.position()
			width, height := shape.extent()
			result = append(result, placedShape{Shape: shape, Top: top, Left: left, Width: width, Height: height})
		}
	}
	for _, group := range t.Groups {
		top, left := 0, 0
		if group.Transform != nil {
			top, left = group.Transform.Off.Y, group.Transform.Off.X
		}
		width, height := 0, 0
		if group.Transform != nil {
			width, height = group.Transform.Ext.CX, group.Transform.Ext.CY
		}
		for _, child := range group.placed() {
			result = append(result, placedShape{Shape: child.Shape, Top: top, Left: left, Width: width, Height: height})
		}
	}
	return result
}

// position is where a shape sits, in EMU. A shape with no transform of its own
// takes its place from the layout, which readers of this cannot see: it sorts
// first, keeping the order it was written in.
func (s rawShape) position() (top, left int) {
	if s.Xfrm != nil {
		return s.Xfrm.Off.Y, s.Xfrm.Off.X
	}
	if s.SpPr != nil && s.SpPr.Xfrm != nil {
		return s.SpPr.Xfrm.Off.Y, s.SpPr.Xfrm.Off.X
	}
	return 0, 0
}

// extent is how big a shape is, in EMU. Zero means the file did not say, which
// is what a shape taking its size from the layout looks like from here.
func (s rawShape) extent() (width, height int) {
	if s.Xfrm != nil {
		return s.Xfrm.Ext.CX, s.Xfrm.Ext.CY
	}
	if s.SpPr != nil && s.SpPr.Xfrm != nil {
		return s.SpPr.Xfrm.Ext.CX, s.SpPr.Xfrm.Ext.CY
	}
	return 0, 0
}

type rawShape struct {
	NvSpPr           *rawNonVisual `xml:"nvSpPr"`
	NvPicPr          *rawNonVisual `xml:"nvPicPr"`
	NvGraphicFramePr *rawNonVisual `xml:"nvGraphicFramePr"`
	SpPr             *rawShapeProp `xml:"spPr"`
	// BlipFill sits beside spPr on a picture rather than inside it, which is
	// where every real template's photographs and logos are.
	BlipFill *rawBlipFill `xml:"blipFill"`
	Xfrm     *rawXfrm     `xml:"xfrm"`
	TxBody   *rawTxBody   `xml:"txBody"`
}

// picture returns the shape's blip fill, wherever it is recorded.
func (s rawShape) picture() *rawBlipFill {
	if s.BlipFill != nil {
		return s.BlipFill
	}
	if s.SpPr != nil {
		return s.SpPr.BlipFill
	}
	return nil
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
	GradFill  *rawGradFill  `xml:"gradFill"`
	BlipFill  *rawBlipFill  `xml:"blipFill"`
	NoFill    *struct{}     `xml:"noFill"`
	Line      *rawLine      `xml:"ln"`
	PrstGeom  *struct {
		Prst string `xml:"prst,attr"`
	} `xml:"prstGeom"`
	CustGeom *struct{} `xml:"custGeom"`
}

// rawGradFill is a linear gradient reduced to its stops and angle. Radial and
// path gradients are approximated by their first and last stop, which reads far
// closer to the template than a flat fill would.
type rawGradFill struct {
	GsLst struct {
		Gs []struct {
			Pos       int `xml:"pos,attr"`
			SchemeClr *struct {
				Val   string `xml:"val,attr"`
				Alpha *struct {
					Val int `xml:"val,attr"`
				} `xml:"alpha"`
				LumMod *struct {
					Val int `xml:"val,attr"`
				} `xml:"lumMod"`
				LumOff *struct {
					Val int `xml:"val,attr"`
				} `xml:"lumOff"`
				Shade *struct {
					Val int `xml:"val,attr"`
				} `xml:"shade"`
				Tint *struct {
					Val int `xml:"val,attr"`
				} `xml:"tint"`
			} `xml:"schemeClr"`
			SrgbClr *struct {
				Val   string `xml:"val,attr"`
				Alpha *struct {
					Val int `xml:"val,attr"`
				} `xml:"alpha"`
			} `xml:"srgbClr"`
		} `xml:"gs"`
	} `xml:"gsLst"`
	Lin *struct {
		Ang int `xml:"ang,attr"`
	} `xml:"lin"`
}

// rawBlipFill points at an image part through a relationship id.
type rawBlipFill struct {
	Blip struct {
		Embed string `xml:"embed,attr"`
		Link  string `xml:"link,attr"`
		Alpha *struct {
			Val int `xml:"val,attr"`
		} `xml:"alphaModFix"`
	} `xml:"blip"`
	SrcRect *struct {
		L int `xml:"l,attr"`
		T int `xml:"t,attr"`
		R int `xml:"r,attr"`
		B int `xml:"b,attr"`
	} `xml:"srcRect"`
	Stretch *struct{} `xml:"stretch"`
	Tile    *struct{} `xml:"tile"`
}

type rawLine struct {
	Width     int           `xml:"w,attr"`
	SolidFill *rawSolidFill `xml:"solidFill"`
	NoFill    *struct{}     `xml:"noFill"`
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
	Rotation int    `xml:"rot,attr"`
	FlipH    string `xml:"flipH,attr"`
	FlipV    string `xml:"flipV,attr"`
	Off      struct {
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
		Vert   string `xml:"vert,attr"`
		Anchor string `xml:"anchor,attr"`
		LIns   string `xml:"lIns,attr"`
	} `xml:"bodyPr"`
	LstStyle rawTextStyle   `xml:"lstStyle"`
	Para     []rawParagraph `xml:"p"`
}

type rawParagraph struct {
	PPr struct {
		Align string `xml:"algn,attr"`
		// Level is the outline depth of a bullet, which a deck being read back in
		// has to keep: a sub-point promoted to a point changes the argument.
		Level int `xml:"lvl,attr"`
	} `xml:"pPr"`
	Runs []struct {
		RPr struct {
			Size      int           `xml:"sz,attr"`
			Bold      string        `xml:"b,attr"`
			Italic    string        `xml:"i,attr"`
			// A rule through a line means the author cancelled it, and this deck
			// has no mark to carry that. Read it anyway: the words come across
			// looking live, and the person who wrote them has to be told which
			// ones to strike again.
			Strike    string        `xml:"strike,attr"`
			Underline string        `xml:"u,attr"`
			SolidFill *rawSolidFill `xml:"solidFill"`
			Latin     struct {
				Typeface string `xml:"typeface,attr"`
			} `xml:"latin"`
			// A link lives in the run that carries it, as a relationship id the
			// slide's own rels resolve. Reading the text and not this is how an
			// imported deck came back with the words of a link and no address.
			HlinkClick *struct {
				ID string `xml:"id,attr"`
			} `xml:"hlinkClick"`
		} `xml:"rPr"`
		Text string `xml:"t"`
	} `xml:"r"`
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
	// Align is the paragraph's own alignment. A template that centres its cover
	// says so here, and a preview that ignores it draws a different slide from
	// the one PowerPoint will.
	Align  string `xml:"algn,attr"`
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

func (l rawLevelStyle) italic() bool { return l.DefRPr.Italic == "1" || l.DefRPr.Italic == "true" }

// align is the paragraph alignment, in DrawingML's own vocabulary.
func (l rawLevelStyle) align() string {
	switch strings.TrimSpace(l.Align) {
	case "l", "ctr", "r", "just":
		// "l" counts: a layout that sets its title left has to override a master
		// that centres everything, and treating left as "unset" lets the master win.
		return strings.TrimSpace(l.Align)
	}
	return ""
}

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

// transform returns the shape's own transform, wherever it is recorded.
func (s rawShape) transform() *rawXfrm {
	if s.SpPr != nil && s.SpPr.Xfrm != nil {
		return s.SpPr.Xfrm
	}
	return s.Xfrm
}

func (s rawShape) geometry() (x, y, width, height int, ok bool) {
	transform := s.transform()
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
		GradFill  *rawGradFill  `xml:"gradFill"`
		BlipFill  *rawBlipFill  `xml:"blipFill"`
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
		Val   string    `xml:"val,attr"`
		Alpha *rawAlpha `xml:"alpha"`
	} `xml:"schemeClr"`
	SrgbClr *struct {
		Val   string    `xml:"val,attr"`
		Alpha *rawAlpha `xml:"alpha"`
	} `xml:"srgbClr"`
	SysClr *struct {
		LastClr string `xml:"lastClr,attr"`
	} `xml:"sysClr"`
}

// rawAlpha is DrawingML's opacity modifier, in thousandths of a percent.
type rawAlpha struct {
	Val int `xml:"val,attr"`
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
