package pptx

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // registers the GIF decoder for image.Decode
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"sync"
)

// A preview embeds its pictures as data URIs. An SVG loaded through <img> is in
// secure static mode and will not fetch anything, so a reference to an API URL
// would silently render as nothing.
//
// Embedding raw template images would be far too heavy — a full-bleed
// photograph is routinely a megabyte — so each one is decoded, scaled to the
// size the preview actually paints it at, and re-encoded.
const (
	// previewImagePixels bounds the longest edge of an embedded picture when
	// nothing says how large it will be painted. It is a ceiling, never a
	// target: a picture is embedded at the size it is drawn at, and this is only
	// what a full-bleed one on a full-size preview needs.
	previewImagePixels = 1400
	// pictureRetina embeds twice what a box measures, so a preview stays sharp
	// on a screen that paints two device pixels per CSS pixel. It is the right
	// number for a screen and the wrong one for a page: see PictureDensity.
	pictureRetina = 2
	// PrintPictureDensity is what a picture drawn into a PDF deserves. The PDF
	// is drawn in points, and a point is a 72nd of an inch, so four pixels to
	// the point is a shade under 300dpi — what a printed page can resolve.
	PrintPictureDensity = 4
	// pictureBucket rounds a budget up so that boxes of near-enough the same
	// size share one cached encoding rather than each holding its own.
	pictureBucket = 128
	// previewJPEGQuality trades a little fidelity for a much smaller preview.
	previewJPEGQuality = 76
	// maximumMediaBytes refuses to decode an implausibly large image part.
	maximumMediaBytes = 24 << 20
)

// pictureBox is the box a picture is painted into, in the preview's own CSS
// pixels — the units the renderer emits coordinates in.
//
// It exists because the size of the drawing and the size of the picture inside
// it are not the same question, and treating them as one made every thumbnail
// cost what a full-size drawing costs. A 320px overview thumbnail of a slide
// carrying a 2.4MB photograph was 968KB: the canvas honoured the width asked
// for, and the photograph inside it was embedded at 1400px all the same.
type pictureBox struct {
	// Width and Height are the box in preview pixels. A zero box means nothing
	// is known about how large this will be drawn, and the ceiling applies.
	Width, Height float64
	// Cover is true when the picture is cropped to fill the box rather than
	// fitted inside it. A cropped picture is scaled up until it covers, so the
	// part that shows is drawn from fewer of its own pixels than a fitted one —
	// which is exactly the case a budget taken from the box alone gets wrong.
	Cover bool
	// Density is how many of the picture's own pixels each drawn unit deserves.
	// Zero reads as a screen's two. A page asks for more, because a drawn unit
	// there is a point rather than a pixel and a printer resolves far more of
	// them — embedding a screen's worth in an exported PDF loses detail nobody
	// asked to lose.
	Density int
}

// budgetFor is the longest edge worth embedding for a picture of these bounds
// drawn into this box.
//
// What has to stay sharp is the part that shows, which is the box. So the
// budget is the picture's longest edge measured at the scale it will be drawn
// at, doubled for a dense screen — and never more than the ceiling, so this can
// only ever ask for less than before.
func (b pictureBox) budgetFor(bounds image.Rectangle) int {
	width, height := float64(bounds.Dx()), float64(bounds.Dy())
	if b.Width <= 0 || b.Height <= 0 || width <= 0 || height <= 0 {
		return previewImagePixels
	}
	// How far the picture is scaled to sit in the box: to cover it, the larger
	// of the two ratios; to fit inside it, the smaller.
	scale := math.Min(b.Width/width, b.Height/height)
	if b.Cover {
		scale = math.Max(b.Width/width, b.Height/height)
	}
	density := b.Density
	if density <= 0 {
		density = pictureRetina
	}
	wanted := int(math.Ceil(math.Max(width, height) * scale * float64(density)))
	// Rounded up to a bucket, so a rail of thumbnails a few pixels apart shares
	// one encoding of each picture instead of one apiece.
	wanted = ((wanted + pictureBucket - 1) / pictureBucket) * pictureBucket
	if wanted < pictureBucket {
		wanted = pictureBucket
	}
	if wanted > previewImagePixels {
		wanted = previewImagePixels
	}
	return wanted
}

// MediaResolver returns a data URI for a package part, or an empty string when
// the part cannot be drawn.
type MediaResolver func(part string) string

