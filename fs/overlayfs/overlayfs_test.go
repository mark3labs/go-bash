package overlayfs_test

import (
	"errors"
	iofs "io/fs"
	"os"
	"path/filepath"
	"testing"

	gobashfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/fstest"
	"github.com/mark3labs/go-bash/fs/overlayfs"
)

func TestContract(t *testing.T) {
	fstest.Run(t, fstest.Suite{
		Name: "overlayfs",
		New: func(t *testing.T) (gobashfs.FileSystem, func()) {
			root := t.TempDir()
			fs, err := overlayfs.New(overlayfs.Options{Root: root, AllowSymlinks: true})
			if err != nil {
				t.Fatalf("overlayfs.New: %v", err)
			}
			return fs, func() {}
		},
		SupportsSymlnk: true,
		SupportsHardLn: true,
		SupportsChmod:  true,
		SupportsChtime: true,
	})
}

// TestReadsFromHostDisk confirms that pre-existing host content is
// readable through the overlay without copy-up.
func TestReadsFromHostDisk(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hostfile"), []byte("host"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := overlayfs.New(overlayfs.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile("/hostfile")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "host" {
		t.Errorf("got %q; want host", got)
	}
}

// TestWritesGoToOverlay confirms host disk is never mutated.
func TestWritesGoToOverlay(t *testing.T) {
	root := t.TempDir()
	fs, err := overlayfs.New(overlayfs.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile("/file", []byte("inmem"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Host disk has no such file.
	if _, err := os.Stat(filepath.Join(root, "file")); !errors.Is(err, iofs.ErrNotExist) {
		t.Errorf("host disk was mutated: %v", err)
	}
}

// TestTombstoneHidesHostFile confirms Remove() on a host-backed file
// hides it from subsequent reads but does not delete it on disk.
func TestTombstoneHidesHostFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rm.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := overlayfs.New(overlayfs.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Remove("/rm.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat("/rm.txt"); !errors.Is(err, iofs.ErrNotExist) {
		t.Errorf("Stat after Remove err = %v; want ErrNotExist", err)
	}
	// Host disk still has it.
	if _, err := os.Stat(filepath.Join(root, "rm.txt")); err != nil {
		t.Errorf("host disk was unexpectedly modified: %v", err)
	}
}

// TestReadOnlyRejectsWrites confirms ReadOnly:true returns EROFS-style
// errors on mutating ops.
func TestReadOnlyRejectsWrites(t *testing.T) {
	root := t.TempDir()
	fs, err := overlayfs.New(overlayfs.Options{Root: root, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	err = fs.WriteFile("/x", []byte("y"), 0o644)
	if err == nil {
		t.Errorf("WriteFile on RO overlay returned nil error")
	}
}
