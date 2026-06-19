package touch_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/go-bash/builtins/touch"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := touch.New().Execute(context.Background(), append([]string{"touch"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestTouchCreate(t *testing.T) {
	mfs := memfs.New()
	_, _, code := runCmd(t, mfs, "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/a"); err != nil {
		t.Fatalf("expected /a: %v", err)
	}
}

func TestTouchNoCreate(t *testing.T) {
	mfs := memfs.New()
	_, _, code := runCmd(t, mfs, "-c", "missing")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := mfs.Stat("/missing"); err == nil {
		t.Fatal("should not have created file")
	}
}

func TestTouchUpdatesMtime(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	old := time.Now().Add(-time.Hour)
	_ = mfs.Chtimes("/a", old, old)
	_, _, code := runCmd(t, mfs, "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/a")
	if !fi.ModTime().After(old) {
		t.Errorf("mtime not updated: %v", fi.ModTime())
	}
}

func TestTouchStamp(t *testing.T) {
	mfs := memfs.New()
	_, _, code := runCmd(t, mfs, "-t", "202001021530.45", "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/a")
	if fi.ModTime().Year() != 2020 || fi.ModTime().Month() != time.January || fi.ModTime().Day() != 2 {
		t.Errorf("mtime=%v", fi.ModTime())
	}
}

func TestTouchReference(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/ref", []byte("x"), 0o644)
	want := time.Date(2021, 6, 15, 10, 30, 0, 0, time.UTC)
	_ = mfs.Chtimes("/ref", want, want)
	_, _, code := runCmd(t, mfs, "-r", "/ref", "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/a")
	if !fi.ModTime().Equal(want) {
		t.Errorf("mtime=%v want=%v", fi.ModTime(), want)
	}
}

func TestTouchDate(t *testing.T) {
	mfs := memfs.New()
	_, _, code := runCmd(t, mfs, "-d", "2022-03-04", "a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	fi, _ := mfs.Stat("/a")
	if fi.ModTime().Year() != 2022 || fi.ModTime().Month() != time.March {
		t.Errorf("mtime=%v", fi.ModTime())
	}
}

func TestTouchHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: touch") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestTouchUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z", "x")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
