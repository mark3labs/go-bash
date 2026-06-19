package rm_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/rm"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := rm.New().Execute(context.Background(), append([]string{"rm"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestRmFile(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/a"); err == nil {
		t.Fatal("expected removed")
	}
}

func TestRmMissing(t *testing.T) {
	_, _, code := runCmd(t, memfs.New(), "nope")
	if code == 0 {
		t.Fatal("expected error")
	}
}

func TestRmForceMissing(t *testing.T) {
	_, _, code := runCmd(t, memfs.New(), "-f", "nope")
	if code != 0 {
		t.Fatalf("force should silence: code=%d", code)
	}
}

func TestRmRecursive(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d/sub", 0o755)
	_ = mfs.WriteFile("/d/sub/x", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "-r", "d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/d"); err == nil {
		t.Fatal("expected /d removed")
	}
}

func TestRmDirWithoutR(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d", 0o755)
	_, _, code := runCmd(t, mfs, "d")
	if code == 0 {
		t.Fatal("expected error on dir without -r")
	}
}

func TestRmDirWithDFlag(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d", 0o755)
	_, _, code := runCmd(t, mfs, "-d", "d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRmCombinedShortFlags(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d", 0o755)
	_ = mfs.WriteFile("/d/x", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "-rf", "d", "nope")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestRmInteractiveRejected(t *testing.T) {
	_, err, code := runCmd(t, memfs.New(), "-i", "x")
	if code == 0 || !strings.Contains(err, "interactive") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestRmHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: rm") {
		t.Errorf("out=%q code=%d", out, code)
	}
}
