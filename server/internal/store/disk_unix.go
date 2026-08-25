//go:build linux || darwin

package store

import "syscall"

// diskRoom is how big the filesystem holding a path is and how much of it is
// free, in bytes. Zero for both when the path cannot be read: a number nobody
// can trust is worse than no number, and the screen leaves it out.
//
// The standard library is enough for this, and an air-gapped build is one
// dependency lighter for it.
func diskRoom(path string) (total, free int64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	return int64(stat.Blocks) * int64(stat.Bsize), int64(stat.Bavail) * int64(stat.Bsize)
}
