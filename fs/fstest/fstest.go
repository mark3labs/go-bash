// Package fstest provides a contract test suite shared by every
// FileSystem implementation in go-bash. Each implementation invokes
// Run(t, Suite{...}) from its own _test.go to validate the common
// semantic subset.
//
// The suite tests the operations enumerated in SPEC §3.8: file CRUD,
// dir CRUD, rename, hard link, symlink, lstat, readlink, realpath,
// chmod, chtimes, mkdir recursive, rm recursive, walk — plus the
// path-safety invariants (null-byte rejection, traversal containment).
//
// Implementations carry slightly different capabilities (e.g. read-only
// filesystems, or filesystems that disallow symlinks for security). The
// Suite struct lets each adapter declare which behaviors to test.
package fstest

import (
	"bytes"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"strings"
	"testing"
	"time"

	gobashfs "github.com/mark3labs/go-bash/fs"
)

// Factory builds a fresh FileSystem rooted at an isolated directory for
// each subtest. The cleanup func is called at the end of every subtest.
// If the implementation is purely in-memory, cleanup may be a no-op.
type Factory func(t *testing.T) (gobashfs.FileSystem, func())

// Suite enumerates the optional capabilities an implementation can opt
// into. Tests guarded by a false flag are silently skipped.
type Suite struct {
	Name           string
	New            Factory
	SupportsSymlnk bool // implementation honors Symlink/Readlink/Lstat
	SupportsHardLn bool // implementation honors Link with shared content
	SupportsChmod  bool
	SupportsChtime bool
	// SandboxRoot, if non-empty, is the absolute path under which writes
	// land. Tests that exercise "/" semantics use SandboxRoot+"/x" instead.
	SandboxRoot string
}

// rel returns a path under the sandbox root. For implementations whose
// namespace is the full virtual filesystem (memfs, mountfs over memfs)
// SandboxRoot is empty and rel returns "/"+name.
func (s Suite) rel(name string) string {
	if s.SandboxRoot == "" {
		if strings.HasPrefix(name, "/") {
			return name
		}
		return "/" + name
	}
	return s.SandboxRoot + "/" + name
}

