package mountfs_test

import (
	"testing"

	gobashfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/fstest"
	"github.com/mark3labs/go-bash/fs/memfs"
	"github.com/mark3labs/go-bash/fs/mountfs"
)

// TestContract runs the shared FileSystem contract suite against a
// mountfs whose base is a memfs and which has no child mounts. This is
// the trivial composition case but exercises every interface method.
func TestContract(t *testing.T) {
	fstest.Run(t, fstest.Suite{
		Name: "mountfs",
		New: func(t *testing.T) (gobashfs.FileSystem, func()) {
			fs, err := mountfs.New(mountfs.Options{Base: memfs.New()})
			if err != nil {
				t.Fatalf("mountfs.New: %v", err)
			}
			return fs, func() {}
		},
		SupportsSymlnk: true,
		SupportsHardLn: true,
		SupportsChmod:  true,
		SupportsChtime: true,
	})
}

// TestMountRouting verifies that a path under a mount point routes to
// the child FS rather than the base.
func TestMountRouting(t *testing.T) {
	base := memfs.New()
	child := memfs.New()
	if err := child.WriteFile("/inner.txt", []byte("from-child"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := mountfs.New(mountfs.Options{
		Base:   base,
		Mounts: []mountfs.Mount{{Path: "/mnt", FileSystem: child}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile("/mnt/inner.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-child" {
		t.Errorf("got %q", got)
	}
	// And a write under /mnt lands in child, not base.
	if err := fs.WriteFile("/mnt/new.txt", []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := child.ReadFile("/new.txt"); err != nil {
		t.Errorf("child did not receive write: %v", err)
	}
	if _, err := base.ReadFile("/mnt/new.txt"); err == nil {
		t.Errorf("base received write that should have gone to child")
	}
}
