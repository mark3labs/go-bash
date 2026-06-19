// Package memfs implements an in-memory FileSystem suitable for use as
// the default backing store of a Bash sandbox. It supports regular
// files, directories, hard links (via shared content), symbolic links,
// permissions, mtime, and lazy file providers (see fs.FileInit).
//
// All operations are protected by an internal sync.RWMutex. The
// implementation aims for byte-identical behavior with the just-bash TS
// in-memory FS where the surfaces overlap.
package memfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	gobashfs "github.com/mark3labs/go-bash/fs"
)

// fileContent is the byte slice + metadata shared by all hard links to
// the same underlying file. A separate type lets us share it by pointer
// so writes through any link path become visible at every other link.
type fileContent struct {
	mu   sync.Mutex // protects data only; node-tree mutations use FS mu
	data []byte
}

type nodeKind int

const (
	kindFile nodeKind = iota
	kindDir
	kindSymlink
	kindLazy
)

// node is a single entry in the in-memory tree. Directory nodes have
// children; file nodes have shared content; symlink nodes have a target
// string; lazy nodes carry a provider closure that resolves on first read.
type node struct {
	kind    nodeKind
	mode    os.FileMode
	mtime   time.Time
	atime   time.Time
	content *fileContent        // file & lazy after materialization
	target  string              // symlink
	lazy    func(context.Context) ([]byte, error)
	parent  *node
	name    string
	entries map[string]*node // dir
}

// FS is the in-memory FileSystem.
type FS struct {
	mu   sync.RWMutex
	root *node
	// fsCtx is used by lazy providers when no caller context is
	// available (e.g. via the iofs.FS reads from WalkDir).
	fsCtx context.Context
}

// New constructs an empty FS with a single root directory at "/".
func New() *FS {
	now := time.Now()
	root := &node{
		kind:    kindDir,
		mode:    0o755,
		mtime:   now,
		atime:   now,
		name:    "",
		entries: map[string]*node{},
	}
	root.parent = root
	return &FS{root: root, fsCtx: context.Background()}
}

