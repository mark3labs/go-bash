package tree_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/tree"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func setup(t *testing.T) *memfs.FS {
	t.Helper()
	mfs := memfs.New()
	_ = mfs.MkdirAll("/r/a", 0o755)
	_ = mfs.MkdirAll("/r/b", 0o755)
	_ = mfs.WriteFile("/r/a/x", []byte("x"), 0o644)
	_ = mfs.WriteFile("/r/y", []byte("y"), 0o644)
	return mfs
}

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := tree.New().Execute(context.Background(), append([]string{"tree"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestTreeBasic(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "/r")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "x") || !strings.Contains(out, "y") {
		t.Errorf("missing entries: %q", out)
	}
}

func TestTreeDepth(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-L", "1", "/r")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(out, "x") {
		t.Errorf("expected x absent at L=1: %q", out)
	}
}

func TestTreeDirsOnly(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-d", "/r")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if strings.Contains(out, "y\n") {
		t.Errorf("expected files filtered: %q", out)
	}
}

func TestTreeJSONRoundtrip(t *testing.T) {
	mfs := setup(t)
	out, _, code := runCmd(t, mfs, "-J", "/r")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json: %v\nout=%s", err, out)
	}
	if len(parsed) < 1 {
		t.Errorf("expected at least one node")
	}
}

func TestTreeHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: tree") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestTreeUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
