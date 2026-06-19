package find_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	_ "github.com/mark3labs/go-bash/builtins/echo"
	"github.com/mark3labs/go-bash/builtins/find"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func newReg() *command.Registry {
	r := command.NewRegistry()
	for _, c := range command.DefaultBuiltins() {
		r.Register(c)
	}
	return r
}

func run(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := find.New().Execute(context.Background(), append([]string{"find"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e, Registry: newReg(),
	})
	return o.String(), e.String(), res.ExitCode
}

func seed(t *testing.T) *memfs.FS {
	t.Helper()
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d/sub", 0o755)
	_ = mfs.WriteFile("/d/a.txt", []byte("a"), 0o644)
	_ = mfs.WriteFile("/d/b.log", []byte("bb"), 0o644)
	_ = mfs.WriteFile("/d/sub/c.txt", []byte("ccc"), 0o644)
	return mfs
}

func TestFindAll(t *testing.T) {
	mfs := seed(t)
	out, _, _ := run(t, mfs, "/d")
	for _, want := range []string{"/d", "/d/a.txt", "/d/b.log", "/d/sub", "/d/sub/c.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestFindName(t *testing.T) {
	mfs := seed(t)
	out, _, _ := run(t, mfs, "/d", "-name", "*.txt")
	if !strings.Contains(out, "a.txt") || !strings.Contains(out, "c.txt") || strings.Contains(out, "b.log") {
		t.Errorf("got %q", out)
	}
}

func TestFindType(t *testing.T) {
	mfs := seed(t)
	out, _, _ := run(t, mfs, "/d", "-type", "d")
	if !strings.Contains(out, "/d\n") || !strings.Contains(out, "/d/sub") {
		t.Errorf("got %q", out)
	}
}

func TestFindNot(t *testing.T) {
	mfs := seed(t)
	out, _, _ := run(t, mfs, "/d", "-not", "-name", "*.txt", "-type", "f")
	if strings.Contains(out, "a.txt") || !strings.Contains(out, "b.log") {
		t.Errorf("got %q", out)
	}
}

func TestFindExec(t *testing.T) {
	mfs := seed(t)
	out, _, _ := run(t, mfs, "/d", "-name", "a.txt", "-exec", "echo", "FOUND:{}", ";")
	if !strings.Contains(out, "FOUND:/d/a.txt") {
		t.Errorf("got %q", out)
	}
}

func TestFindGlobLimit(t *testing.T) {
	mfs := seed(t)
	var o, e bytes.Buffer
	res := find.New().Execute(context.Background(), []string{"find", "/d"}, &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e, Registry: newReg(),
		Limits: command.Limits{MaxGlobOperations: 2},
	})
	if res.ExitCode == 0 {
		t.Errorf("expected limit failure")
	}
}

func TestFindHelp(t *testing.T) {
	mfs := seed(t)
	out, _, code := run(t, mfs, "--help")
	if code != 0 || !strings.Contains(out, "Usage: find") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestFindUnknown(t *testing.T) {
	mfs := seed(t)
	_, e, code := run(t, mfs, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
