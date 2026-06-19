package memfs_test

import (
	"context"
	"testing"

	gobashfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/fstest"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestContract(t *testing.T) {
	fstest.Run(t, fstest.Suite{
		Name: "memfs",
		New: func(t *testing.T) (gobashfs.FileSystem, func()) {
			return memfs.New(), func() {}
		},
		SupportsSymlnk: true,
		SupportsHardLn: true,
		SupportsChmod:  true,
		SupportsChtime: true,
	})
}

// TestSeedLazy ensures lazy file providers are invoked at first read
// and then cached for subsequent reads.
func TestSeedLazy(t *testing.T) {
	calls := 0
	m := memfs.New()
	err := m.Seed(map[string]gobashfs.FileInit{
		"/lazy.txt": {Lazy: func(_ context.Context) ([]byte, error) {
			calls++
			return []byte("from-lazy"), nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("lazy ran before any read; calls=%d", calls)
	}
	got, err := m.ReadFile("/lazy.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-lazy" {
		t.Errorf("got %q; want from-lazy", got)
	}
	if calls != 1 {
		t.Errorf("calls = %d; want 1", calls)
	}
	// Re-read should not call the provider again.
	_, _ = m.ReadFile("/lazy.txt")
	if calls != 1 {
		t.Errorf("calls = %d after re-read; want 1", calls)
	}
}

// TestSeedFiles ensures plain file/directory entries from Seed are
// readable through the FileSystem API.
func TestSeedFiles(t *testing.T) {
	m := memfs.New()
	if err := m.Seed(map[string]gobashfs.FileInit{
		"/etc/hosts":     {Content: []byte("127.0.0.1 localhost\n")},
		"/var/log":       {Dir: true},
		"/home/user/.bashrc": {Content: []byte("echo hi\n")},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := m.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "127.0.0.1 localhost\n" {
		t.Errorf("got %q", got)
	}
	fi, err := m.Stat("/var/log")
	if err != nil || !fi.IsDir() {
		t.Errorf("Stat /var/log: %+v %v", fi, err)
	}
}

// TestHardLinkSharesContent verifies the shared-content semantic of
// hard links: writing through one path is observable through the other.
func TestHardLinkSharesContent(t *testing.T) {
	m := memfs.New()
	if err := m.WriteFile("/a", []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Link("/a", "/b"); err != nil {
		t.Fatal(err)
	}
	if err := m.WriteFile("/a", []byte("v2-via-a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// After WriteFile (which truncates) the content slice has been
	// rewritten. Hard-link semantics require both paths to observe v2.
	got, err := m.ReadFile("/b")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2-via-a" {
		t.Errorf("hard link did not share content; b=%q", got)
	}
}
