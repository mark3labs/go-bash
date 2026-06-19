package main

import (
	"os"

	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/memfs"
)

// newMemFS returns an empty in-memory FileSystem. Split out from
// main.go so tests can override the base FS factory if a future
// change wants to pre-seed the base.
func newMemFS() gbfs.FileSystem { return memfs.New() }

// isCharDevice reports whether f is connected to a character device
// (typically a terminal). Used by main() to decide whether to read
// the script from stdin or print help when no -c / file is supplied.
// The check uses os.Stat's mode bits; a closed or pipe stdin returns
// false.
func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