// PackageMedia builds a resolver over a template package. Results are cached by
// content, so a gallery of previews over the same template pays for each image
// once.
func PackageMedia(pkg *Package, maxPixels int) MediaResolver {
	if pkg == nil {
		return func(string) string { return "" }
	}
	if maxPixels <= 0 {
		maxPixels = previewImagePixels
	}
	return func(part string) string {
		data, ok := pkg.Part(part)
		if !ok || len(data) == 0 || len(data) > maximumMediaBytes {
			return ""
		}
		return mediaDataURI(part, data, pictureBox{}, maxPixels)
	}
}

type mediaCacheEntry struct {
	uri  string
	size int
}

var mediaCache = struct {
	sync.Mutex
	entries map[string]mediaCacheEntry
	bytes   int
}{entries: map[string]mediaCacheEntry{}}

// maximumMediaCacheBytes bounds what the encoded-image cache may hold. Previews
// are cheap to regenerate, so the cache is simply emptied when it grows past
// this rather than tracking access order.
const maximumMediaCacheBytes = 64 << 20

// mediaDataURI encodes a picture for a preview, at the size it will be drawn.
//
// The box is what the renderer is about to paint into; ceiling is the most this
// caller will ever embed, for the callers that know no box. The cache is keyed
// on the box rather than on the budget the box works out to, because the budget
// is only known once the picture has been decoded — and decoding it is the
// thing the cache exists to avoid.
func mediaDataURI(part string, data []byte, box pictureBox, ceiling int) string {
	if ceiling <= 0 {
		ceiling = previewImagePixels
	}
	digest := sha256.Sum256(data)
	key := fmt.Sprintf("%s|%d|%d|%d|%t|%d", hex.EncodeToString(digest[:16]), ceiling,
		int(math.Ceil(box.Width)), int(math.Ceil(box.Height)), box.Cover, box.Density)

	mediaCache.Lock()
	if entry, ok := mediaCache.entries[key]; ok {
		mediaCache.Unlock()
		return entry.uri
	}
	mediaCache.Unlock()

	uri := encodeMedia(part, data, box, ceiling)

	mediaCache.Lock()
	defer mediaCache.Unlock()
	if mediaCache.bytes+len(uri) > maximumMediaCacheBytes {
		mediaCache.entries = map[string]mediaCacheEntry{}
		mediaCache.bytes = 0
	}
	mediaCache.entries[key] = mediaCacheEntry{uri: uri, size: len(uri)}
	mediaCache.bytes += len(uri)
	return uri
}

func encodeMedia(part string, data []byte, box pictureBox, ceiling int) string {
	// SVG needs no decoding: a browser draws it directly, and it is usually the
	// smallest form of a logo anyway.
	if strings.HasSuffix(strings.ToLower(part), ".svg") {
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data)
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// EMF, WMF and TIFF are common in older templates and cannot be decoded
		// here. Drawing nothing is better than drawing a broken image.
		return ""
	}
	// The budget needs the picture's own bounds, so it is worked out here rather
	// than by the caller: how large this picture is drawn depends on both.
	maxPixels := box.budgetFor(decoded.Bounds())
	if maxPixels > ceiling {
		maxPixels = ceiling
	}
	scaled := downscale(decoded, maxPixels)
	// A JPEG cannot carry transparency, and a logo that loses its transparency
	// paints a white box over the design.
	if hasTransparency(scaled) {
		var buffer bytes.Buffer
		if png.Encode(&buffer, scaled) != nil {
			return ""
		}
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
	}
	var buffer bytes.Buffer
	if jpeg.Encode(&buffer, scaled, &jpeg.Options{Quality: previewJPEGQuality}) != nil {
		return ""
	}
	// An already-small original may still be the better choice.
	if format == "jpeg" && len(data) <= buffer.Len() && sameBounds(decoded, scaled) {
		return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func sameBounds(a, b image.Image) bool {
	return a.Bounds().Dx() == b.Bounds().Dx() && a.Bounds().Dy() == b.Bounds().Dy()
}

// downscale reduces an image to fit maxPixels on its longest edge, averaging
// over each source region so the result stays smooth rather than aliased.
func downscale(source image.Image, maxPixels int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return source
	}
	longest := max(width, height)
	if longest <= maxPixels {
		return source
	}
	ratio := float64(maxPixels) / float64(longest)
	targetWidth := max(1, int(math.Round(float64(width)*ratio)))
	targetHeight := max(1, int(math.Round(float64(height)*ratio)))
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceTop := bounds.Min.Y + y*height/targetHeight
		sourceBottom := bounds.Min.Y + (y+1)*height/targetHeight
		if sourceBottom <= sourceTop {
			sourceBottom = sourceTop + 1
		}
		for x := 0; x < targetWidth; x++ {
			sourceLeft := bounds.Min.X + x*width/targetWidth
			sourceRight := bounds.Min.X + (x+1)*width/targetWidth
			if sourceRight <= sourceLeft {
				sourceRight = sourceLeft + 1
			}
			var red, green, blue, alpha, count uint64
			for sampleY := sourceTop; sampleY < sourceBottom; sampleY++ {
				for sampleX := sourceLeft; sampleX < sourceRight; sampleX++ {
					r, g, b, a := source.At(sampleX, sampleY).RGBA()
					red += uint64(r)
					green += uint64(g)
					blue += uint64(b)
					alpha += uint64(a)
					count++
				}
			}
			if count == 0 {
				continue
			}
			target.SetNRGBA(x, y, color.NRGBA{
				R: uint8(red / count >> 8), G: uint8(green / count >> 8),
				B: uint8(blue / count >> 8), A: uint8(alpha / count >> 8),
			})
		}
	}
	return target
}

