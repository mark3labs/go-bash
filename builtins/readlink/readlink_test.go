package readlink_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/readlink"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := readlink.New().Execute(context.Background(), append([]string{"readlink"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestReadlinkSimple(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_ = mfs.Symlink("/a", "/b")
	out, _, code := runCmd(t, mfs, "/b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "/a\n" {
		t.Errorf("out=%q", out)
	}
}

func TestReadlinkNoNewline(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_ = mfs.Symlink("/a", "/b")
	out, _, code := runCmd(t, mfs, "-n", "/b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "/a" {
		t.Errorf("out=%q", out)
	}
}

func TestReadlinkExisting(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	_ = mfs.Symlink("/a", "/b")
	out, _, code := runCmd(t, mfs, "-e", "/b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "/a") {
		t.Errorf("out=%q", out)
	}
}

func TestReadlinkMissing(t *testing.T) {
	_, _, code := runCmd(t, memfs.New(), "/nope")
	if code == 0 {
		t.Fatal("expected error")
	}
}

func TestReadlinkHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: readlink") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestReadlinkUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z", "a")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
