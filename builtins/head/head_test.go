package head_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/head"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := head.New().Execute(context.Background(), append([]string{"head"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestHeadDefault(t *testing.T) {
	mfs := memfs.New()
	lines := ""
	for i := 1; i <= 20; i++ {
		lines += "L\n"
	}
	out, _, _ := run(t, mfs, lines)
	if strings.Count(out, "\n") != 10 {
		t.Errorf("expected 10 lines: %q", out)
	}
}

func TestHeadN(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "a\nb\nc\nd\n", "-n", "2")
	if out != "a\nb\n" {
		t.Errorf("got %q", out)
	}
}

func TestHeadNegativeN(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "a\nb\nc\nd\n", "-n", "-1")
	if out != "a\nb\nc\n" {
		t.Errorf("got %q", out)
	}
}

func TestHeadBytes(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "abcdef", "-c", "3")
	if out != "abc" {
		t.Errorf("got %q", out)
	}
}

func TestHeadBytesNegative(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "abcdef", "-c", "-2")
	if out != "abcd" {
		t.Errorf("got %q", out)
	}
}

func TestHeadFile(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x\ny\n"), 0o644)
	out, _, _ := run(t, mfs, "", "-n", "1", "/a")
	if out != "x\n" {
		t.Errorf("got %q", out)
	}
}

func TestHeadMultiHeader(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("b\n"), 0o644)
	out, _, _ := run(t, mfs, "", "/a", "/b")
	if !strings.Contains(out, "==> /a <==") || !strings.Contains(out, "==> /b <==") {
		t.Errorf("missing headers: %q", out)
	}
}

func TestHeadQuiet(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("b\n"), 0o644)
	out, _, _ := run(t, mfs, "", "-q", "/a", "/b")
	if strings.Contains(out, "==>") {
		t.Errorf("unexpected header: %q", out)
	}
}

func TestHeadShortN(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "1\n2\n3\n4\n", "-2")
	if out != "1\n2\n" {
		t.Errorf("got %q", out)
	}
}

func TestHeadHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: head") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestHeadUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
