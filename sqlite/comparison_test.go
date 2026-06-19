package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/internal/cmpfixture"
	"github.com/mark3labs/go-bash/sqlite"
)

// TestComparisonFixtures runs every *.json file in
// internal/testdata/fixtures/sqlite3/ as a subtest. Unlike the
// builtins/sqlite3 stub comparison test (which uses cmpfixture.RunDir
// directly), each fixture here is executed against a Bash on which
// sqlite.Register has installed the real modernc.org/sqlite-backed
// runtime.
func TestComparisonFixtures(t *testing.T) {
	dir := "../internal/testdata/fixtures/sqlite3"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Skip("no fixtures under " + dir)
	}
	for _, name := range names {
		t.Run(strings.TrimSuffix(name, ".json"), func(t *testing.T) {
			runFixture(t, filepath.Join(dir, name))
		})
	}
}

func runFixture(t *testing.T, path string) {
	t.Helper()
	fx, err := cmpfixture.Load(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	files := make(map[string]fs.FileInit, len(fx.Files))
	for p, content := range fx.Files {
		files[p] = fs.FileInit{Content: []byte(content)}
	}
	b, err := gobash.New(gobash.BashOptions{
		Cwd:   fx.Cwd,
		Env:   fx.Env,
		Files: files,
	})
	if err != nil {
		t.Fatalf("gobash.New: %v", err)
	}
	if err := sqlite.Register(b, sqlite.Options{}); err != nil {
		t.Fatalf("sqlite.Register: %v", err)
	}
	res, err := b.Exec(context.Background(), fx.Script, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != fx.Expected.Stdout {
		t.Errorf("stdout mismatch\n got: %q\nwant: %q", res.Stdout, fx.Expected.Stdout)
	}
	if res.Stderr != fx.Expected.Stderr {
		t.Errorf("stderr mismatch\n got: %q\nwant: %q", res.Stderr, fx.Expected.Stderr)
	}
	if res.ExitCode != fx.Expected.ExitCode {
		t.Errorf("exitCode mismatch\n got: %d\nwant: %d", res.ExitCode, fx.Expected.ExitCode)
	}
}