func hasTransparency(source image.Image) bool {
	switch source.ColorModel() {
	case color.YCbCrModel, color.CMYKModel, color.GrayModel, color.Gray16Model:
		return false
	}
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, alpha := source.At(x, y).RGBA(); alpha < 0xffff {
				return true
			}
		}
	}
	return false
}

// describePictures fills in each picture's mean colour, which is what decides
// whether text placed over it should be light or dark. It happens once, during
// analysis, so no preview or export pays for it.
func describePictures(pkg *Package, artwork []Artwork) {
	if pkg == nil {
		return
	}
	for index := range artwork {
		piece := &artwork[index]
		if piece.Kind != "picture" || piece.Image == "" || piece.Average != "" {
			continue
		}
		data, ok := pkg.Part(piece.Image)
		if !ok || len(data) == 0 || len(data) > maximumMediaBytes {
			continue
		}
		if average, found := averageColor(data); found {
			piece.Average = average
		}
	}
}

// averageCache remembers a picture's mean colour by content. A master's
// background is inherited by every layout, and decoding it once per layout is
// wasted work on every template analysis.
var averageCache = struct {
	sync.Mutex
	entries map[string]string
}{entries: map[string]string{}}

// maximumAverageCacheEntries bounds the cache; a template has a handful of
// distinct images, so this is only a backstop.
const maximumAverageCacheEntries = 512

// averageColor is the mean of an image, computed over a coarse sample rather
// than every pixel: a full-bleed photograph has millions, and a hundredth of
// them describes its tone just as well.
func averageColor(data []byte) (string, bool) {
	digest := sha256.Sum256(data)
	key := hex.EncodeToString(digest[:16])
	averageCache.Lock()
	cached, ok := averageCache.entries[key]
	averageCache.Unlock()
	if ok {
		return cached, cached != ""
	}
	value, found := computeAverageColor(data)
	averageCache.Lock()
	if len(averageCache.entries) >= maximumAverageCacheEntries {
		averageCache.entries = map[string]string{}
	}
	averageCache.entries[key] = value
	averageCache.Unlock()
	return value, found
}

func computeAverageColor(data []byte) (string, bool) {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", false
	}
	bounds := decoded.Bounds()
	if bounds.Empty() {
		return "", false
	}
	stepX := max(1, bounds.Dx()/48)
	stepY := max(1, bounds.Dy()/48)
	var red, green, blue, weight uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r, g, b, a := decoded.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			// Weight by coverage so a mostly transparent logo does not drag the
			// average toward its few opaque pixels.
			alpha := uint64(a >> 8)
			red += uint64(r>>8) * alpha
			green += uint64(g>>8) * alpha
			blue += uint64(b>>8) * alpha
			weight += alpha
		}
	}
	if weight == 0 {
		return "", false
	}
	return fmt.Sprintf("%02X%02X%02X", red/weight, green/weight, blue/weight), true
}
