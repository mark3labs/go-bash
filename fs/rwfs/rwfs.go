// Package rwfs implements a read-write FileSystem that maps virtual
// paths under a sandbox root onto the host filesystem.
//
// All writes hit the host disk after a path-containment check and an
// O_NOFOLLOW open (where supported). Symlinks at any intermediate
// component are rejected by default — see Options.AllowSymlinks.
package rwfs

import (
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gobashfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/realfs"
)

// Options configures an FS instance.
type Options struct {
	Root          string // absolute path; required
	AllowSymlinks bool   // when false, symlinks are rejected during resolve
}

// FS is the rwfs FileSystem.
type FS struct {
	root          string
	allowSymlinks bool
}

// New constructs an FS. The root directory is created if it does not
// already exist.
func New(opts Options) (*FS, error) {
	if opts.Root == "" {
		return nil, errors.New("rwfs: Root is required")
	}
	if !filepath.IsAbs(opts.Root) {
		return nil, fmt.Errorf("rwfs: Root %q is not absolute", opts.Root)
	}
	clean := filepath.Clean(opts.Root)
	if err := os.MkdirAll(clean, 0o755); err != nil {
		return nil, err
	}
	return &FS{root: clean, allowSymlinks: opts.AllowSymlinks}, nil
}

// resolve translates a virtual path into a real on-disk path.
func (f *FS) resolve(name string) (string, error) {
	real, _, err := realfs.ResolveAndValidate(f.root, name, f.allowSymlinks)
	return real, err
}

// ----------------------------------------------------------------------
// io/fs interfaces
// ----------------------------------------------------------------------

func (f *FS) Open(name string) (iofs.File, error) {
	file, err := f.OpenFile(translate(name), os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (f *FS) Stat(name string) (os.FileInfo, error) {
	real, err := f.resolve(translate(name))
	if err != nil {
		return nil, err
	}
	return os.Stat(real)
}

func (f *FS) ReadDir(name string) ([]iofs.DirEntry, error) {
	real, err := f.resolve(translate(name))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(real)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func translate(name string) string {
	if name == "" || name == "." {
		return "/"
	}
	if strings.HasPrefix(name, "/") {
		return name
	}
	return "/" + name
}

// ----------------------------------------------------------------------
// Mutating ops
// ----------------------------------------------------------------------

func (f *FS) OpenFile(name string, flag int, perm os.FileMode) (gobashfs.File, error) {
	real, err := f.resolve(name)
	if err != nil {
		return nil, err
	}
	osFile, err := realfs.OpenFileNoFollow(real, flag, perm, f.allowSymlinks)
	if err != nil {
		return nil, err
	}
	return &fileHandle{File: osFile}, nil
}

func (f *FS) Create(name string) (gobashfs.File, error) {
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
}

func (f *FS) Mkdir(name string, perm os.FileMode) error {
	real, err := f.resolve(name)
	if err != nil {
		return err
	}
	return os.Mkdir(real, perm)
}

func (f *FS) MkdirAll(name string, perm os.FileMode) error {
	real, err := f.resolve(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(real, perm)
}

func (f *FS) Remove(name string) error {
	real, err := f.resolve(name)
	if err != nil {
		return err
	}
	return os.Remove(real)
}

func (f *FS) RemoveAll(name string) error {
	real, err := f.resolve(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(real)
}

func (f *FS) Rename(oldpath, newpath string) error {
	src, err := f.resolve(oldpath)
	if err != nil {
		return err
	}
	dst, err := f.resolve(newpath)
	if err != nil {
		return err
	}
	return os.Rename(src, dst)
}

func (f *FS) Symlink(target, linkpath string) error {
	if !f.allowSymlinks {
		return gobashfs.PathError("symlink", linkpath, errors.New("symlinks disabled"))
	}
	real, err := f.resolve(linkpath)
	if err != nil {
		return err
	}
	// Translate target into a host-absolute path when it is a virtual
	// absolute path. Relative targets are stored as-is so they resolve
	// relative to the link's parent at follow time.
	t := target
	if strings.HasPrefix(t, "/") {
		t = filepath.Join(f.root, filepath.FromSlash(gobashfs.Clean(t)))
	}
	return os.Symlink(t, real)
}

func (f *FS) Link(oldpath, newpath string) error {
	src, err := f.resolve(oldpath)
	if err != nil {
		return err
	}
	dst, err := f.resolve(newpath)
	if err != nil {
		return err
	}
	return os.Link(src, dst)
}

func (f *FS) Readlink(name string) (string, error) {
	if !f.allowSymlinks {
		return "", gobashfs.PathError("readlink", name, errors.New("symlinks disabled"))
	}
	// Resolve without following symlinks at the final component.
	real := filepath.Join(f.root, filepath.FromSlash(gobashfs.Clean(translate(name))))
	target, err := os.Readlink(real)
	if err != nil {
		return "", err
	}
	// Inverse-translate host-absolute targets back into virtual paths
	// so callers see the same string they passed to Symlink. Relative
	// targets are returned verbatim.
	if filepath.IsAbs(target) {
		if rel, err := filepath.Rel(f.root, target); err == nil && !strings.HasPrefix(rel, "..") {
			return gobashfs.Clean("/" + filepath.ToSlash(rel)), nil
		}
	}
	return target, nil
}

func (f *FS) Lstat(name string) (os.FileInfo, error) {
	real := filepath.Join(f.root, filepath.FromSlash(gobashfs.Clean(translate(name))))
	return os.Lstat(real)
}

func (f *FS) Realpath(name string) (string, error) {
	real, err := f.resolve(name)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		return "", err
	}
	// Return virtual path (strip root prefix).
	rel, err := filepath.Rel(f.root, resolved)
	if err != nil {
		return resolved, nil
	}
	v := "/" + filepath.ToSlash(rel)
	return gobashfs.Clean(v), nil
}

func (f *FS) Chmod(name string, mode os.FileMode) error {
	real, err := f.resolve(name)
	if err != nil {
		return err
	}
	return os.Chmod(real, mode)
}

func (f *FS) Chtimes(name string, atime, mtime time.Time) error {
	real, err := f.resolve(name)
	if err != nil {
		return err
	}
	return os.Chtimes(real, atime, mtime)
}

func (f *FS) ReadFile(name string) ([]byte, error) {
	real, err := f.resolve(name)
	if err != nil {
		return nil, err
	}
	osFile, err := realfs.OpenFileNoFollow(real, os.O_RDONLY, 0, f.allowSymlinks)
	if err != nil {
		return nil, err
	}
	defer func() { _ = osFile.Close() }()
	fi, err := osFile.Stat()
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, gobashfs.PathError("read", name, gobashfs.ErrIsDirectory)
	}
	return io.ReadAll(osFile)
}

func (f *FS) WriteFile(name string, data []byte, perm os.FileMode) error {
	h, err := f.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := h.Write(data); err != nil {
		_ = h.Close()
		return err
	}
	return h.Close()
}

func (f *FS) AppendFile(name string, data []byte, perm os.FileMode) error {
	h, err := f.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, perm)
	if err != nil {
		return err
	}
	if _, err := h.Write(data); err != nil {
		_ = h.Close()
		return err
	}
	return h.Close()
}

// AllPaths returns nil — enumerating the entire host subtree is too
// expensive to do eagerly. Glob expansion against rwfs walks lazily via
// ReadDir.
func (f *FS) AllPaths() []string { return nil }

// ----------------------------------------------------------------------
// Handle wrapper
// ----------------------------------------------------------------------

type fileHandle struct{ *os.File }

func (h *fileHandle) Stat() (os.FileInfo, error) { return h.File.Stat() }
