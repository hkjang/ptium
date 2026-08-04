package pptx

import (
	"encoding/xml"
	"strings"
	"testing"
)

// layoutXML is the shape of a real branded layout: a full-bleed picture, a
// gradient panel, a grouped logo and a static footer.
const layoutXML = `<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
	xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
	xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
 <p:cSld name="Cover">
  <p:bg><p:bgPr><a:solidFill><a:srgbClr val="0B1B33"/></a:solidFill></p:bgPr></p:bg>
  <p:spTree>
   <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
   <p:grpSpPr/>
   <p:pic><p:nvPicPr><p:cNvPr id="2" name="backdrop"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr>
    <p:blipFill><a:blip r:embed="rId2"/><a:srcRect l="5000" t="0" r="5000" b="0"/><a:stretch/></p:blipFill>
    <p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="12192000" cy="6858000"/></a:xfrm>
     <a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>
   <p:sp><p:nvSpPr><p:cNvPr id="3" name="panel"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
    <p:spPr><a:xfrm rot="5400000"><a:off x="0" y="6000000"/><a:ext cx="12192000" cy="120000"/></a:xfrm>
     <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
     <a:gradFill><a:gsLst>
       <a:gs pos="0"><a:schemeClr val="accent1"/></a:gs>
       <a:gs pos="100000"><a:srgbClr val="00E0C6"><a:alpha val="60000"/></a:srgbClr></a:gs>
      </a:gsLst><a:lin ang="0"/></a:gradFill></p:spPr></p:sp>
   <p:grpSp><p:nvGrpSpPr><p:cNvPr id="4" name="logos"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
    <p:grpSpPr><a:xfrm><a:off x="600000" y="6200000"/><a:ext cx="2000000" cy="200000"/>
      <a:chOff x="0" y="0"/><a:chExt cx="1000000" cy="100000"/></a:xfrm></p:grpSpPr>
    <p:sp><p:nvSpPr><p:cNvPr id="5" name="mark"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
     <p:spPr><a:xfrm><a:off x="500000" y="50000"/><a:ext cx="250000" cy="25000"/></a:xfrm>
      <a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom>
      <a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill></p:spPr></p:sp></p:grpSp>
   <p:sp><p:nvSpPr><p:cNvPr id="6" name="footer text"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
    <p:spPr><a:xfrm><a:off x="400000" y="6500000"/><a:ext cx="3000000" cy="250000"/></a:xfrm>
     <a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>
    <p:txBody><a:bodyPr anchor="ctr"/><a:p><a:pPr algn="ctr"/>
      <a:r><a:rPr sz="1000" b="1"><a:solidFill><a:srgbClr val="EEEEEE"/></a:solidFill></a:rPr>
       <a:t>주최 · 주관</a:t></a:r></a:p></p:txBody></p:sp>
   <p:sp><p:nvSpPr><p:cNvPr id="7" name="Title 1"/><p:cNvSpPr/><p:nvPr><p:ph type="ctrTitle"/></p:nvPr></p:nvSpPr>
    <p:spPr><a:xfrm><a:off x="800000" y="2000000"/><a:ext cx="8000000" cy="1200000"/></a:xfrm></p:spPr></p:sp>
  </p:spTree>
 </p:cSld>
</p:sldLayout>`

