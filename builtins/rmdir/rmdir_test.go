package rmdir_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/rmdir"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := rmdir.New().Execute(context.Background(), append([]string{"rmdir"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestRmdirEmpty(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/a", 0o755)
	_, _, code := runCmd(t, mfs, "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/a"); err == nil {
		t.Fatal("expected /a gone")
	}
}

func TestRmdirNonEmpty(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/a", 0o755)
	_ = mfs.WriteFile("/a/x", []byte("x"), 0o644)
	_, err, code := runCmd(t, mfs, "a")
	if code == 0 || err == "" {
		t.Fatalf("expected error on non-empty dir, err=%q code=%d", err, code)
	}
}

func TestRmdirParents(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/a/b/c", 0o755)
	_, _, code := runCmd(t, mfs, "-p", "a/b/c")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/a"); err == nil {
		t.Fatal("expected /a removed with -p")
	}
}

func TestRmdirHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: rmdir") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestRmdirMissingOperand(t *testing.T) {
	_, err, code := runCmd(t, nil)
	if code == 0 || !strings.Contains(err, "missing") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestRmdirUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-z", "x")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
