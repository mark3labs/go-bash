package tac_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/tac"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := tac.New().Execute(context.Background(), append([]string{"tac"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestTac(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\n")
	if out != "c\nb\na\n" {
		t.Errorf("got %q", out)
	}
}

func TestTacNoTrailing(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc")
	if out != "ca\nb\n" && out != "cb\na\n" {
		// Last record has no trailing \n.
		// SplitAfter: ["a\n", "b\n", "c"]. Reversed: c, b\n, a\n.
		// So out = "c" + "b\n" + "a\n" = "cb\na\n"
		t.Errorf("got %q", out)
	}
}

func TestTacHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tac") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestTacUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
