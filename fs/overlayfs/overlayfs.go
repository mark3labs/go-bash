// Package overlayfs implements an overlay FileSystem where writes land
// in an in-memory overlay and reads come from the overlay-or-real-disk
// union.
//
// Semantics:
//   - Reads: serve from the overlay if present; otherwise from the host
//     subtree under Root.
//   - Writes: always go into the overlay; the host disk is never modified.
//   - Deletes: store a tombstone in the overlay; later reads see ENOENT.
//   - Listing: union of host readdir + overlay entries, minus tombstones.
//   - Symlinks: rejected by default for security; opt in via AllowSymlinks.
package overlayfs

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
	"github.com/mark3labs/go-bash/fs/memfs"
	"github.com/mark3labs/go-bash/fs/realfs"
)

// Options configures an FS instance.
type Options struct {
	Root          string // required, must be absolute
	AllowSymlinks bool   // default false
	ReadOnly      bool   // if true, mutating ops return EROFS
}

// FS is the overlay FileSystem.
type FS struct {
	root          string
	allowSymlinks bool
	readOnly      bool
	overlay       *memfs.FS

	// tombstones maps virtual paths whose host content has been deleted
	// or shadowed by an overlay entry. We track them so subsequent
	// reads / listings see the right thing.
	tombstones map[string]struct{}
}

// New constructs an FS.
func New(opts Options) (*FS, error) {
	if opts.Root == "" {
		return nil, errors.New("overlayfs: Root is required")
	}
	if !filepath.IsAbs(opts.Root) {
		return nil, fmt.Errorf("overlayfs: Root %q is not absolute", opts.Root)
	}
	return &FS{
		root:          filepath.Clean(opts.Root),
		allowSymlinks: opts.AllowSymlinks,
		readOnly:      opts.ReadOnly,
		overlay:       memfs.New(),
		tombstones:    map[string]struct{}{},
	}, nil
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

// resolveReal returns the host on-disk path for a virtual path. The
// symlink check (if enabled) is delegated to realfs.ResolveAndValidate.
func (f *FS) resolveReal(name string) (string, error) {
	real, _, err := realfs.ResolveAndValidate(f.root, name, f.allowSymlinks)
	return real, err
}

// isTombstoned reports whether the virtual path is hidden.
func (f *FS) isTombstoned(virt string) bool {
	_, ok := f.tombstones[gobashfs.Clean(virt)]
	return ok
}

// overlayHas reports whether the overlay holds an entry at virt.
func (f *FS) overlayHas(virt string) bool {
	_, err := f.overlay.Lstat(virt)
	return err == nil
}

func (f *FS) writable(name string) error {
	if f.readOnly {
		return gobashfs.PathError("write", name, gobashfs.ErrReadOnly)
	}
	return nil
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
	v := translate(name)
	if f.isTombstoned(v) {
		return nil, gobashfs.PathError("stat", name, iofs.ErrNotExist)
	}
	if f.overlayHas(v) {
		return f.overlay.Stat(v)
	}
	real, err := f.resolveReal(v)
	if err != nil {
		return nil, err
	}
	return os.Stat(real)
}

func (f *FS) ReadDir(name string) ([]iofs.DirEntry, error) {
	v := translate(name)
	if f.isTombstoned(v) {
		return nil, gobashfs.PathError("readdir", name, iofs.ErrNotExist)
	}
	entries := map[string]iofs.DirEntry{}
	// Real disk first.
	real, err := f.resolveReal(v)
	if err == nil {
		if disk, err := os.ReadDir(real); err == nil {
			for _, e := range disk {
				childPath := gobashfs.Clean(v + "/" + e.Name())
				if f.isTombstoned(childPath) {
					continue
				}
				entries[e.Name()] = e
			}
		}
	}
	// Overlay overrides / adds.
	if f.overlayHas(v) {
		overlayEntries, err := f.overlay.ReadDir(v)
		if err == nil {
			for _, e := range overlayEntries {
				entries[e.Name()] = e
			}
		}
	}
	if len(entries) == 0 {
		// Confirm at least one of the two layers reported the dir.
		if !f.overlayHas(v) {
			if _, err := os.Stat(real); err != nil {
				return nil, err
			}
		}
	}
	names := make([]string, 0, len(entries))
	for k := range entries {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]iofs.DirEntry, 0, len(names))
	for _, k := range names {
		out = append(out, entries[k])
	}
	return out, nil
}

