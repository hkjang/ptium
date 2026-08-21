package store

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const sampleID = "3f2504e0-4f89-11d3-9a0c-0305e82c3301"

func TestFileBlobsRoundTripsAnImage(t *testing.T) {
	blobs, err := OpenFileBlobs(filepath.Join(t.TempDir(), "assets"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := blobs.Put(sampleID, []byte("PNG-ish")); err != nil {
		t.Fatalf("put: %v", err)
	}
	data, err := blobs.Get(sampleID)
	if err != nil || string(data) != "PNG-ish" {
		t.Fatalf("get = %q, %v", data, err)
	}
	// Re-uploading the same name replaces the bytes rather than appending.
	if err := blobs.Put(sampleID, []byte("newer")); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if data, _ := blobs.Get(sampleID); string(data) != "newer" {
		t.Fatalf("replaced bytes = %q", data)
	}
	if err := blobs.Delete(sampleID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := blobs.Get(sampleID); !errors.Is(err, ErrBlobMissing) {
		t.Fatalf("after delete, err = %v, want ErrBlobMissing", err)
	}
	// Deleting what is already gone is what a retried delete does.
	if err := blobs.Delete(sampleID); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

// The id becomes a path, so anything that is not an id must never become one.
func TestFileBlobsRefusesAnIdThatIsNotAUUID(t *testing.T) {
	root := t.TempDir()
	blobs, err := OpenFileBlobs(root)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, id := range []string{"../../etc/passwd", "..", "", "a/b", "3f2504e0-4f89-11d3-9a0c-0305e82c3301/../x"} {
		if err := blobs.Put(id, []byte("x")); err == nil {
			t.Fatalf("put(%q) was accepted", id)
		}
		if _, err := blobs.Get(id); err == nil {
			t.Fatalf("get(%q) was accepted", id)
		}
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("a rejected id still wrote something: %v", entries)
	}
}

// A read-only or missing volume is a deployment mistake, and the process should
// find out while it is starting rather than when someone uploads a logo.
func TestOpenFileBlobsRefusesADirectoryItCannotWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(root, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes to any directory")
	}
	if _, err := OpenFileBlobs(filepath.Join(root, "assets")); err == nil {
		t.Fatal("a directory that cannot be created was accepted")
	}
}

// Nothing is left behind when a write fails halfway, and no reader sees a
// half-written image.
func TestFileBlobsLeavesNoTemporaryFiles(t *testing.T) {
	root := t.TempDir()
	blobs, err := OpenFileBlobs(root)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := blobs.Put(sampleID, make([]byte, 4096)); err != nil {
		t.Fatalf("put: %v", err)
	}
	var files []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, filepath.Base(path))
		}
		return nil
	})
	if len(files) != 1 || files[0] != sampleID {
		t.Fatalf("files under the volume = %v", files)
	}
}

// Tags are how a library of two hundred pictures stays findable, so the list has
// to stay short, trimmed and free of repeats however it is typed.
func TestNormalizeTagsKeepsAListSomeoneCanScan(t *testing.T) {
	got := normalizeTags([]string{" 로고 ", "로고", "Logo", "logo", "", "   ", "제품컷", "배경", "a", "b", "c", "d", "e", "f"})
	if len(got) != 8 {
		t.Fatalf("tags = %v", got)
	}
	// Trimmed, in the order they were written, and never the same word twice.
	if got[0] != "로고" || got[1] != "Logo" || got[2] != "제품컷" {
		t.Fatalf("tags lost their order or their trimming: %v", got)
	}
	seen := map[string]bool{}
	for _, tag := range got {
		if seen[strings.ToLower(tag)] {
			t.Fatalf("the same word was kept twice in different case: %v", got)
		}
		seen[strings.ToLower(tag)] = true
	}
	// A tag nobody could read in a chip is not a tag.
	if got := normalizeTags([]string{strings.Repeat("가", 30), "짧음"}); len(got) != 1 || got[0] != "짧음" {
		t.Fatalf("an overlong tag was kept: %v", got)
	}
}

// The usage table is written from stored slide content, so what counts as "this
// deck places that image" is decided here.
func TestAssetsInContentFindsBothPlacedAndDrawnImages(t *testing.T) {
	content := []byte(`{
		"images": {"picture": {"assetId": "11111111-1111-1111-1111-111111111111", "name": "logo.png"},
		           "empty": {"name": "gone.png"}},
		"elements": [{"kind": "image", "assetId": "22222222-2222-2222-2222-222222222222"},
		             {"kind": "text", "text": "no image here"},
		             {"kind": "image", "assetId": "  "}]
	}`)
	got := assetsInContent(content)
	sort.Strings(got)
	want := []string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("assets = %v, want %v", got, want)
	}
	// Content written before images existed, and content that is not JSON at all,
	// must not fail a save.
	if got := assetsInContent([]byte(`{"fields":{}}`)); len(got) != 0 {
		t.Fatalf("old content produced %v", got)
	}
	if got := assetsInContent([]byte("not json")); len(got) != 0 {
		t.Fatalf("unreadable content produced %v", got)
	}
}
