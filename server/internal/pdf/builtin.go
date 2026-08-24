package pdf

import (
	_ "embed"
	"sync"
)

// The font a Ptium PDF is set in.
//
// A PDF has no Korean face of its own to fall back on — the fourteen fonts
// every reader is required to have are Latin — so a deck written in Korean can
// only be put on paper by carrying a face that has Hangul in it. This one is
// NanumBarunGothic, under the SIL Open Font License, which is what lets it be
// shipped inside an air-gapped image.
//
// It is not the deck's own font: a PowerPoint template names its typeface and
// does not carry it, so nothing in a deck says what 맑은 고딕 looks like. The
// exported pptx is where the deck's own design lives; the PDF is where it can
// be read anywhere.
//
//go:embed fonts/NanumBarunGothic.ttf
var builtinFont []byte

//go:embed fonts/LICENSE-NanumBarunGothic.txt
var builtinFontLicense string

var (
	loadOnce   sync.Once
	loaded     *TrueType
	loadFailed error
)

// BuiltinFont is the embedded face, parsed once.
func BuiltinFont() (*TrueType, error) {
	loadOnce.Do(func() { loaded, loadFailed = ParseTrueType(builtinFont) })
	return loaded, loadFailed
}

// FontLicense is the licence the embedded face is shipped under. It is served
// with the workspace's own licences rather than only living in the repository:
// a font inside a product is redistributed by whoever deploys it.
func FontLicense() string { return builtinFontLicense }
