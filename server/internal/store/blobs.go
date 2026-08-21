package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// BlobStore keeps the bytes of an uploaded image outside the database.
//
// Images are the only thing a deployment stores that grows without bound: a
// hundred slide photographs is a database an operator has to plan backups
// around. Putting them on a volume keeps the database to rows a person could
// read, and lets a Kubernetes deployment point a PersistentVolumeClaim at them.
//
// Nothing is required to use one. Without a blob store the bytes stay in the
// assets table exactly as before, which is the right answer for a single-node
// install that would rather back up one thing.
type BlobStore interface {
	// Put writes the bytes for an asset id, replacing whatever was there.
	Put(id string, data []byte) error
	// Get reads the bytes. It returns ErrBlobMissing when the id has no file,
	// which is not an error the caller has to treat as failure: the bytes may
	// still be in the database row from before the volume existed.
	Get(id string) ([]byte, error)
	// Delete removes the bytes. Removing what is not there is not an error.
	Delete(id string) error
	// Describe names the store for logs and for the readiness message.
	Describe() string
}

// ErrBlobMissing says the store has no bytes for that id.
var ErrBlobMissing = errors.New("no stored bytes for this asset")

// FileBlobs keeps each image as one file under a directory, which is what a
// PersistentVolumeClaim, an NFS mount or a plain disk all look like.
type FileBlobs struct {
	root string
}

// OpenFileBlobs prepares a directory to hold images, failing immediately if it
// cannot be written to.
//
// Failing at startup is deliberate. A volume that is missing, read-only or owned
// by another user is a deployment mistake, and finding out when the first person
// uploads a logo is finding out too late.
func OpenFileBlobs(root string) (*FileBlobs, error) {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return nil, errors.New("an image directory is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create image directory %s: %w", root, err)
	}
	probe := filepath.Join(root, ".writable")
	if err := os.WriteFile(probe, []byte("ptium"), 0o640); err != nil {
		return nil, fmt.Errorf("image directory %s is not writable: %w", root, err)
	}
	if err := os.Remove(probe); err != nil {
		return nil, fmt.Errorf("image directory %s is not writable: %w", root, err)
	}
	return &FileBlobs{root: root}, nil
}

func (f *FileBlobs) Describe() string { return "directory " + f.root }

// path is where one asset's bytes live. The id is a UUID and is checked as one:
// a path is built from it, and nothing that is not a UUID may build a path.
func (f *FileBlobs) path(id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", fmt.Errorf("asset id %q is not a UUID", id)
	}
	name := parsed.String()
	// One level of fan-out. A flat directory of a hundred thousand files is slow
	// to list on every filesystem that people actually mount.
	return filepath.Join(f.root, name[:2], name), nil
}

func (f *FileBlobs) Put(id string, data []byte) error {
	target, err := f.path(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	// Written beside the target and renamed over it, so a reader never sees half
	// an image and a crash never leaves one.
	temporary, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporary.Name(), 0o640); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), target)
}

func (f *FileBlobs) Get(id string) ([]byte, error) {
	target, err := f.path(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrBlobMissing
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (f *FileBlobs) Delete(id string) error {
	target, err := f.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
