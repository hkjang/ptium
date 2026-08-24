package pdf

import (
	"os"
	"testing"
)

// A sample written where a real reader can be pointed at it. Skipped unless
// PTIUM_PDF_SAMPLE says where to put it.
func TestWriteSample(t *testing.T) {
	path := os.Getenv("PTIUM_PDF_SAMPLE")
	if path == "" {
		t.Skip("set PTIUM_PDF_SAMPLE to write a sample PDF")
	}
	font, err := BuiltinFont()
	if err != nil {
		t.Fatal(err)
	}
	document := New(720, 405, "분기 보고 · Q3", font)
	page := document.AddPage()
	page.Rect(0, 0, 720, 405, "FFFFFF")
	page.Rect(48, 40, 64, 4, "2563EB")
	page.Text(48, 96, 30, "15181D", "매출이 12% 늘었습니다", true, false)
	page.Text(48, 140, 15, "3C4250", "가정은 인건비 동결과 트래픽 20% 증가입니다", false, true)
	width := page.Text(48, 176, 15, "1155CC", "안내 문서", false, false)
	page.Underline(48, 176, width, "1155CC", 15)
	page.Link(48, 164, width, 18, "https://example.com/a", 0)
	page.Link(48, 200, 120, 18, "", 2)
	second := document.AddPage()
	second.Rect(0, 0, 720, 405, "FFFFFF")
	second.Text(48, 96, 24, "15181D", "부록: 산출 근거", true, false)
	if err := os.WriteFile(path, document.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
