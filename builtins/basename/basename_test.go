package basename_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/basename"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := basename.New().Execute(context.Background(), append([]string{"basename"}, args...), &command.Context{Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestBasenameSimple(t *testing.T) {
	out, _, code := run(t, "/foo/bar.txt")
	if code != 0 || out != "bar.txt\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestBasenameSuffix(t *testing.T) {
	out, _, _ := run(t, "/foo/bar.txt", ".txt")
	if out != "bar\n" {
		t.Errorf("out=%q", out)
	}
}

func TestBasenameNoStripWhenEqual(t *testing.T) {
	out, _, _ := run(t, ".txt", ".txt")
	if out != ".txt\n" {
		t.Errorf("out=%q", out)
	}
}

func TestBasenameTrailingSlash(t *testing.T) {
	out, _, _ := run(t, "/foo/bar/")
	if out != "bar\n" {
		t.Errorf("out=%q", out)
	}
}

func TestBasenameRoot(t *testing.T) {
	out, _, _ := run(t, "/")
	if out != "/\n" {
		t.Errorf("out=%q", out)
	}
}

func TestBasenameMultipleA(t *testing.T) {
	out, _, _ := run(t, "-a", "/x/a", "/y/b")
	if out != "a\nb\n" {
		t.Errorf("out=%q", out)
	}
}

func TestBasenameSuffixS(t *testing.T) {
	out, _, _ := run(t, "-s", ".log", "app.log", "sys.log")
	if out != "app\nsys\n" {
		t.Errorf("out=%q", out)
	}
}

func TestBasenameZero(t *testing.T) {
	out, _, _ := run(t, "-z", "/x/a")
	if out != "a\x00" {
		t.Errorf("out=%q", out)
	}
}

func TestBasenameMissingOperand(t *testing.T) {
	_, err, code := run(t)
	if code != 1 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(err, "missing operand") {
		t.Errorf("stderr = %q", err)
	}
}

func TestBasenameUnknownFlag(t *testing.T) {
	_, err, code := run(t, "-Q", "x")
	if code != 2 {
		t.Errorf("code = %d", code)
	}
	if !strings.Contains(err, "usage:") {
		t.Errorf("stderr = %q", err)
	}
}

func TestBasenameHelp(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: basename") {
		t.Errorf("out=%q code=%d", out, code)
	}
}