// ----------------------------------------------------------------------
// Mutating ops
// ----------------------------------------------------------------------

func (f *FS) OpenFile(name string, flag int, perm os.FileMode) (gobashfs.File, error) {
	v := translate(name)
	if f.isTombstoned(v) && flag&os.O_CREATE == 0 {
		return nil, gobashfs.PathError("open", name, iofs.ErrNotExist)
	}
	// Read-only opens that miss the overlay fall through to the host disk.
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_TRUNC) == 0 {
		if f.overlayHas(v) {
			return f.overlay.OpenFile(v, flag, perm)
		}
		real, err := f.resolveReal(v)
		if err != nil {
			return nil, err
		}
		osFile, err := realfs.OpenFileNoFollow(real, flag, perm, f.allowSymlinks)
		if err != nil {
			return nil, err
		}
		return &osHandle{File: osFile}, nil
	}
	// Mutating opens — always go through the overlay. If the file
	// currently lives only on disk we copy it up first.
	if err := f.writable(name); err != nil {
		return nil, err
	}
	if !f.overlayHas(v) {
		// Copy up if base file exists and we're appending / read-write
		// without trunc.
		if flag&os.O_TRUNC == 0 {
			if real, err := f.resolveReal(v); err == nil {
				if data, err := os.ReadFile(real); err == nil {
					_ = f.overlay.WriteFile(v, data, perm)
				}
			}
		}
	}
	delete(f.tombstones, gobashfs.Clean(v))
	return f.overlay.OpenFile(v, flag, perm)
}

func (f *FS) Create(name string) (gobashfs.File, error) {
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
}

func (f *FS) Mkdir(name string, perm os.FileMode) error {
	if err := f.writable(name); err != nil {
		return err
	}
	v := translate(name)
	delete(f.tombstones, gobashfs.Clean(v))
	// Mirror parent dirs if they only exist on the host disk.
	if dir := gobashfs.Dirname(gobashfs.Clean(v)); dir != "/" && dir != "." {
		_ = f.overlay.MkdirAll(dir, 0o755)
	}
	return f.overlay.Mkdir(v, perm)
}

func (f *FS) MkdirAll(name string, perm os.FileMode) error {
	if err := f.writable(name); err != nil {
		return err
	}
	v := translate(name)
	delete(f.tombstones, gobashfs.Clean(v))
	return f.overlay.MkdirAll(v, perm)
}

func (f *FS) Remove(name string) error {
	if err := f.writable(name); err != nil {
		return err
	}
	v := translate(name)
	if f.overlayHas(v) {
		_ = f.overlay.Remove(v)
	}
	// If the host disk had the entry, tombstone it.
	if real, err := f.resolveReal(v); err == nil {
		if _, err := os.Lstat(real); err == nil {
			f.tombstones[gobashfs.Clean(v)] = struct{}{}
			return nil
		}
	}
	// If we removed from overlay and there's no host entry, success.
	return nil
}

func (f *FS) RemoveAll(name string) error {
	if err := f.writable(name); err != nil {
		return err
	}
	v := translate(name)
	if f.overlayHas(v) {
		_ = f.overlay.RemoveAll(v)
	}
	if real, err := f.resolveReal(v); err == nil {
		if _, err := os.Lstat(real); err == nil {
			f.tombstones[gobashfs.Clean(v)] = struct{}{}
		}
	}
	return nil
}

func (f *FS) Rename(oldpath, newpath string) error {
	if err := f.writable(oldpath); err != nil {
		return err
	}
	data, err := f.ReadFile(oldpath)
	if err != nil {
		return err
	}
	fi, _ := f.Stat(oldpath)
	mode := os.FileMode(0o644)
	if fi != nil {
		mode = fi.Mode().Perm()
	}
	if err := f.WriteFile(newpath, data, mode); err != nil {
		return err
	}
	return f.Remove(oldpath)
}

