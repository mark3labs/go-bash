package ln_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/ln"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := ln.New().Execute(context.Background(), append([]string{"ln"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestLnSymbolic(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "-s", "/a", "/b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	tgt, _ := mfs.Readlink("/b")
	if tgt != "/a" {
		t.Errorf("target=%q", tgt)
	}
}

func TestLnHard(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_, _, code := runCmd(t, mfs, "/a", "/b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, _ := mfs.ReadFile("/b")
	if string(data) != "x" {
		t.Errorf("data=%q", data)
	}
}

func TestLnForce(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_ = mfs.WriteFile("/b", []byte("old"), 0o644)
	_, _, code := runCmd(t, mfs, "-sf", "/a", "/b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestLnHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: ln") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestLnUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z", "a", "b")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
