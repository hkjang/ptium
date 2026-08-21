package store

import (
	"errors"
	"os"
	"path/filepath"
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
