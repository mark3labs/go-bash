package diff_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/diff"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := diff.New().Execute(context.Background(), append([]string{"diff"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestSameFiles(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("x\n"), 0o644)
	out, _, code := run(t, mfs, "/a", "/b")
	if code != 0 || out != "" {
		t.Errorf("got code=%d out=%q", code, out)
	}
}

func TestDifferent(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("y\n"), 0o644)
	out, _, code := run(t, mfs, "/a", "/b")
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
	if !strings.Contains(out, "< x") || !strings.Contains(out, "> y") {
		t.Errorf("got %q", out)
	}
}

func TestBrief(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("y\n"), 0o644)
	out, _, code := run(t, mfs, "-q", "/a", "/b")
	if code != 1 || !strings.Contains(out, "differ") {
		t.Errorf("got %q code=%d", out, code)
	}
}

func TestUnified(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\nb\nc\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("a\nx\nc\n"), 0o644)
	out, _, code := run(t, mfs, "-u", "/a", "/b")
	if code != 1 {
		t.Errorf("code=%d", code)
	}
	if !strings.Contains(out, "--- /a") || !strings.Contains(out, "+++ /b") {
		t.Errorf("got %q", out)
	}
}

func TestErrorExit(t *testing.T) {
	mfs := memfs.New()
	_, _, code := run(t, mfs, "/nope1", "/nope2")
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "--help")
	if code != 0 || !strings.Contains(out, "Usage: diff") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
