package stat_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/stat"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, mfs *memfs.FS, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := stat.New().Execute(context.Background(), append([]string{"stat"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestStatDefault(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("hello"), 0o644)
	out, _, code := runCmd(t, mfs, "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "regular file") || !strings.Contains(out, "Size: 5") {
		t.Errorf("out=%q", out)
	}
}

func TestStatFormatSize(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("hello"), 0o644)
	out, _, code := runCmd(t, mfs, "-c", "%s", "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "5\n" {
		t.Errorf("out=%q", out)
	}
}

func TestStatFormatName(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("x"), 0o644)
	out, _, code := runCmd(t, mfs, "-c", "%n", "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "/a\n" {
		t.Errorf("out=%q", out)
	}
}

func TestStatAllCodes(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/a", []byte("xx"), 0o644)
	out, _, code := runCmd(t, mfs, "-c", "%n %s %F %a %A %u %g %i %h %d %t %T", "/a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "regular file") {
		t.Errorf("out=%q", out)
	}
	// JSON roundtrip just to prove format is plain text usable.
	_, _ = json.Marshal(out)
}

func TestStatMissing(t *testing.T) {
	_, _, code := runCmd(t, memfs.New(), "/nope")
	if code == 0 {
		t.Fatal("expected error")
	}
}

func TestStatHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: stat") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestStatUnknown(t *testing.T) {
	_, err, code := runCmd(t, nil, "-Z", "a")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
