package rg_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/rg"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, files map[string]string, args ...string) (string, string, int) {
	t.Helper()
	fs := memfs.New()
	for p, content := range files {
		_ = fs.MkdirAll(parentDir(p), 0o755)
		if err := fs.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	var o, e bytes.Buffer
	res := rg.New().Execute(context.Background(), append([]string{"rg"}, args...), &command.Context{
		FS: fs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func parentDir(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return "/"
	}
	return p[:idx]
}

func TestBasic(t *testing.T) {
	files := map[string]string{"/d/a.txt": "foo\nbar\nfoobar\n"}
	out, _, code := runCmd(t, files, "foo", "/d")
	if code != 0 || !strings.Contains(out, "1:foo") {
		t.Errorf("out=%q exit=%d", out, code)
	}
}

func TestIgnoreCase(t *testing.T) {
	files := map[string]string{"/d/a.txt": "FOO\nbar\n"}
	out, _, _ := runCmd(t, files, "-i", "foo", "/d")
	if !strings.Contains(out, "FOO") {
		t.Errorf("got %q", out)
	}
}

func TestCount(t *testing.T) {
	files := map[string]string{"/d/a.txt": "a\na\nb\n"}
	out, _, _ := runCmd(t, files, "-c", "a", "/d")
	if !strings.Contains(out, ":2") {
		t.Errorf("got %q", out)
	}
}

func TestFilesOnly(t *testing.T) {
	files := map[string]string{"/d/a.txt": "alpha\n", "/d/b.txt": "beta\n"}
	out, _, _ := runCmd(t, files, "-l", "alpha", "/d")
	if !strings.Contains(out, "/d/a.txt") || strings.Contains(out, "/d/b.txt") {
		t.Errorf("got %q", out)
	}
}

func TestTypeFilter(t *testing.T) {
	files := map[string]string{"/d/a.go": "alpha\n", "/d/a.txt": "alpha\n"}
	out, _, _ := runCmd(t, files, "-t", "go", "alpha", "/d")
	if !strings.Contains(out, "/d/a.go") || strings.Contains(out, "/d/a.txt") {
		t.Errorf("got %q", out)
	}
}

func TestGlob(t *testing.T) {
	files := map[string]string{"/d/a.txt": "x\n", "/d/b.md": "x\n"}
	out, _, _ := runCmd(t, files, "-g", "*.md", "x", "/d")
	if !strings.Contains(out, "/d/b.md") || strings.Contains(out, "/d/a.txt") {
		t.Errorf("got %q", out)
	}
}

func TestHiddenSkipped(t *testing.T) {
	files := map[string]string{"/d/.hidden": "x\n", "/d/visible.txt": "x\n"}
	out, _, _ := runCmd(t, files, "x", "/d")
	if strings.Contains(out, ".hidden") {
		t.Errorf("hidden should be skipped: %q", out)
	}
	out, _, _ = runCmd(t, files, "--hidden", "x", "/d")
	if !strings.Contains(out, ".hidden") {
		t.Errorf("hidden should appear with --hidden: %q", out)
	}
}

func TestJSON(t *testing.T) {
	files := map[string]string{"/d/a.txt": "hello\nworld\n"}
	out, _, _ := runCmd(t, files, "--json", "hello", "/d")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least begin/match/end, got %d:\n%s", len(lines), out)
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("not JSON: %v: %q", err, lines[0])
	}
	if first["type"] != "begin" {
		t.Errorf("expected first event type begin, got %v", first["type"])
	}
	gotMatch := false
	for _, l := range lines {
		var ev map[string]any
		if err := json.Unmarshal([]byte(l), &ev); err == nil && ev["type"] == "match" {
			gotMatch = true
		}
	}
	if !gotMatch {
		t.Errorf("no match event")
	}
}

func TestNoMatch(t *testing.T) {
	files := map[string]string{"/d/a.txt": "alpha\n"}
	_, _, code := runCmd(t, files, "zzz", "/d")
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: rg") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := runCmd(t, nil, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}
