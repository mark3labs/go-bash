package comm_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/comm"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := comm.New().Execute(context.Background(), append([]string{"comm"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestCommBasic(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\nb\nc\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("b\nc\nd\n"), 0o644)
	out, _, _ := run(t, mfs, "/a", "/b")
	if !strings.Contains(out, "a\n") || !strings.Contains(out, "\tb") || !strings.Contains(out, "\td") {
		t.Errorf("got %q", out)
	}
}

func TestCommSuppress(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("a\nb\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("b\nc\n"), 0o644)
	out, _, _ := run(t, mfs, "-12", "/a", "/b")
	if out != "b\n" {
		t.Errorf("got %q", out)
	}
}

func TestCommHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "--help")
	if code != 0 || !strings.Contains(out, "Usage: comm") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestCommUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
