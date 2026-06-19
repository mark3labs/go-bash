package wc_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/wc"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := wc.New().Execute(context.Background(), append([]string{"wc"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestWcDefault(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "hello world\nfoo bar\n")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "2") || !strings.Contains(out, "4") {
		t.Errorf("expected lines+words: %q", out)
	}
}

func TestWcL(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "a\nb\nc\n", "-l")
	if strings.TrimSpace(out) != "3" {
		t.Errorf("got %q", out)
	}
}

func TestWcW(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "foo bar baz\n", "-w")
	if strings.TrimSpace(out) != "3" {
		t.Errorf("got %q", out)
	}
}

func TestWcC(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "abc", "-c")
	if strings.TrimSpace(out) != "3" {
		t.Errorf("got %q", out)
	}
}

func TestWcM(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "héllo", "-m")
	if strings.TrimSpace(out) != "5" {
		t.Errorf("got %q", out)
	}
}

func TestWcLong(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "abc\nabcdef\n", "-L")
	if strings.TrimSpace(out) != "6" {
		t.Errorf("got %q", out)
	}
}

func TestWcFile(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x\ny\n"), 0o644)
	out, _, _ := run(t, mfs, "", "-l", "/a")
	if !strings.Contains(out, "2 /a") {
		t.Errorf("got %q", out)
	}
}

func TestWcMultiTotal(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("b\nc\n"), 0o644)
	out, _, _ := run(t, mfs, "", "-l", "/a", "/b")
	if !strings.Contains(out, "total") {
		t.Errorf("missing total: %q", out)
	}
	if !strings.Contains(out, "3 total") {
		t.Errorf("expected 3 total: %q", out)
	}
}

func TestWcHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: wc") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestWcUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