func (f *FS) Symlink(target, linkpath string) error {
	if !f.allowSymlinks {
		return gobashfs.PathError("symlink", linkpath, errors.New("symlinks disabled"))
	}
	if err := f.writable(linkpath); err != nil {
		return err
	}
	return f.overlay.Symlink(target, translate(linkpath))
}

func (f *FS) Link(oldpath, newpath string) error {
	if err := f.writable(newpath); err != nil {
		return err
	}
	data, err := f.ReadFile(oldpath)
	if err != nil {
		return err
	}
	return f.WriteFile(newpath, data, 0o644)
}

func (f *FS) Readlink(name string) (string, error) {
	if !f.allowSymlinks {
		return "", gobashfs.PathError("readlink", name, errors.New("symlinks disabled"))
	}
	v := translate(name)
	if f.overlayHas(v) {
		return f.overlay.Readlink(v)
	}
	real, err := f.resolveReal(v)
	if err != nil {
		return "", err
	}
	return os.Readlink(real)
}

func (f *FS) Lstat(name string) (os.FileInfo, error) {
	v := translate(name)
	if f.isTombstoned(v) {
		return nil, gobashfs.PathError("lstat", name, iofs.ErrNotExist)
	}
	if f.overlayHas(v) {
		return f.overlay.Lstat(v)
	}
	real, err := f.resolveReal(v)
	if err != nil {
		return nil, err
	}
	return os.Lstat(real)
}

func (f *FS) Realpath(name string) (string, error) {
	v := translate(name)
	if f.isTombstoned(v) {
		return "", gobashfs.PathError("realpath", name, iofs.ErrNotExist)
	}
	if f.overlayHas(v) {
		return f.overlay.Realpath(v)
	}
	real, err := f.resolveReal(v)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(real); err != nil {
		return "", err
	}
	return gobashfs.Clean(v), nil
}

func (f *FS) Chmod(name string, mode os.FileMode) error {
	if err := f.writable(name); err != nil {
		return err
	}
	v := translate(name)
	if !f.overlayHas(v) {
		// Copy up from disk so chmod is visible without mutating the host.
		data, err := f.ReadFile(name)
		if err == nil {
			_ = f.overlay.WriteFile(v, data, mode)
		} else {
			return err
		}
	}
	return f.overlay.Chmod(v, mode)
}

func (f *FS) Chtimes(name string, atime, mtime time.Time) error {
	if err := f.writable(name); err != nil {
		return err
	}
	v := translate(name)
	if !f.overlayHas(v) {
		data, err := f.ReadFile(name)
		if err != nil {
			return err
		}
		_ = f.overlay.WriteFile(v, data, 0o644)
	}
	return f.overlay.Chtimes(v, atime, mtime)
}

func (f *FS) ReadFile(name string) ([]byte, error) {
	v := translate(name)
	if f.isTombstoned(v) {
		return nil, gobashfs.PathError("read", name, iofs.ErrNotExist)
	}
	if f.overlayHas(v) {
		fi, err := f.overlay.Stat(v)
		if err == nil && fi.IsDir() {
			return nil, gobashfs.PathError("read", name, gobashfs.ErrIsDirectory)
		}
		return f.overlay.ReadFile(v)
	}
	real, err := f.resolveReal(v)
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
	if err := f.writable(name); err != nil {
		return err
	}
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
	if err := f.writable(name); err != nil {
		return err
	}
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

// AllPaths returns the overlay's contents — host paths require a
// disk walk we do not perform here. Globbing tests should populate the
// overlay or use memfs directly.
func (f *FS) AllPaths() []string {
	return f.overlay.AllPaths()
}

// MountPoint returns "/" for the overlay; multi-mount support comes
// later via mountfs.
func (f *FS) MountPoint() string { return "/" }

type osHandle struct{ *os.File }

func (h *osHandle) Stat() (os.FileInfo, error) { return h.File.Stat() }
