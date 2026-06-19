package dirname_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/dirname"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := dirname.New().Execute(context.Background(), append([]string{"dirname"}, args...), &command.Context{Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestDirnameSimple(t *testing.T) {
	out, _, _ := run(t, "/foo/bar.txt")
	if out != "/foo\n" {
		t.Errorf("out=%q", out)
	}
}

func TestDirnameNoSlash(t *testing.T) {
	out, _, _ := run(t, "file.txt")
	if out != ".\n" {
		t.Errorf("out=%q", out)
	}
}

func TestDirnameRoot(t *testing.T) {
	out, _, _ := run(t, "/")
	if out != "/\n" {
		t.Errorf("out=%q", out)
	}
}

func TestDirnameTrailing(t *testing.T) {
	out, _, _ := run(t, "/foo/bar/")
	if out != "/foo\n" {
		t.Errorf("out=%q", out)
	}
}

func TestDirnameOneLevel(t *testing.T) {
	out, _, _ := run(t, "/foo")
	if out != "/\n" {
		t.Errorf("out=%q", out)
	}
}

func TestDirnameMultiple(t *testing.T) {
	out, _, _ := run(t, "/a/b", "/c/d")
	if out != "/a\n/c\n" {
		t.Errorf("out=%q", out)
	}
}

func TestDirnameZero(t *testing.T) {
	out, _, _ := run(t, "-z", "/x/y")
	if out != "/x\x00" {
		t.Errorf("out=%q", out)
	}
}

func TestDirnameMissingOperand(t *testing.T) {
	_, err, code := run(t)
	if code != 1 || !strings.Contains(err, "missing") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestDirnameHelp(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: dirname") {
		t.Errorf("out=%q code=%d", out, code)
	}
}
