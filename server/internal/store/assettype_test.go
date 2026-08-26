package store

import "testing"

// What an upload is called decides nothing about what it is.
//
// A browser sends "image/png" for whatever the person picked, and trusting that
// let a PDF, a web page and a line of text into the picture library under a
// .png name. A picture that is not a picture is found out later — by PowerPoint,
// which tells the person their exported deck is damaged.
func TestWhatAFileIsComesFromItsBytes(t *testing.T) {
	images := map[string][]byte{
		"image/png":     append([]byte{0x89}, []byte("PNG\r\n\x1a\n....")...),
		"image/jpeg":    {0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0},
		"image/gif":     []byte("GIF89a........"),
		"image/svg+xml": []byte(`<?xml version="1.0"?><!-- drawn by hand --><svg xmlns="http://www.w3.org/2000/svg"/>`),
	}
	for want, data := range images {
		if got := detectAssetType("application/octet-stream", data); got != want {
			t.Errorf("detectAssetType(%q…) = %q, want %q", data[:min(6, len(data))], got, want)
		}
	}
	// Everything else is refused however it is labelled.
	for what, data := range map[string][]byte{
		"a line of text": []byte("nope not an image at all"),
		"a web page":     []byte("<!doctype html><html><body>hi</body></html>"),
		"a PDF":          []byte("%PDF-1.7\n1 0 obj\n<</Type /Catalog>>"),
		"a zip":          []byte("PK\x03\x04 an ordinary zip"),
		"nothing":        {},
	} {
		if got := detectAssetType("image/png", data); got != "" {
			t.Errorf("%s was taken for %q", what, got)
		}
	}
}
