package strings_test

import (
	"bytes"
	"context"
	stdstrings "strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/strings"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := strings.New().Execute(context.Background(), append([]string{"strings"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdin: stdstrings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestStringsRuns(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/bin", []byte("abcd\x00\x01XYZW\x00short\x00"), 0o644)
	out, _, _ := run(t, mfs, "", "/bin")
	if !stdstrings.Contains(out, "abcd") || !stdstrings.Contains(out, "XYZW") || !stdstrings.Contains(out, "short") {
		t.Errorf("got %q", out)
	}
}

func TestStringsMinLen(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/bin", []byte("ab\x00abcdef"), 0o644)
	out, _, _ := run(t, mfs, "", "-n", "4", "/bin")
	if stdstrings.Contains(out, "ab\n") || !stdstrings.Contains(out, "abcdef") {
		t.Errorf("got %q", out)
	}
}

func TestStringsOffsetD(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.WriteFile("/bin", []byte("\x00\x00abcd"), 0o644)
	out, _, _ := run(t, mfs, "", "-t", "d", "/bin")
	if !stdstrings.Contains(out, "abcd") || !stdstrings.Contains(out, "2") {
		t.Errorf("got %q", out)
	}
}

func TestStringsHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "", "--help")
	if code != 0 || !stdstrings.Contains(out, "Usage: strings") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestStringsUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "", "--bogus")
	if code != 2 || !stdstrings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
