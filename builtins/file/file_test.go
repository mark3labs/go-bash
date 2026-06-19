package file_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/file"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := file.New().Execute(context.Background(), append([]string{"file"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestFileASCII(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("hello world\n"), 0o644)
	out, _, code := runCmd(t, mfs, "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "ASCII text") {
		t.Errorf("out=%q", out)
	}
}

func TestFileEmpty(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte{}, 0o644)
	out, _, code := runCmd(t, mfs, "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "empty") {
		t.Errorf("out=%q", out)
	}
}

func TestFileBinary(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte{0, 1, 2, 255}, 0o644)
	out, _, code := runCmd(t, mfs, "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "data") {
		t.Errorf("out=%q", out)
	}
}

func TestFileDir(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/d", 0o755)
	out, _, code := runCmd(t, mfs, "/d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "directory") {
		t.Errorf("out=%q", out)
	}
}

func TestFileBrief(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("hi"), 0o644)
	out, _, code := runCmd(t, mfs, "-b", "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if strings.HasPrefix(out, "/a:") {
		t.Errorf("expected brief format: %q", out)
	}
}

func TestFileHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: file") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestFileMissing(t *testing.T) {
	_, _, code := runCmd(t, memfs.New())
	if code == 0 {
		t.Fatal("expected error")
	}
}

func TestFileUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z", "a")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
