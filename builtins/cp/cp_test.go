package cp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/cp"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := cp.New().Execute(context.Background(), append([]string{"cp"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestCpFile(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("hello"), 0o644)
	_, _, code := runCmd(t, mfs, "a", "b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, _ := mfs.ReadFile("/b")
	if string(data) != "hello" {
		t.Errorf("got %q", data)
	}
}

func TestCpFileToDir(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_ = mfs.MkdirAll("/d", 0o755)
	_, _, code := runCmd(t, mfs, "a", "d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/d/a"); err != nil {
		t.Errorf("expected /d/a: %v", err)
	}
}

func TestCpRecursive(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/src/sub", 0o755)
	_ = mfs.WriteFile("/src/sub/x", []byte("X"), 0o644)
	_, _, code := runCmd(t, mfs, "-r", "src", "dst")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, _ := mfs.ReadFile("/dst/sub/x")
	if string(data) != "X" {
		t.Errorf("got %q", data)
	}
}

func TestCpDirWithoutR(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/src", 0o755)
	_, _, code := runCmd(t, mfs, "src", "dst")
	if code == 0 {
		t.Fatal("expected error: dir without -r")
	}
}

func TestCpInteractiveRejected(t *testing.T) {
	_, err, code := runCmd(t, memfs.New(), "-i", "a", "b")
	if code == 0 || !strings.Contains(err, "interactive") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestCpHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: cp") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestCpMissingOperand(t *testing.T) {
	_, _, code := runCmd(t, memfs.New())
	if code == 0 {
		t.Fatal("expected error")
	}
}

func TestCpArchiveBundle(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/src/sub", 0o755)
	_ = mfs.WriteFile("/src/sub/x", []byte("X"), 0o600)
	_, _, code := runCmd(t, mfs, "-a", "src", "dst")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, err := mfs.Stat("/dst/sub/x")
	if err != nil {
		t.Fatalf("missing: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("perms not preserved: %v", fi.Mode().Perm())
	}
}
