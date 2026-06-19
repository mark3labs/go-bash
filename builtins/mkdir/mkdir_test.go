package mkdir_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/mkdir"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := mkdir.New().Execute(context.Background(), append([]string{"mkdir"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestMkdirSimple(t *testing.T) {
	mfs := memfs.New()
	_, _, code := runCmd(t, mfs, "foo")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, err := mfs.Stat("/foo")
	if err != nil || !fi.IsDir() {
		t.Fatalf("expected /foo dir: err=%v", err)
	}
}

func TestMkdirParents(t *testing.T) {
	mfs := memfs.New()
	_, _, code := runCmd(t, mfs, "-p", "a/b/c")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/a/b/c"); err != nil {
		t.Fatalf("expected /a/b/c: %v", err)
	}
}

func TestMkdirExisting(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/exists", 0o755)
	_, _, code := runCmd(t, mfs, "exists")
	if code == 0 {
		t.Fatalf("expected error on existing dir without -p")
	}
}

func TestMkdirParentsExisting(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/exists", 0o755)
	_, _, code := runCmd(t, mfs, "-p", "exists")
	if code != 0 {
		t.Fatalf("expected success with -p on existing dir")
	}
}

func TestMkdirMode(t *testing.T) {
	mfs := memfs.New()
	_, _, code := runCmd(t, mfs, "-m", "700", "secret")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/secret")
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("mode=%v", fi.Mode().Perm())
	}
}

func TestMkdirHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: mkdir") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestMkdirUnknownFlag(t *testing.T) {
	_, err, code := runCmd(t, nil, "-z", "foo")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestMkdirMissingOperand(t *testing.T) {
	_, err, code := runCmd(t, nil)
	if code == 0 || !strings.Contains(err, "missing") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
