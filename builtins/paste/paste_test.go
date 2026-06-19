package paste_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/paste"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := paste.New().Execute(context.Background(), append([]string{"paste"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestPasteParallel(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("1\n2\n3\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("x\ny\nz\n"), 0o644)
	out, _, _ := run(t, mfs, "/a", "/b")
	if out != "1\tx\n2\ty\n3\tz\n" {
		t.Errorf("got %q", out)
	}
}

func TestPasteSerial(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("1\n2\n3\n"), 0o644)
	out, _, _ := run(t, mfs, "-s", "/a")
	if out != "1\t2\t3\n" {
		t.Errorf("got %q", out)
	}
}

func TestPasteDelim(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("1\n2\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("x\ny\n"), 0o644)
	out, _, _ := run(t, mfs, "-d", ",", "/a", "/b")
	if out != "1,x\n2,y\n" {
		t.Errorf("got %q", out)
	}
}

func TestPasteHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "--help")
	if code != 0 || !strings.Contains(out, "Usage: paste") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestPasteUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
