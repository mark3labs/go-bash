package tee_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/tee"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, stdin string, lim int, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := tee.New().Execute(context.Background(), append([]string{"tee"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
		Limits: command.Limits{MaxFileDescriptors: lim},
	})
	return o.String(), e.String(), res.ExitCode
}

func TestTeeWritesAndStdout(t *testing.T) {
	mfs := memfs.New()
	out, _, code := runCmd(t, mfs, "hello", 0, "/a", "/b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "hello" {
		t.Errorf("stdout=%q", out)
	}
	a, _ := mfs.ReadFile("/a")
	b, _ := mfs.ReadFile("/b")
	if string(a) != "hello" || string(b) != "hello" {
		t.Errorf("a=%q b=%q", a, b)
	}
}

func TestTeeAppend(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("X"), 0o644)
	_, _, _ = runCmd(t, mfs, "Y", 0, "-a", "/a")
	a, _ := mfs.ReadFile("/a")
	if string(a) != "XY" {
		t.Errorf("got %q", a)
	}
}

func TestTeeFDLimit(t *testing.T) {
	mfs := memfs.New()
	_, e, code := runCmd(t, mfs, "", 2, "/a", "/b") // stdout + 2 files > 2
	if code == 0 || !strings.Contains(e, "too many open files") {
		t.Errorf("expected limit, got code=%d e=%q", code, e)
	}
}

func TestTeeHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := runCmd(t, mfs, "", 0, "--help")
	if code != 0 || !strings.Contains(out, "Usage: tee") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestTeeUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := runCmd(t, mfs, "", 0, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
