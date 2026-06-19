// Package mountfs implements a mount-table FileSystem composed of one
// or more child FileSystems mounted at virtual paths over a base FS.
//
// Each public method resolves the requested path to the longest-prefix
// mount, translates the path into the child's namespace, and forwards
// the call. Cross-mount Rename and Link fall back to copy+delete.
package mountfs

import (
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	gobashfs "github.com/mark3labs/go-bash/fs"
)

// Mount associates a virtual mount point with a child FileSystem.
type Mount struct {
	Path       string
	FileSystem gobashfs.FileSystem
}

// Options configures an FS instance.
type Options struct {
	Base   gobashfs.FileSystem
	Mounts []Mount
}

// FS is the composed FileSystem.
type FS struct {
	mu     sync.RWMutex
	base   gobashfs.FileSystem
	mounts []Mount
}

// New constructs an FS.
func New(opts Options) (*FS, error) {
	if opts.Base == nil {
		return nil, errors.New("mountfs: Base is required")
	}
	f := &FS{base: opts.Base}
	for _, m := range opts.Mounts {
		if err := f.Mount(m.Path, m.FileSystem); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// Mount adds a child FileSystem at path. The path is cleaned; an empty
// or root path means the child replaces the base (rejected here — use
// Options.Base instead).
func (f *FS) Mount(path string, child gobashfs.FileSystem) error {
	clean := gobashfs.Clean(path)
	if clean == "/" || clean == "." {
		return fmt.Errorf("mountfs: cannot mount at %q (use Options.Base)", path)
	}
	if child == nil {
		return errors.New("mountfs: child FileSystem is nil")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mounts = append(f.mounts, Mount{Path: clean, FileSystem: child})
	// Sort by descending length so longest-prefix resolution is a
	// straight linear scan.
	sort.Slice(f.mounts, func(i, j int) bool {
		return len(f.mounts[i].Path) > len(f.mounts[j].Path)
	})
	return nil
}

// Unmount removes a child FileSystem at path.
func (f *FS) Unmount(path string) error {
	clean := gobashfs.Clean(path)
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, m := range f.mounts {
		if m.Path == clean {
			f.mounts = append(f.mounts[:i], f.mounts[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("mountfs: no mount at %q", path)
}

// resolve returns the child FileSystem and translated path for name. If
// no mount matches, returns the base FS with name unchanged.
func (f *FS) resolve(name string) (gobashfs.FileSystem, string) {
	clean := gobashfs.Clean(translate(name))
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, m := range f.mounts {
		if clean == m.Path || strings.HasPrefix(clean, m.Path+"/") {
			rel := strings.TrimPrefix(clean, m.Path)
			if rel == "" {
				rel = "/"
			}
			return m.FileSystem, rel
		}
	}
	return f.base, clean
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
// io/fs interfaces
// ----------------------------------------------------------------------

func (f *FS) Open(name string) (iofs.File, error) {
	child, p := f.resolve(name)
	return child.Open(strings.TrimPrefix(p, "/"))
}

func (f *FS) Stat(name string) (os.FileInfo, error) {
	child, p := f.resolve(name)
	return child.Stat(p)
}

func (f *FS) ReadDir(name string) ([]iofs.DirEntry, error) {
	child, p := f.resolve(name)
	entries, err := child.ReadDir(strings.TrimPrefix(p, "/"))
	if err != nil {
		// On the base FS, listing a directory that holds child mount
		// points should still succeed. Inject mount entries we hold
		// for this directory before returning the error.
		if child == f.base {
			return f.synthesizeMountEntries(name)
		}
		return nil, err
	}
	if child == f.base {
		// Add synthetic entries for any mount that sits directly under name.
		entries = append(entries, f.mountsUnder(name)...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	}
	return entries, nil
}

func (f *FS) synthesizeMountEntries(name string) ([]iofs.DirEntry, error) {
	entries := f.mountsUnder(name)
	if len(entries) == 0 {
		return nil, gobashfs.PathError("readdir", name, iofs.ErrNotExist)
	}
	return entries, nil
}

func (f *FS) mountsUnder(name string) []iofs.DirEntry {
	clean := gobashfs.Clean(translate(name))
	prefix := clean
	if prefix != "/" {
		prefix += "/"
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	seen := map[string]bool{}
	var out []iofs.DirEntry
	for _, m := range f.mounts {
		if !strings.HasPrefix(m.Path, prefix) {
			continue
		}
		rest := strings.TrimPrefix(m.Path, prefix)
		// Only the first segment matters (sub-mounts are children of
		// that segment, not of name).
		name := rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name = rest[:i]
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, mountEntry{name: name})
	}
	return out
}

// ----------------------------------------------------------------------
// Mutating ops
// ----------------------------------------------------------------------

func (f *FS) OpenFile(name string, flag int, perm os.FileMode) (gobashfs.File, error) {
	child, p := f.resolve(name)
	return child.OpenFile(p, flag, perm)
}
func (f *FS) Create(name string) (gobashfs.File, error) {
	child, p := f.resolve(name)
	return child.Create(p)
}
func (f *FS) Mkdir(name string, perm os.FileMode) error {
	child, p := f.resolve(name)
	return child.Mkdir(p, perm)
}
func (f *FS) MkdirAll(name string, perm os.FileMode) error {
	child, p := f.resolve(name)
	return child.MkdirAll(p, perm)
}
func (f *FS) Remove(name string) error {
	child, p := f.resolve(name)
	return child.Remove(p)
}
func (f *FS) RemoveAll(name string) error {
	child, p := f.resolve(name)
	return child.RemoveAll(p)
}

// Rename moves oldpath to newpath. If the two paths route to the same
// child FS, the child's Rename is delegated; otherwise copy+delete.
func (f *FS) Rename(oldpath, newpath string) error {
	src, sp := f.resolve(oldpath)
	dst, dp := f.resolve(newpath)
	if src == dst {
		return src.Rename(sp, dp)
	}
	data, err := src.ReadFile(sp)
	if err != nil {
		return err
	}
	if err := dst.WriteFile(dp, data, 0o644); err != nil {
		return err
	}
	return src.Remove(sp)
}
func (f *FS) Symlink(target, linkpath string) error {
	child, p := f.resolve(linkpath)
	return child.Symlink(target, p)
}
func (f *FS) Link(oldpath, newpath string) error {
	src, sp := f.resolve(oldpath)
	dst, dp := f.resolve(newpath)
	if src == dst {
		return src.Link(sp, dp)
	}
	data, err := src.ReadFile(sp)
	if err != nil {
		return err
	}
	return dst.WriteFile(dp, data, 0o644)
}
func (f *FS) Readlink(name string) (string, error) {
	child, p := f.resolve(name)
	return child.Readlink(p)
}
func (f *FS) Lstat(name string) (os.FileInfo, error) {
	child, p := f.resolve(name)
	return child.Lstat(p)
}
func (f *FS) Realpath(name string) (string, error) {
	child, p := f.resolve(name)
	return child.Realpath(p)
}
func (f *FS) Chmod(name string, mode os.FileMode) error {
	child, p := f.resolve(name)
	return child.Chmod(p, mode)
}
func (f *FS) Chtimes(name string, atime, mtime time.Time) error {
	child, p := f.resolve(name)
	return child.Chtimes(p, atime, mtime)
}
func (f *FS) ReadFile(name string) ([]byte, error) {
	child, p := f.resolve(name)
	return child.ReadFile(p)
}
func (f *FS) WriteFile(name string, data []byte, perm os.FileMode) error {
	child, p := f.resolve(name)
	return child.WriteFile(p, data, perm)
}
func (f *FS) AppendFile(name string, data []byte, perm os.FileMode) error {
	child, p := f.resolve(name)
	return child.AppendFile(p, data, perm)
}
func (f *FS) AllPaths() []string {
	out := f.base.AllPaths()
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, m := range f.mounts {
		for _, p := range m.FileSystem.AllPaths() {
			out = append(out, m.Path+strings.TrimPrefix(p, "/"))
		}
	}
	return out
}

// mountEntry implements iofs.DirEntry for synthetic mount-point entries.
type mountEntry struct{ name string }

func (m mountEntry) Name() string { return m.name }
func (m mountEntry) IsDir() bool  { return true }
func (m mountEntry) Type() iofs.FileMode {
	return iofs.ModeDir
}
func (m mountEntry) Info() (iofs.FileInfo, error) {
	return mountInfo(m), nil
}

type mountInfo struct{ name string }

func (m mountInfo) Name() string       { return m.name }
func (m mountInfo) Size() int64        { return 0 }
func (m mountInfo) Mode() os.FileMode  { return os.ModeDir | 0o755 }
func (m mountInfo) ModTime() time.Time { return time.Time{} }
func (m mountInfo) IsDir() bool        { return true }
func (m mountInfo) Sys() any           { return nil }

// Compile-time check that we still implement io.Closer-free helpers.
var _ io.Reader = (*os.File)(nil)
