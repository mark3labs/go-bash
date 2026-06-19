package join_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/join"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := join.New().Execute(context.Background(), append([]string{"join"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestJoinBasic(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("1 alice\n2 bob\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("1 red\n2 blue\n"), 0o644)
	out, _, _ := run(t, mfs, "/a", "/b")
	if !strings.Contains(out, "1 alice red") || !strings.Contains(out, "2 bob blue") {
		t.Errorf("got %q", out)
	}
}

func TestJoinTab(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("1,a\n2,b\n"), 0o644)
	_ = mfs.WriteFile("/b", []byte("1,x\n2,y\n"), 0o644)
	out, _, _ := run(t, mfs, "-t", ",", "/a", "/b")
	if !strings.Contains(out, "1,a,x") {
		t.Errorf("got %q", out)
	}
}

func TestJoinHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "--help")
	if code != 0 || !strings.Contains(out, "Usage: join") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestJoinUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
