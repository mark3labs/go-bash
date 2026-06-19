package chmod_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/chmod"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := chmod.New().Execute(context.Background(), append([]string{"chmod"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestChmodNumeric(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "700", "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/a")
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("perm=%v", fi.Mode().Perm())
	}
}

func TestChmodSymbolic(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "u+x", "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/a")
	if fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("expected u+x, perm=%v", fi.Mode().Perm())
	}
}

func TestChmodSymbolicCombined(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "u+x,g-w,o=r", "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/a")
	got := fi.Mode().Perm()
	if got&0o100 == 0 {
		t.Errorf("u+x missing: %v", got)
	}
	if got&0o020 != 0 {
		t.Errorf("g-w not applied: %v", got)
	}
}

func TestChmodRecursive(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d/sub", 0o755)
	_ = mfs.WriteFile("/d/sub/x", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "-R", "700", "d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/d/sub/x")
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("perm=%v", fi.Mode().Perm())
	}
}

func TestChmodVerbose(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	out, _, code := runCmd(t, mfs, "-v", "755", "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "mode of") {
		t.Errorf("expected verbose output: %q", out)
	}
}

func TestChmodHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: chmod") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestChmodUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z", "755", "a")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestChmodMissing(t *testing.T) {
	_, err, code := runCmd(t, memfs.New())
	if code == 0 || !strings.Contains(err, "missing") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
