package which_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/which"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func setup(t *testing.T) *memfs.FS {
	t.Helper()
	mfs := memfs.New()
	if err := mfs.MkdirAll("/usr/bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mfs.MkdirAll("/bin", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mfs.WriteFile("/usr/bin/foo", []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mfs.WriteFile("/bin/foo", []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return mfs
}

func run(t *testing.T, env map[string]string, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := which.New().Execute(context.Background(), append([]string{"which"}, args...), &command.Context{
		FS:     mfs,
		Env:    env,
		Stdout: &o,
		Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestWhichFindsFirst(t *testing.T) {
	mfs := setup(t)
	out, _, code := run(t, map[string]string{"PATH": "/usr/bin:/bin"}, mfs, "foo")
	if code != 0 || out != "/usr/bin/foo\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestWhichAll(t *testing.T) {
	mfs := setup(t)
	out, _, code := run(t, map[string]string{"PATH": "/usr/bin:/bin"}, mfs, "-a", "foo")
	if code != 0 || out != "/usr/bin/foo\n/bin/foo\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestWhichSilent(t *testing.T) {
	mfs := setup(t)
	out, _, code := run(t, map[string]string{"PATH": "/usr/bin"}, mfs, "-s", "foo")
	if code != 0 || out != "" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestWhichMissing(t *testing.T) {
	mfs := setup(t)
	out, _, code := run(t, map[string]string{"PATH": "/usr/bin"}, mfs, "nope")
	if code != 1 || out != "" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestWhichAbsolutePath(t *testing.T) {
	mfs := setup(t)
	out, _, code := run(t, nil, mfs, "/usr/bin/foo")
	if code != 0 || out != "/usr/bin/foo\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestWhichHelp(t *testing.T) {
	out, _, code := run(t, nil, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: which") {
		t.Errorf("out=%q code=%d", out, code)
	}
}
