package cat_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/cat"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := cat.New().Execute(context.Background(), append([]string{"cat"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestCatStdin(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "hello\nworld\n")
	if code != 0 || out != "hello\nworld\n" {
		t.Errorf("got %q code=%d", out, code)
	}
}

func TestCatFile(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("abc\n"), 0o644)
	out, _, code := run(t, mfs, "", "/a")
	if code != 0 || out != "abc\n" {
		t.Errorf("got %q code=%d", out, code)
	}
}

func TestCatNumber(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x\ny\n"), 0o644)
	out, _, code := run(t, mfs, "", "-n", "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "     1\tx") || !strings.Contains(out, "     2\ty") {
		t.Errorf("got %q", out)
	}
}

func TestCatNumberNonblank(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x\n\ny\n"), 0o644)
	out, _, code := run(t, mfs, "", "-b", "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	// blanks are unnumbered
	if !strings.Contains(out, "     1\tx") || !strings.Contains(out, "     2\ty") {
		t.Errorf("got %q", out)
	}
}

func TestCatShowEnds(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\nb\n"), 0o644)
	out, _, _ := run(t, mfs, "", "-E", "/a")
	if !strings.Contains(out, "a$") || !strings.Contains(out, "b$") {
		t.Errorf("got %q", out)
	}
}

func TestCatShowTabs(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\tb\n"), 0o644)
	out, _, _ := run(t, mfs, "", "-T", "/a")
	if !strings.Contains(out, "a^Ib") {
		t.Errorf("got %q", out)
	}
}

func TestCatSqueeze(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\n\n\n\nb\n"), 0o644)
	out, _, _ := run(t, mfs, "", "-s", "/a")
	if strings.Count(out, "\n\n") != 1 {
		t.Errorf("squeeze failed: %q", out)
	}
}

func TestCatBundled(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\tb\n"), 0o644)
	out, _, _ := run(t, mfs, "", "-nT", "/a")
	if !strings.Contains(out, "a^Ib") || !strings.Contains(out, "     1\t") {
		t.Errorf("got %q", out)
	}
}

func TestCatMissing(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "", "/no")
	if code == 0 || !strings.Contains(e, "cat:") {
		t.Errorf("expected error, got code=%d e=%q", code, e)
	}
}

func TestCatHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: cat") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestCatUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
