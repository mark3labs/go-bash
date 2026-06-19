package mv_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/mv"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := mv.New().Execute(context.Background(), append([]string{"mv"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestMvFile(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("X"), 0o644)
	_, _, code := runCmd(t, mfs, "a", "b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/a"); err == nil {
		t.Fatal("expected /a gone")
	}
	data, _ := mfs.ReadFile("/b")
	if string(data) != "X" {
		t.Errorf("got %q", data)
	}
}

func TestMvIntoDir(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("X"), 0o644)
	_ = mfs.MkdirAll("/d", 0o755)
	_, _, code := runCmd(t, mfs, "a", "d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/d/a"); err != nil {
		t.Errorf("expected /d/a: %v", err)
	}
}

func TestMvOverwriteForce(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("new"), 0o644)
	_ = mfs.WriteFile("/b", []byte("old"), 0o644)
	_, _, code := runCmd(t, mfs, "-f", "a", "b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, _ := mfs.ReadFile("/b")
	if string(data) != "new" {
		t.Errorf("got %q", data)
	}
}

func TestMvNoClobber(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("new"), 0o644)
	_ = mfs.WriteFile("/b", []byte("old"), 0o644)
	_, _, code := runCmd(t, mfs, "-n", "a", "b")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, _ := mfs.ReadFile("/b")
	if string(data) != "old" {
		t.Errorf("clobbered: %q", data)
	}
}

func TestMvInteractiveRejected(t *testing.T) {
	_, err, code := runCmd(t, memfs.New(), "-i", "a", "b")
	if code == 0 || !strings.Contains(err, "interactive") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestMvHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: mv") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestMvMissingOperand(t *testing.T) {
	_, _, code := runCmd(t, memfs.New())
	if code == 0 {
		t.Fatal("expected error")
	}
}
