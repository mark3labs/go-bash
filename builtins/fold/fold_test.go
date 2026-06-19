package fold_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/fold"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := fold.New().Execute(context.Background(), append([]string{"fold"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestFoldWidth(t *testing.T) {
	out, _, _ := run(t, "abcdef\n", "-w", "3")
	if out != "abc\ndef\n" {
		t.Errorf("got %q", out)
	}
}

func TestFoldSpaces(t *testing.T) {
	out, _, _ := run(t, "hello world abc\n", "-w", "7", "-s")
	if !strings.Contains(out, "hello ") {
		t.Errorf("got %q", out)
	}
}

func TestFoldHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: fold") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestFoldUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
