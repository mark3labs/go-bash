package du_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/du"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func setup(t *testing.T) *memfs.FS {
	t.Helper()
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d/sub", 0o755)
	_ = mfs.WriteFile("/d/a", make([]byte, 2000), 0o644)
	_ = mfs.WriteFile("/d/sub/b", make([]byte, 4096), 0o644)
	return mfs
}

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := du.New().Execute(context.Background(), append([]string{"du"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestDuSummarize(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-s", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "/d") {
		t.Errorf("out=%q", out)
	}
}

func TestDuHuman(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-s", "-h", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	// 6096 bytes total => 6.0K (one decimal) or 6K
	if !strings.Contains(out, "K") {
		t.Errorf("expected human size, out=%q", out)
	}
}

func TestDuBytes(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-s", "-b", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.HasPrefix(out, "6096\t") {
		t.Errorf("expected 6096 bytes total, got %q", out)
	}
}

func TestDuTotal(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-s", "-c", "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "total") {
		t.Errorf("expected total row: %q", out)
	}
}

func TestDuHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: du") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestDuUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
