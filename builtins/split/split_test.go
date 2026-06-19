package split_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/split"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := split.New().Execute(context.Background(), append([]string{"split"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestSplitLines(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/in", []byte("a\nb\nc\nd\n"), 0o644)
	_, _, code := runCmd(t, mfs, "", "-l", "2", "/in", "/out_")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, err := mfs.ReadFile("/out_aa")
	if err != nil || string(data) != "a\nb\n" {
		t.Errorf("out_aa=%q err=%v", data, err)
	}
	data, err = mfs.ReadFile("/out_ab")
	if err != nil || string(data) != "c\nd\n" {
		t.Errorf("out_ab=%q err=%v", data, err)
	}
}

func TestSplitBytes(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/in", []byte("ABCDEFGHIJ"), 0o644)
	_, _, code := runCmd(t, mfs, "", "-b", "3", "/in", "/p")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if data, _ := mfs.ReadFile("/paa"); string(data) != "ABC" {
		t.Errorf("paa=%q", data)
	}
	if data, _ := mfs.ReadFile("/pad"); string(data) != "J" {
		t.Errorf("pad=%q", data)
	}
}

func TestSplitNumeric(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/in", []byte("a\nb\n"), 0o644)
	_, _, code := runCmd(t, mfs, "", "-l", "1", "-d", "/in", "/p")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/p00"); err != nil {
		t.Errorf("expected /p00: %v", err)
	}
}

func TestSplitStdin(t *testing.T) {
	mfs := memfs.New()
	_, _, code := runCmd(t, mfs, "a\nb\n", "-l", "1", "-", "/p")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/paa"); err != nil {
		t.Errorf("expected /paa: %v", err)
	}
}

func TestSplitHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: split") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestSplitUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "", "-Z")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
