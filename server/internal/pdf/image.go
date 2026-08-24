package pdf

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// jpegImage carries JPEG bytes into the file as they are. A PDF reads the same
// encoding, so a photograph is not decoded and re-encoded on its way to paper.
func jpegImage(data []byte) *Image {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return &Image{Width: config.Width, Height: config.Height, Data: data,
		ColorSpace: "DeviceRGB", Filter: "DCTDecode", Bits: 8}
}

// pngImage decodes to samples, because PDF's own Flate images are not PNG: the
// predictors and the palette are the format's, not the picture's. Transparency
// is carried as the soft mask a PDF uses for it, so a logo cut out of its
// background stays cut out.
func pngImage(data []byte) *Image {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	rgba := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
	samples := make([]byte, 0, width*height*3)
	alpha := make([]byte, 0, width*height)
	opaque := true
	for y := range height {
		for x := range width {
			at := rgba.PixOffset(x, y)
			samples = append(samples, rgba.Pix[at], rgba.Pix[at+1], rgba.Pix[at+2])
			alpha = append(alpha, rgba.Pix[at+3])
			if rgba.Pix[at+3] != 0xFF {
				opaque = false
			}
		}
	}
	picture := &Image{Width: width, Height: height, Data: deflate(samples),
		ColorSpace: "DeviceRGB", Filter: "FlateDecode", Bits: 8}
	if !opaque {
		picture.Alpha = alpha
	}
	return picture
}
