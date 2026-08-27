package pptx

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"testing"
)

// A photograph, in the sense that matters here: large, and not compressible
// down to nothing.
func photograph(width, height int) []byte {
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	source := rand.New(rand.NewSource(7))
	for y := 0; y < height; y += 4 {
		for x := 0; x < width; x += 4 {
			shade := color.RGBA{uint8(source.Intn(256)), uint8(source.Intn(256)), uint8(source.Intn(256)), 255}
			for dy := 0; dy < 4 && y+dy < height; dy++ {
				for dx := 0; dx < 4 && x+dx < width; dx++ {
					picture.Set(x+dx, y+dy, shade)
				}
			}
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, picture, &jpeg.Options{Quality: 88}); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}

// The one that was wrong: a preview honoured the width it was asked for, and
// the picture inside it was embedded at a fixed 1400px however small that width
// was. A 320px overview thumbnail of a slide carrying a 2.4MB photograph came
// to 968KB — the same as the full-screen drawing.
func TestAPictureIsEmbeddedAtTheSizeItIsDrawn(t *testing.T) {
	bounds := image.Rect(0, 0, 2400, 1600)
	thumbnail := pictureBox{Width: 320, Height: 180, Cover: true}.budgetFor(bounds)
	full := pictureBox{Width: 1600, Height: 900, Cover: true}.budgetFor(bounds)
	if thumbnail >= full {
		t.Fatalf("a thumbnail asked for %d pixels and a full drawing for %d: the thumbnail must ask for less", thumbnail, full)
	}
	if thumbnail > 768 {
		t.Fatalf("a 320px box asked for %d pixels; twice its own size and a little rounding is all it can use", thumbnail)
	}
}

func TestAPictureBudgetNeverExceedsTheCeiling(t *testing.T) {
	bounds := image.Rect(0, 0, 8000, 6000)
	for _, box := range []pictureBox{
		{Width: 1600, Height: 900, Cover: true},
		{Width: 4000, Height: 3000, Cover: true},
		{Width: 320, Height: 4000, Cover: true},
	} {
		if got := box.budgetFor(bounds); got > previewImagePixels {
			t.Fatalf("box %v asked for %d pixels, above the %d ceiling", box, got, previewImagePixels)
		}
	}
}

// Knowing nothing about the box is not a licence to guess small: a caller that
// cannot say how large a picture will be drawn gets what it always got.
func TestAnUnknownBoxKeepsTheCeiling(t *testing.T) {
	if got := (pictureBox{}).budgetFor(image.Rect(0, 0, 2400, 1600)); got != previewImagePixels {
		t.Fatalf("an unknown box asked for %d pixels, want the %d ceiling", got, previewImagePixels)
	}
}

// A cropped picture is scaled up until it covers its box, so the part that
// shows is drawn from fewer of the picture's own pixels than a fitted one —
// and a budget that ignored the crop would leave it soft.
func TestCroppingAsksForMoreThanFitting(t *testing.T) {
	bounds := image.Rect(0, 0, 400, 3000) // tall, in a wide box
	box := pictureBox{Width: 640, Height: 360}
	fitted := box.budgetFor(bounds)
	box.Cover = true
	cropped := box.budgetFor(bounds)
	if cropped <= fitted {
		t.Fatalf("cropping asked for %d pixels and fitting for %d: cropping needs more", cropped, fitted)
	}
}

// The measurement that found it, in miniature: the same photograph encoded for
// a thumbnail and for a full-screen drawing.
func TestEncodingShrinksWithTheBox(t *testing.T) {
	data := photograph(2400, 1600)
	thumbnail := encodeMedia("photo.jpg", data, pictureBox{Width: 320, Height: 180, Cover: true}, previewImagePixels)
	full := encodeMedia("photo.jpg", data, pictureBox{Width: 1600, Height: 900, Cover: true}, previewImagePixels)
	if thumbnail == "" || full == "" {
		t.Fatal("the photograph did not encode")
	}
	if len(thumbnail)*4 > len(full) {
		t.Fatalf("a thumbnail came to %d bytes against a full drawing's %d: it must cost a fraction, not the same", len(thumbnail), len(full))
	}
}
