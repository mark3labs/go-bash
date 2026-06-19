package rwfs_test

import (
	"testing"

	gobashfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/fstest"
	"github.com/mark3labs/go-bash/fs/rwfs"
)

func TestContract(t *testing.T) {
	fstest.Run(t, fstest.Suite{
		Name: "rwfs",
		New: func(t *testing.T) (gobashfs.FileSystem, func()) {
			root := t.TempDir()
			fs, err := rwfs.New(rwfs.Options{Root: root, AllowSymlinks: true})
			if err != nil {
				t.Fatalf("rwfs.New: %v", err)
			}
			return fs, func() {}
		},
		SupportsSymlnk: true,
		SupportsHardLn: true,
		SupportsChmod:  true,
		SupportsChtime: true,
	})
}

// TestPathTraversalRejected confirms that "/../../../etc/passwd"
// resolves to a path inside the sandbox root (which won't exist there),
// not to the host /etc/passwd.
func TestPathTraversalRejected(t *testing.T) {
	root := t.TempDir()
	fs, err := rwfs.New(rwfs.Options{Root: root, AllowSymlinks: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile("/../../etc/passwd"); err == nil {
		t.Errorf("traversal succeeded; expected error")
	}
}
