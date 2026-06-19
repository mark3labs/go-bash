package cut_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/cut"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, mfs *memfs.FS, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := cut.New().Execute(context.Background(), append([]string{"cut"}, args...), &command.Context{
		FS: mfs, Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestCutFields(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "a:b:c\nd:e:f\n", "-d", ":", "-f", "1,3")
	if out != "a:c\nd:f\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutFieldRange(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "a b c d\n", "-d", " ", "-f", "2-3")
	if out != "b c\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutOpenLo(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "1,2,3,4\n", "-d", ",", "-f", "-2")
	if out != "1,2\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutOpenHi(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "1,2,3,4\n", "-d", ",", "-f", "3-")
	if out != "3,4\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutChars(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "abcdef\n", "-c", "2-4")
	if out != "bcd\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutBytes(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "abcdef\n", "-b", "1,3,5")
	if out != "ace\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutOnlyDelim(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "no-delim-line\nA:B\n", "-d", ":", "-f", "1", "-s")
	if out != "A\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutComplement(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "abcde\n", "-c", "2", "--complement")
	if out != "acde\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutOutputDelim(t *testing.T) {
	mfs := memfs.New()
	out, _, _ := run(t, mfs, "a:b:c\n", "-d", ":", "-f", "1,2", "--output-delimiter=,")
	if out != "a,b\n" {
		t.Errorf("got %q", out)
	}
}

func TestCutHelp(t *testing.T) {
	mfs := memfs.New()
	out, _, code := run(t, mfs, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: cut") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestCutUnknown(t *testing.T) {
	mfs := memfs.New()
	_, e, code := run(t, mfs, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