// Seed loads the given FileInit map into the FS, creating parent
// directories as needed. It is intended for use by the gobash New()
// constructor when honoring BashOptions.Files.
func (m *FS) Seed(files map[string]gobashfs.FileInit) error {
	for p, init := range files {
		if err := gobashfs.Validate(p); err != nil {
			return err
		}
		clean := gobashfs.Clean(p)
		if init.Dir {
			mode := init.Mode
			if mode == 0 {
				mode = 0o755
			}
			if err := m.MkdirAll(clean, mode); err != nil {
				return err
			}
			continue
		}
		// Ensure parent.
		if dir := gobashfs.Dirname(clean); dir != "." && dir != "/" {
			if err := m.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		mode := init.Mode
		if mode == 0 {
			mode = 0o644
		}
		if init.Symlink != "" {
			if err := m.Symlink(init.Symlink, clean); err != nil {
				return err
			}
			continue
		}
		if init.Lazy != nil {
			if err := m.installLazy(clean, init.Lazy, mode); err != nil {
				return err
			}
			continue
		}
		if err := m.WriteFile(clean, init.Content, mode); err != nil {
			return err
		}
	}
	return nil
}

func (m *FS) installLazy(p string, lazy func(context.Context) ([]byte, error), mode os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	parent, name, err := m.lookupParent(p, true)
	if err != nil {
		return err
	}
	now := time.Now()
	n := &node{
		kind:   kindLazy,
		mode:   mode,
		mtime:  now,
		atime:  now,
		lazy:   lazy,
		parent: parent,
		name:   name,
	}
	parent.entries[name] = n
	return nil
}

// ----------------------------------------------------------------------
// internal: name resolution
// ----------------------------------------------------------------------

// lookupParent returns the parent directory node for p and the leaf name.
// If create is true, missing intermediate dirs are NOT created — caller
// uses MkdirAll for that. The flag is here so we can distinguish "parent
// must exist" from "parent must exist OR be the root" cleanly.
func (m *FS) lookupParent(p string, _ bool) (*node, string, error) {
	clean := gobashfs.Clean(p)
	if clean == "/" {
		return nil, "", gobashfs.PathError("open", p, errors.New("operation on root"))
	}
	dir := gobashfs.Dirname(clean)
	base := gobashfs.Basename(clean)
	d, err := m.resolveDir(dir, MaxFollow)
	if err != nil {
		return nil, "", err
	}
	return d, base, nil
}

// MaxFollow caps symlink traversal at the FS layer. Matches fs.MaxSymlinkDepth.
const MaxFollow = gobashfs.MaxSymlinkDepth

// resolveDir resolves dir, following symlinks, and asserts the result is
// a directory.
func (m *FS) resolveDir(p string, maxHops int) (*node, error) {
	n, err := m.resolveNode(p, true, maxHops)
	if err != nil {
		return nil, err
	}
	if n.kind != kindDir {
		return nil, gobashfs.PathError("open", p, gobashfs.ErrNotDirectory)
	}
	return n, nil
}

// resolveNode walks the tree from the root, following symlinks if
// followFinal is true. It does not follow trailing symlinks when
// followFinal is false (Lstat / Readlink semantics).
func (m *FS) resolveNode(p string, followFinal bool, maxHops int) (*node, error) {
	if err := gobashfs.Validate(p); err != nil {
		return nil, gobashfs.PathError("open", p, err)
	}
	clean := gobashfs.Clean(p)
	parts := gobashfs.SplitParts(clean)
	cur := m.root
	for i, name := range parts {
		isLast := i == len(parts)-1
		if cur.kind != kindDir {
			return nil, gobashfs.PathError("open", p, gobashfs.ErrNotDirectory)
		}
		next, ok := cur.entries[name]
		if !ok {
			return nil, gobashfs.PathError("open", p, iofs.ErrNotExist)
		}
		// Follow symlinks unless this is the final component and
		// followFinal=false.
		if next.kind == kindSymlink && (!isLast || followFinal) {
			if maxHops <= 0 {
				return nil, gobashfs.PathError("open", p, gobashfs.ErrSymlinkLoop)
			}
			// Resolve target relative to the current directory (parent
			// of the symlink), not to root.
			target := next.target
			var targetPath string
			if strings.HasPrefix(target, "/") {
				targetPath = target
			} else {
				targetPath = gobashfs.Resolve(absPath(cur), target)
			}
			resolved, err := m.resolveNode(targetPath, true, maxHops-1)
			if err != nil {
				return nil, err
			}
			next = resolved
		}
		cur = next
	}
	return cur, nil
}

// absPath returns the cleaned absolute path of n.
func absPath(n *node) string {
	if n == nil {
		return "/"
	}
	parts := []string{}
	for cur := n; cur != cur.parent; cur = cur.parent {
		parts = append([]string{cur.name}, parts...)
	}
	if len(parts) == 0 {
		return "/"
	}
	return "/" + path.Join(parts...)
}

// ----------------------------------------------------------------------
// io/fs.FS / StatFS / ReadDirFS
// ----------------------------------------------------------------------

// Open implements iofs.FS. Read-only.
func (m *FS) Open(name string) (iofs.File, error) {
	f, err := m.OpenFile(translateOpenName(name), os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// translateOpenName accepts both the io/fs.FS convention ("a/b" without
// leading slash) and our native absolute paths. Empty / "." → "/".
func translateOpenName(name string) string {
	if name == "" || name == "." {
		return "/"
	}
	if strings.HasPrefix(name, "/") {
		return name
	}
	return "/" + name
}

// Stat returns a FileInfo for the named entry, following symlinks.
func (m *FS) Stat(name string) (os.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, err := m.resolveNode(translateOpenName(name), true, MaxFollow)
	if err != nil {
		return nil, err
	}
	return newFileInfo(n), nil
}

// Lstat returns a FileInfo without following the final symlink.
func (m *FS) Lstat(name string) (os.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, err := m.resolveNode(translateOpenName(name), false, MaxFollow)
	if err != nil {
		return nil, err
	}
	return newFileInfo(n), nil
}

// ReadDir lists the entries in name (a directory). Result is sorted by
// name to match the io/fs.ReadDirFS contract.
func (m *FS) ReadDir(name string) ([]iofs.DirEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, err := m.resolveDir(translateOpenName(name), MaxFollow)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(n.entries))
	for k := range n.entries {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]iofs.DirEntry, 0, len(names))
	for _, k := range names {
		out = append(out, iofs.FileInfoToDirEntry(newFileInfo(n.entries[k])))
	}
	return out, nil
}

// ----------------------------------------------------------------------
// Mutating ops
// ----------------------------------------------------------------------

// OpenFile opens or creates a file according to the standard POSIX flags.
func (m *FS) OpenFile(name string, flag int, perm os.FileMode) (gobashfs.File, error) {
	if err := gobashfs.Validate(name); err != nil {
		return nil, gobashfs.PathError("open", name, err)
	}
	clean := gobashfs.Clean(translateOpenName(name))
	m.mu.Lock()
	// Resolve under write lock so the create-or-open race is atomic.
	n, err := m.resolveNode(clean, true, MaxFollow)
	if err != nil && !errors.Is(err, iofs.ErrNotExist) {
		m.mu.Unlock()
		return nil, err
	}
	if err == nil && flag&os.O_EXCL != 0 && flag&os.O_CREATE != 0 {
		m.mu.Unlock()
		return nil, gobashfs.PathError("open", name, iofs.ErrExist)
	}
	if errors.Is(err, iofs.ErrNotExist) {
		if flag&os.O_CREATE == 0 {
			m.mu.Unlock()
			return nil, gobashfs.PathError("open", name, iofs.ErrNotExist)
		}
		// Create new file node.
		parent, base, perr := m.lookupParent(clean, false)
		if perr != nil {
			m.mu.Unlock()
			return nil, perr
		}
		now := time.Now()
		n = &node{
			kind:    kindFile,
			mode:    perm & os.ModePerm,
			mtime:   now,
			atime:   now,
			content: &fileContent{},
			parent:  parent,
			name:    base,
		}
		parent.entries[base] = n
	}
	// Materialize lazy node on first open.
	if n.kind == kindLazy {
		if err := m.materializeLazy(n); err != nil {
			m.mu.Unlock()
			return nil, gobashfs.PathError("open", name, err)
		}
	}
	if n.kind == kindDir {
		// Opening a directory for reading is allowed (returns 0 bytes);
		// writing is not.
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_TRUNC|os.O_APPEND) != 0 {
			m.mu.Unlock()
			return nil, gobashfs.PathError("open", name, gobashfs.ErrIsDirectory)
		}
		m.mu.Unlock()
		return &dirHandle{n: n}, nil
	}
	if flag&os.O_TRUNC != 0 && (flag&(os.O_WRONLY|os.O_RDWR) != 0) {
		n.content.mu.Lock()
		n.content.data = n.content.data[:0]
		n.content.mu.Unlock()
		n.mtime = time.Now()
	}
	m.mu.Unlock()
	h := &fileHandle{
		fs:     m,
		n:      n,
		flag:   flag,
		readOK: flag&(os.O_WRONLY) == 0,
		// Writable if O_WRONLY or O_RDWR set; O_RDONLY = 0 so &mask trick.
		writeOK: flag&(os.O_WRONLY|os.O_RDWR) != 0,
		append:  flag&os.O_APPEND != 0,
	}
	return h, nil
}

