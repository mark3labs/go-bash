package ls_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/ls"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func setup(t *testing.T) *memfs.FS {
	t.Helper()
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d", 0o755)
	_ = mfs.WriteFile("/d/a", []byte("aaa"), 0o644)
	_ = mfs.WriteFile("/d/b", []byte("bbbbb"), 0o755)
	_ = mfs.WriteFile("/d/.hidden", []byte("x"), 0o644)
	_ = mfs.MkdirAll("/d/sub", 0o755)
	return mfs
}

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := ls.New().Execute(context.Background(), append([]string{"ls"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestLsBasic(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(out, ".hidden") {
		t.Errorf("should not show hidden: %q", out)
	}
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") {
		t.Errorf("missing entries: %q", out)
	}
}

func TestLsAll(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-a", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, ".hidden") {
		t.Errorf("expected hidden: %q", out)
	}
}

func TestLsLong(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-l", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "drwx") && !strings.Contains(out, "-rw") {
		t.Errorf("expected long form: %q", out)
	}
}

func TestLsOnePerLine(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-1", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "a\n") || !strings.Contains(out, "b\n") {
		t.Errorf("expected one per line: %q", out)
	}
}

func TestLsClassify(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-F", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "sub/") {
		t.Errorf("expected /: %q", out)
	}
}

func TestLsDirOnly(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-d", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "/d") {
		t.Errorf("expected /d in output: %q", out)
	}
}

func TestLsRecursive(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-R", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "/sub") {
		t.Errorf("expected recursive: %q", out)
	}
}

func TestLsHuman(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-lh", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	_ = out
}

func TestLsHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: ls") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestLsUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