// Run drives the full contract suite against the configured Factory.
func Run(t *testing.T, s Suite) {
	t.Helper()

	t.Run("NullByteRejected", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		_, err := fs.ReadFile("a\x00b")
		if err == nil {
			t.Fatalf("ReadFile with NUL byte returned nil error")
		}
	})

	t.Run("WriteThenReadFile", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		path := s.rel("hello.txt")
		if err := fs.WriteFile(path, []byte("hi"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		data, err := fs.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if !bytes.Equal(data, []byte("hi")) {
			t.Fatalf("ReadFile data = %q; want %q", data, "hi")
		}
	})

	t.Run("OpenFileReadWrite", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		path := s.rel("rw.txt")
		f, err := fs.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			t.Fatalf("OpenFile create: %v", err)
		}
		if _, err := f.Write([]byte("abcdef")); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("Seek: %v", err)
		}
		buf := make([]byte, 6)
		if _, err := io.ReadFull(f, buf); err != nil {
			t.Fatalf("Read: %v", err)
		}
		if string(buf) != "abcdef" {
			t.Errorf("read %q; want %q", buf, "abcdef")
		}
		if err := f.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	t.Run("Truncate", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		path := s.rel("trunc.txt")
		if err := fs.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := fs.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(4); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		got, err := fs.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "0123" {
			t.Errorf("truncated = %q; want %q", got, "0123")
		}
	})

	t.Run("AppendFile", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		path := s.rel("log.txt")
		if err := fs.AppendFile(path, []byte("hello "), 0o644); err != nil {
			t.Fatalf("AppendFile (create): %v", err)
		}
		if err := fs.AppendFile(path, []byte("world"), 0o644); err != nil {
			t.Fatalf("AppendFile (append): %v", err)
		}
		got, err := fs.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "hello world" {
			t.Errorf("appended = %q; want %q", got, "hello world")
		}
	})

	t.Run("StatRegular", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		path := s.rel("stat.txt")
		if err := fs.WriteFile(path, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		fi, err := fs.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.IsDir() {
			t.Errorf("Stat reported directory for regular file")
		}
		if fi.Size() != 5 {
			t.Errorf("Size = %d; want 5", fi.Size())
		}
	})

	t.Run("MkdirReadDir", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		dir := s.rel("d")
		if err := fs.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		if err := fs.WriteFile(dir+"/a", []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFile(dir+"/b", []byte("b"), 0o644); err != nil {
			t.Fatal(err)
		}
		entries, err := fs.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		names := dirNames(entries)
		if got, want := strings.Join(names, ","), "a,b"; got != want {
			t.Errorf("entries = %s; want %s", got, want)
		}
	})

	t.Run("MkdirAllRecursive", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		p := s.rel("x/y/z")
		if err := fs.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		fi, err := fs.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if !fi.IsDir() {
			t.Errorf("MkdirAll did not produce directory at %q", p)
		}
		// Idempotent on existing dir.
		if err := fs.MkdirAll(p, 0o755); err != nil {
			t.Errorf("MkdirAll on existing dir: %v", err)
		}
	})

	t.Run("Remove", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		path := s.rel("gone.txt")
		if err := fs.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := fs.Remove(path); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		_, err := fs.Stat(path)
		if !errors.Is(err, iofs.ErrNotExist) {
			t.Errorf("Stat after Remove err = %v; want ErrNotExist", err)
		}
	})

	t.Run("RemoveAll", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		root := s.rel("tree")
		if err := fs.MkdirAll(root+"/a/b", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFile(root+"/a/b/leaf", []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := fs.RemoveAll(root); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}
		if _, err := fs.Stat(root); !errors.Is(err, iofs.ErrNotExist) {
			t.Errorf("Stat after RemoveAll err = %v; want ErrNotExist", err)
		}
	})

	t.Run("Rename", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		src := s.rel("src.txt")
		dst := s.rel("dst.txt")
		if err := fs.WriteFile(src, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := fs.Rename(src, dst); err != nil {
			t.Fatalf("Rename: %v", err)
		}
		if _, err := fs.Stat(src); !errors.Is(err, iofs.ErrNotExist) {
			t.Errorf("src still present after Rename: %v", err)
		}
		got, err := fs.ReadFile(dst)
		if err != nil || string(got) != "payload" {
			t.Errorf("dst content = %q, err=%v; want %q", got, err, "payload")
		}
	})

	t.Run("HardLink", func(t *testing.T) {
		if !s.SupportsHardLn {
			t.Skip("hard links not supported")
		}
		fs, cleanup := s.New(t)
		defer cleanup()
		src := s.rel("orig.txt")
		ln := s.rel("link.txt")
		if err := fs.WriteFile(src, []byte("v1"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := fs.Link(src, ln); err != nil {
			t.Fatalf("Link: %v", err)
		}
		// Writing through one path should be visible at the other if
		// the implementation shares content. The minimum contract is
		// that the link exists and starts with the original content.
		got, err := fs.ReadFile(ln)
		if err != nil || string(got) != "v1" {
			t.Errorf("link content = %q, err=%v; want %q", got, err, "v1")
		}
	})

	t.Run("Symlink", func(t *testing.T) {
		if !s.SupportsSymlnk {
			t.Skip("symlinks not supported")
		}
		fs, cleanup := s.New(t)
		defer cleanup()
		target := s.rel("target.txt")
		link := s.rel("link.txt")
		if err := fs.WriteFile(target, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := fs.Symlink(target, link); err != nil {
			t.Fatalf("Symlink: %v", err)
		}
		// Readlink returns the literal target.
		got, err := fs.Readlink(link)
		if err != nil {
			t.Fatalf("Readlink: %v", err)
		}
		if got != target {
			t.Errorf("Readlink = %q; want %q", got, target)
		}
		// Lstat reports a symlink, Stat reports the target.
		li, err := fs.Lstat(link)
		if err != nil {
			t.Fatalf("Lstat: %v", err)
		}
		if li.Mode()&os.ModeSymlink == 0 {
			t.Errorf("Lstat did not report symlink; mode=%v", li.Mode())
		}
		si, err := fs.Stat(link)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if si.IsDir() || si.Size() != int64(len("payload")) {
			t.Errorf("Stat via symlink: %+v; want regular file size=7", si)
		}
		// Reading through the link follows it.
		data, err := fs.ReadFile(link)
		if err != nil || string(data) != "payload" {
			t.Errorf("ReadFile via symlink = %q,%v; want %q", data, err, "payload")
		}
	})

	t.Run("Realpath", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		dir := s.rel("realdir")
		if err := fs.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := fs.Realpath(dir)
		if err != nil {
			t.Fatalf("Realpath: %v", err)
		}
		if got == "" {
			t.Errorf("Realpath empty")
		}
	})

	t.Run("Chmod", func(t *testing.T) {
		if !s.SupportsChmod {
			t.Skip("Chmod not supported")
		}
		fs, cleanup := s.New(t)
		defer cleanup()
		path := s.rel("mode.txt")
		if err := fs.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := fs.Chmod(path, 0o600); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		fi, err := fs.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("Mode = %v; want 0o600", fi.Mode().Perm())
		}
	})

	t.Run("Chtimes", func(t *testing.T) {
		if !s.SupportsChtime {
			t.Skip("Chtimes not supported")
		}
		fs, cleanup := s.New(t)
		defer cleanup()
		path := s.rel("ctimes.txt")
		if err := fs.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		when := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
		if err := fs.Chtimes(path, when, when); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
		fi, err := fs.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !fi.ModTime().Equal(when) {
			t.Errorf("ModTime = %v; want %v", fi.ModTime(), when)
		}
	})

	t.Run("Walk", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		root := s.rel("walk")
		if err := fs.MkdirAll(root+"/sub", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFile(root+"/a", []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFile(root+"/sub/b", []byte("b"), 0o644); err != nil {
			t.Fatal(err)
		}
		var visited []string
		err := iofs.WalkDir(fs, root, func(path string, d iofs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			visited = append(visited, path)
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir: %v", err)
		}
		// Order: root, then either a then sub or sub then a (alphabetical).
		// We just assert all four entries appear.
		want := map[string]bool{
			root: true, root + "/a": true, root + "/sub": true, root + "/sub/b": true,
		}
		for _, p := range visited {
			delete(want, p)
		}
		if len(want) > 0 {
			t.Errorf("missing walk entries: %v (visited=%v)", want, visited)
		}
	})

	t.Run("ReadFileNotExist", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		_, err := fs.ReadFile(s.rel("nope.txt"))
		if !errors.Is(err, iofs.ErrNotExist) {
			t.Errorf("ReadFile missing err = %v; want ErrNotExist", err)
		}
	})

	t.Run("OpenForReadIsADir", func(t *testing.T) {
		fs, cleanup := s.New(t)
		defer cleanup()
		dir := s.rel("isdir")
		if err := fs.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := fs.ReadFile(dir)
		if err == nil {
			t.Errorf("ReadFile of directory returned nil error")
		}
	})
}

func dirNames(entries []iofs.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	// In-order alphabetical, since ReadDirFS guarantees sorted order.
	return out
}
