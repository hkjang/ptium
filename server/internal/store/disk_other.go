//go:build !linux && !darwin

package store

// diskRoom cannot be read on this platform, and says so by answering nothing.
func diskRoom(string) (total, free int64) { return 0, 0 }
