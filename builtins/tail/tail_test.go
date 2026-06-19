package tail_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/tail"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := tail.New().Execute(context.Background(), append([]string{"tail"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestTailDefault(t *testing.T) {
	mfs := memfs.New()
	in := ""
	for i := 1; i <= 20; i++ {
		in += "L\n"
	}
	out, _, _ := run(t, mfs, in)
	if strings.Count(out, "\n") != 10 {
		t.Errorf("expected 10 lines: %q", out)
	}
}

func TestTailN(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "a\nb\nc\nd\n", "-n", "2")
	if out != "c\nd\n" {
		t.Errorf("got %q", out)
	}
}

func TestTailFromLine(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "a\nb\nc\nd\n", "-n", "+2")
	if out != "b\nc\nd\n" {
		t.Errorf("got %q", out)
	}
}

func TestTailBytes(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "abcdef", "-c", "3")
	if out != "def" {
		t.Errorf("got %q", out)
	}
}

func TestTailFollowRejected(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "", "-f", "/no")
	if code != 1 || !strings.Contains(e, "tail -f not supported in sandbox") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}

func TestTailHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tail") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestTailUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