func parseLayoutArtwork(t *testing.T) []Artwork {
	t.Helper()
	var parsed struct {
		CSld struct {
			Background *rawFillHolder `xml:"bg"`
			SpTree     rawShapeTree   `xml:"spTree"`
		} `xml:"cSld"`
	}
	if err := xml.Unmarshal([]byte(layoutXML), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ctx := artworkContext{
		colorMap:  map[string]string{"accent1": "accent1"},
		theme:     Theme{Colors: map[string]string{"accent1": "1E6FFF", "tx1": "111111"}},
		relations: map[string]string{"rId2": "ppt/media/image7.jpg"},
	}
	return collectArtwork(parsed.CSld.SpTree, ctx, nil)
}

func TestCollectArtworkKeepsTheDesignInPaintOrder(t *testing.T) {
	pieces := parseLayoutArtwork(t)
	if len(pieces) != 4 {
		for _, piece := range pieces {
			t.Logf("%+v", piece)
		}
		t.Fatalf("expected the backdrop, panel, grouped mark and footer, got %d pieces", len(pieces))
	}
	// Paint order is document order: the backdrop first, the footer last.
	if pieces[0].Kind != "picture" || pieces[0].Image != "ppt/media/image7.jpg" {
		t.Fatalf("first piece = %+v", pieces[0])
	}
	if pieces[0].Crop != [4]int{5000, 0, 5000, 0} {
		t.Fatalf("the source crop was lost: %v", pieces[0].Crop)
	}
	panel := pieces[1]
	if len(panel.Gradient) != 2 || panel.Gradient[0].Color != "1E6FFF" {
		t.Fatalf("gradient = %+v", panel.Gradient)
	}
	if panel.Gradient[1].Opacity == 0 || panel.Gradient[1].Opacity >= 1 {
		t.Fatalf("the second stop's alpha was lost: %+v", panel.Gradient[1])
	}
	if panel.Rotation != 5400000 {
		t.Fatalf("rotation = %d", panel.Rotation)
	}
	// A grouped shape is projected into slide coordinates: the group maps a
	// 1000000x100000 child space onto a 2000000x200000 frame at 600000,6200000.
	mark := pieces[2]
	if mark.Preset != "ellipse" || mark.Fill != "FFFFFF" {
		t.Fatalf("grouped mark = %+v", mark)
	}
	if mark.X != 600000+1000000 || mark.Y != 6200000+100000 {
		t.Fatalf("grouped mark sits at %d,%d; want %d,%d", mark.X, mark.Y, 1600000, 6300000)
	}
	if mark.Width != 500000 || mark.Height != 50000 {
		t.Fatalf("grouped mark is %dx%d; want 500000x50000", mark.Width, mark.Height)
	}
	footer := pieces[3]
	if footer.Kind != "text" || footer.Text != "주최 · 주관" {
		t.Fatalf("footer = %+v", footer)
	}
	if footer.Align != "ctr" || footer.Anchor != "ctr" || !footer.Bold || footer.Color != "EEEEEE" {
		t.Fatalf("footer styling = %+v", footer)
	}
	// The title placeholder is a writable region, not artwork.
	for _, piece := range pieces {
		if piece.X == 800000 && piece.Y == 2000000 {
			t.Fatal("a placeholder was captured as artwork")
		}
	}
}

func TestPreviewDrawsArtworkAndItsPictures(t *testing.T) {
	manifest := Manifest{Version: ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme: Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "000000", "accent1": "1E6FFF"}}}
	layout := Layout{ID: "cover", Name: "Cover", Background: "0B1B33",
		Fill:    Background{Gradient: []GradientStop{{Position: 0, Color: "0B1B33"}, {Position: 1, Color: "1E6FFF"}}, GradientAngle: 90},
		Artwork: parseLayoutArtwork(t)}
	requested := ""
	svg := PreviewSVG(manifest, layout, Slide{LayoutID: "cover"}, PreviewOptions{Width: 960,
		Media: func(part string) string { requested = part; return "data:image/jpeg;base64,AAAA" }})

	if requested != "ppt/media/image7.jpg" {
		t.Fatalf("the preview did not ask for the picture, requested %q", requested)
	}
	for _, want := range []string{"<image", "data:image/jpeg;base64,AAAA", "<linearGradient", "<ellipse", "주최 · 주관", "<clipPath"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("preview is missing %q:\n%s", want, truncateSVG(svg))
		}
	}
	// Without a resolver the preview still renders, just without photographs.
	plain := PreviewSVG(manifest, layout, Slide{LayoutID: "cover"}, PreviewOptions{Width: 960})
	if strings.Contains(plain, "<image") {
		t.Fatal("a preview without a media resolver must not emit an image element")
	}
}

func truncateSVG(value string) string {
	if len(value) > 1200 {
		return value[:1200] + "…"
	}
	return value
}