func (m *FS) materializeLazy(n *node) error {
	if n.kind != kindLazy {
		return nil
	}
	data, err := n.lazy(m.fsCtx)
	if err != nil {
		return err
	}
	n.kind = kindFile
	n.content = &fileContent{data: data}
	n.lazy = nil
	return nil
}

// Create is shorthand for OpenFile(name, O_WRONLY|O_CREATE|O_TRUNC, 0o644).
func (m *FS) Create(name string) (gobashfs.File, error) {
	return m.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// Mkdir creates a single directory.
func (m *FS) Mkdir(name string, perm os.FileMode) error {
	if err := gobashfs.Validate(name); err != nil {
		return gobashfs.PathError("mkdir", name, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := gobashfs.Clean(translateOpenName(name))
	if clean == "/" {
		return gobashfs.PathError("mkdir", name, iofs.ErrExist)
	}
	parent, base, err := m.lookupParent(clean, false)
	if err != nil {
		return err
	}
	if _, ok := parent.entries[base]; ok {
		return gobashfs.PathError("mkdir", name, iofs.ErrExist)
	}
	now := time.Now()
	parent.entries[base] = &node{
		kind:    kindDir,
		mode:    perm & os.ModePerm,
		mtime:   now,
		atime:   now,
		parent:  parent,
		name:    base,
		entries: map[string]*node{},
	}
	return nil
}

// MkdirAll creates name and any missing parents.
func (m *FS) MkdirAll(name string, perm os.FileMode) error {
	if err := gobashfs.Validate(name); err != nil {
		return gobashfs.PathError("mkdir", name, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := gobashfs.Clean(translateOpenName(name))
	parts := gobashfs.SplitParts(clean)
	cur := m.root
	for _, p := range parts {
		next, ok := cur.entries[p]
		if !ok {
			now := time.Now()
			next = &node{
				kind:    kindDir,
				mode:    perm & os.ModePerm,
				mtime:   now,
				atime:   now,
				parent:  cur,
				name:    p,
				entries: map[string]*node{},
			}
			cur.entries[p] = next
		} else if next.kind != kindDir {
			return gobashfs.PathError("mkdir", name, gobashfs.ErrNotDirectory)
		}
		cur = next
	}
	return nil
}

// Remove deletes a single file or empty directory.
func (m *FS) Remove(name string) error {
	if err := gobashfs.Validate(name); err != nil {
		return gobashfs.PathError("remove", name, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := gobashfs.Clean(translateOpenName(name))
	if clean == "/" {
		return gobashfs.PathError("remove", name, errors.New("cannot remove root"))
	}
	// Find without following the final symlink (matches POSIX unlink).
	n, err := m.resolveNode(clean, false, MaxFollow)
	if err != nil {
		return err
	}
	if n.kind == kindDir && len(n.entries) > 0 {
		return gobashfs.PathError("remove", name, errors.New("directory not empty"))
	}
	delete(n.parent.entries, n.name)
	return nil
}

// RemoveAll deletes name and any descendants.
func (m *FS) RemoveAll(name string) error {
	if err := gobashfs.Validate(name); err != nil {
		return gobashfs.PathError("remove", name, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := gobashfs.Clean(translateOpenName(name))
	if clean == "/" {
		// Reset the root.
		m.root.entries = map[string]*node{}
		return nil
	}
	n, err := m.resolveNode(clean, false, MaxFollow)
	if err != nil {
		if errors.Is(err, iofs.ErrNotExist) {
			return nil
		}
		return err
	}
	delete(n.parent.entries, n.name)
	return nil
}

// Rename moves oldpath to newpath. Cross-directory renames are allowed.
func (m *FS) Rename(oldpath, newpath string) error {
	if err := gobashfs.Validate(oldpath); err != nil {
		return gobashfs.PathError("rename", oldpath, err)
	}
	if err := gobashfs.Validate(newpath); err != nil {
		return gobashfs.PathError("rename", newpath, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	srcClean := gobashfs.Clean(translateOpenName(oldpath))
	dstClean := gobashfs.Clean(translateOpenName(newpath))
	src, err := m.resolveNode(srcClean, false, MaxFollow)
	if err != nil {
		return err
	}
	dstParent, dstBase, err := m.lookupParent(dstClean, false)
	if err != nil {
		return err
	}
	// Detach from src parent.
	delete(src.parent.entries, src.name)
	// Replace dst if it exists.
	src.parent = dstParent
	src.name = dstBase
	dstParent.entries[dstBase] = src
	return nil
}

// Symlink creates a symbolic link at linkpath pointing to target. The
// target string is stored verbatim; resolution happens at lookup time.
func (m *FS) Symlink(target, linkpath string) error {
	if err := gobashfs.Validate(linkpath); err != nil {
		return gobashfs.PathError("symlink", linkpath, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := gobashfs.Clean(translateOpenName(linkpath))
	parent, base, err := m.lookupParent(clean, false)
	if err != nil {
		return err
	}
	if _, ok := parent.entries[base]; ok {
		return gobashfs.PathError("symlink", linkpath, iofs.ErrExist)
	}
	now := time.Now()
	parent.entries[base] = &node{
		kind:   kindSymlink,
		mode:   0o777 | os.ModeSymlink,
		mtime:  now,
		atime:  now,
		target: target,
		parent: parent,
		name:   base,
	}
	return nil
}

// Link creates a hard link at newpath pointing to the same content as
// oldpath. The two paths thereafter share the same content slice (writes
// through either become visible at the other).
func (m *FS) Link(oldpath, newpath string) error {
	if err := gobashfs.Validate(oldpath); err != nil {
		return gobashfs.PathError("link", oldpath, err)
	}
	if err := gobashfs.Validate(newpath); err != nil {
		return gobashfs.PathError("link", newpath, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	src, err := m.resolveNode(gobashfs.Clean(translateOpenName(oldpath)), true, MaxFollow)
	if err != nil {
		return err
	}
	if src.kind == kindDir {
		return gobashfs.PathError("link", oldpath, gobashfs.ErrIsDirectory)
	}
	if src.kind == kindLazy {
		if err := m.materializeLazy(src); err != nil {
			return gobashfs.PathError("link", oldpath, err)
		}
	}
	parent, base, err := m.lookupParent(gobashfs.Clean(translateOpenName(newpath)), false)
	if err != nil {
		return err
	}
	if _, ok := parent.entries[base]; ok {
		return gobashfs.PathError("link", newpath, iofs.ErrExist)
	}
	now := time.Now()
	parent.entries[base] = &node{
		kind:    kindFile,
		mode:    src.mode,
		mtime:   now,
		atime:   now,
		content: src.content, // shared
		parent:  parent,
		name:    base,
	}
	return nil
}

// Readlink returns the literal target of a symlink at name.
func (m *FS) Readlink(name string) (string, error) {
	if err := gobashfs.Validate(name); err != nil {
		return "", gobashfs.PathError("readlink", name, err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, err := m.resolveNode(gobashfs.Clean(translateOpenName(name)), false, MaxFollow)
	if err != nil {
		return "", err
	}
	if n.kind != kindSymlink {
		return "", gobashfs.PathError("readlink", name, errors.New("not a symlink"))
	}
	return n.target, nil
}

// Realpath canonicalizes name: cleans the path, follows symlinks, and
// returns the absolute path to the underlying object. It is *not* an
// existence check by itself, but a non-existent path returns ErrNotExist.
func (m *FS) Realpath(name string) (string, error) {
	if err := gobashfs.Validate(name); err != nil {
		return "", gobashfs.PathError("realpath", name, err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	n, err := m.resolveNode(gobashfs.Clean(translateOpenName(name)), true, MaxFollow)
	if err != nil {
		return "", err
	}
	return absPath(n), nil
}

// Chmod updates the permissions of name (following symlinks).
func (m *FS) Chmod(name string, mode os.FileMode) error {
	if err := gobashfs.Validate(name); err != nil {
		return gobashfs.PathError("chmod", name, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n, err := m.resolveNode(gobashfs.Clean(translateOpenName(name)), true, MaxFollow)
	if err != nil {
		return err
	}
	n.mode = (n.mode &^ os.ModePerm) | (mode & os.ModePerm)
	return nil
}

// Chtimes updates atime/mtime on name.
func (m *FS) Chtimes(name string, atime, mtime time.Time) error {
	if err := gobashfs.Validate(name); err != nil {
		return gobashfs.PathError("chtimes", name, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n, err := m.resolveNode(gobashfs.Clean(translateOpenName(name)), true, MaxFollow)
	if err != nil {
		return err
	}
	n.atime = atime
	n.mtime = mtime
	return nil
}

// ReadFile reads the entire contents of name into memory.
func (m *FS) ReadFile(name string) ([]byte, error) {
	f, err := m.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if fi, _ := f.Stat(); fi != nil && fi.IsDir() {
		return nil, gobashfs.PathError("read", name, gobashfs.ErrIsDirectory)
	}
	return io.ReadAll(f)
}

// WriteFile writes data to name, creating or truncating as needed.
func (m *FS) WriteFile(name string, data []byte, perm os.FileMode) error {
	f, err := m.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// AppendFile appends data to name, creating it if necessary.
func (m *FS) AppendFile(name string, data []byte, perm os.FileMode) error {
	f, err := m.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// AllPaths returns every regular file path in the FS. Used by glob
// expansion's getAllPaths in Phase 6.
func (m *FS) AllPaths() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	var walk func(n *node, prefix string)
	walk = func(n *node, prefix string) {
		for _, name := range sortedKeys(n.entries) {
			child := n.entries[name]
			p := prefix + "/" + name
			if prefix == "/" {
				p = "/" + name
			}
			switch child.kind {
			case kindFile, kindLazy:
				out = append(out, p)
			case kindDir:
				walk(child, p)
			}
		}
	}
	walk(m.root, "/")
	return out
}

func sortedKeys(m map[string]*node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ----------------------------------------------------------------------
// FileInfo / DirEntry
// ----------------------------------------------------------------------

type fileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	mtime time.Time
	isDir bool
}

func newFileInfo(n *node) os.FileInfo {
	fi := fileInfo{
		name:  n.name,
		mode:  n.mode,
		mtime: n.mtime,
		isDir: n.kind == kindDir,
	}
	if fi.name == "" {
		fi.name = "/"
	}
	switch n.kind {
	case kindFile:
		if n.content != nil {
			n.content.mu.Lock()
			fi.size = int64(len(n.content.data))
			n.content.mu.Unlock()
		}
	case kindDir:
		fi.mode |= os.ModeDir
	case kindSymlink:
		fi.size = int64(len(n.target))
		fi.mode |= os.ModeSymlink
	case kindLazy:
		// Size is 0 until materialized; cheap, deterministic answer.
	}
	return fi
}

func (f fileInfo) Name() string       { return f.name }
func (f fileInfo) Size() int64        { return f.size }
func (f fileInfo) Mode() os.FileMode  { return f.mode }
func (f fileInfo) ModTime() time.Time { return f.mtime }
func (f fileInfo) IsDir() bool        { return f.isDir }
func (f fileInfo) Sys() any           { return nil }

// ----------------------------------------------------------------------
// File handles
// ----------------------------------------------------------------------

type fileHandle struct {
	fs      *FS
	n       *node
	flag    int
	readOK  bool
	writeOK bool
	append  bool
	pos     int64
	closed  bool
	mu      sync.Mutex
}

func (f *fileHandle) Stat() (os.FileInfo, error) {
	return newFileInfo(f.n), nil
}

func (f *fileHandle) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, iofs.ErrClosed
	}
	if !f.readOK {
		return 0, errors.New("file not open for read")
	}
	f.n.content.mu.Lock()
	data := f.n.content.data
	f.n.content.mu.Unlock()
	if f.pos >= int64(len(data)) {
		return 0, io.EOF
	}
	n := copy(p, data[f.pos:])
	f.pos += int64(n)
	return n, nil
}

func (f *fileHandle) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, iofs.ErrClosed
	}
	if !f.writeOK {
		return 0, errors.New("file not open for write")
	}
	f.n.content.mu.Lock()
	defer f.n.content.mu.Unlock()
	if f.append {
		f.pos = int64(len(f.n.content.data))
	}
	end := f.pos + int64(len(p))
	if end > int64(len(f.n.content.data)) {
		grow := make([]byte, end)
		copy(grow, f.n.content.data)
		f.n.content.data = grow
	}
	n := copy(f.n.content.data[f.pos:], p)
	f.pos += int64(n)
	f.n.mtime = time.Now()
	return n, nil
}

func (f *fileHandle) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, iofs.ErrClosed
	}
	f.n.content.mu.Lock()
	size := int64(len(f.n.content.data))
	f.n.content.mu.Unlock()
	var np int64
	switch whence {
	case io.SeekStart:
		np = offset
	case io.SeekCurrent:
		np = f.pos + offset
	case io.SeekEnd:
		np = size + offset
	default:
		return 0, fmt.Errorf("invalid whence %d", whence)
	}
	if np < 0 {
		return 0, errors.New("negative seek")
	}
	f.pos = np
	return np, nil
}

func (f *fileHandle) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return iofs.ErrClosed
	}
	if !f.writeOK {
		return errors.New("file not open for write")
	}
	f.n.content.mu.Lock()
	defer f.n.content.mu.Unlock()
	if size < 0 {
		return errors.New("negative size")
	}
	cur := int64(len(f.n.content.data))
	if size < cur {
		f.n.content.data = f.n.content.data[:size]
	} else if size > cur {
		grow := make([]byte, size)
		copy(grow, f.n.content.data)
		f.n.content.data = grow
	}
	f.n.mtime = time.Now()
	return nil
}

func (f *fileHandle) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return iofs.ErrClosed
	}
	f.closed = true
	return nil
}

// dirHandle is the read-only handle returned by Open on a directory.
type dirHandle struct {
	n      *node
	pos    int
	closed bool
}

func (d *dirHandle) Stat() (os.FileInfo, error) { return newFileInfo(d.n), nil }
func (d *dirHandle) Read(_ []byte) (int, error) {
	return 0, gobashfs.PathError("read", d.n.name, gobashfs.ErrIsDirectory)
}
func (d *dirHandle) Write(_ []byte) (int, error) {
	return 0, gobashfs.PathError("write", d.n.name, gobashfs.ErrIsDirectory)
}
func (d *dirHandle) Seek(int64, int) (int64, error) { return 0, errors.New("seek on dir") }
func (d *dirHandle) Truncate(int64) error           { return gobashfs.ErrIsDirectory }
func (d *dirHandle) Close() error {
	if d.closed {
		return iofs.ErrClosed
	}
	d.closed = true
	return nil
}

// ReadDir on a dirHandle implements iofs.ReadDirFile.
func (d *dirHandle) ReadDir(n int) ([]iofs.DirEntry, error) {
	names := make([]string, 0, len(d.n.entries))
	for k := range d.n.entries {
		names = append(names, k)
	}
	sort.Strings(names)
	if d.pos >= len(names) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}
	end := len(names)
	if n > 0 && d.pos+n < end {
		end = d.pos + n
	}
	out := make([]iofs.DirEntry, 0, end-d.pos)
	for _, name := range names[d.pos:end] {
		out = append(out, iofs.FileInfoToDirEntry(newFileInfo(d.n.entries[name])))
	}
	d.pos = end
	return out, nil
}
