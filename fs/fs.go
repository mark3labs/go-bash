package fs

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"time"
)

// FileSystem is the writable virtual filesystem interface implemented by
// memfs, overlayfs, rwfs, and mountfs. It embeds the read-only stdlib
// interfaces so callers can use io/fs helpers (fs.WalkDir, fs.ReadFile)
// against the sandbox.
//
// Every path argument is a forward-slash POSIX path; implementations
// MUST call Validate(path) (or equivalent) at every public entry point
// and reject null bytes and empty paths before any I/O.
type FileSystem interface {
	iofs.FS
	iofs.StatFS
	iofs.ReadDirFS

	OpenFile(name string, flag int, perm os.FileMode) (File, error)
	Create(name string) (File, error)
	Mkdir(name string, perm os.FileMode) error
	MkdirAll(name string, perm os.FileMode) error
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldpath, newpath string) error
	Symlink(target, linkpath string) error
	Link(oldpath, newpath string) error
	Readlink(name string) (string, error)
	Lstat(name string) (os.FileInfo, error)
	Realpath(name string) (string, error)
	Chmod(name string, mode os.FileMode) error
	Chtimes(name string, atime, mtime time.Time) error

	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	AppendFile(name string, data []byte, perm os.FileMode) error

	// AllPaths returns every regular-file path the implementation can
	// enumerate cheaply. Used by glob expansion against in-memory
	// filesystems. Implementations that would have to do unbounded I/O
	// (e.g. recursive disk scans) may return nil.
	AllPaths() []string
}

// File is a handle to an open file. It satisfies io/fs.File and adds
// the writer/seeker/truncate operations needed by shell redirection.
type File interface {
	iofs.File
	io.Writer
	io.Seeker
	Truncate(size int64) error
}

// FileInit describes a single file passed via BashOptions.Files. Exactly
// one of Content or Lazy must be non-nil for a file entry; both nil
// produces an empty file. Directory entries (Dir=true) ignore Content
// and Lazy.
//
// When Lazy is set, the underlying memfs creates a placeholder node that
// invokes the provider on first read, then memoizes the result. If a
// write occurs before any read, the provider is never called.
type FileInit struct {
	Content []byte
	Lazy    func(ctx context.Context) ([]byte, error)
	Mode    os.FileMode
	// Dir signals that the entry is a directory rather than a file.
	// The directory is created (recursively) with Mode (0o755 if zero).
	Dir bool
	// Symlink, when non-empty, marks the entry as a symbolic link
	// pointing at the named target. Content/Lazy are ignored.
	Symlink string
}

// Seek-whence constants re-exported so users of the fs package do not
// need a separate io import when constructing File operations. Matches
// io.SeekStart, io.SeekCurrent, io.SeekEnd values exactly.
const (
	SeekStart   = io.SeekStart
	SeekCurrent = io.SeekCurrent
	SeekEnd     = io.SeekEnd
)

// Helper sentinels exposed for cross-package error matching. Implementations
// should return wrapped *os.PathError values so callers can use errors.Is/As.
var (
	ErrNotImplemented = errors.New("operation not implemented")
	ErrReadOnly       = errors.New("read-only filesystem")
	ErrSymlinkLoop    = errors.New("too many levels of symbolic links")
	ErrNotDirectory   = errors.New("not a directory")
	ErrIsDirectory    = errors.New("is a directory")
)

// PathError builds an *os.PathError with the supplied op/path/err triple.
// Centralized so implementations stay consistent and so the type can be
// swapped without churn.
func PathError(op, path string, err error) error {
	return &os.PathError{Op: op, Path: path, Err: err}
}
