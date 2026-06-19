package expand_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/expand"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := expand.New().Execute(context.Background(), append([]string{"expand"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestExpandDefault(t *testing.T) {
	out, _, _ := run(t, "a\tb\n")
	if out != "a       b\n" {
		t.Errorf("got %q", out)
	}
}

func TestExpandWidth(t *testing.T) {
	out, _, _ := run(t, "a\tb\n", "-t", "4")
	if out != "a   b\n" {
		t.Errorf("got %q", out)
	}
}

func TestExpandInitial(t *testing.T) {
	out, _, _ := run(t, "\tabc\tdef\n", "-i", "-t", "4")
	if out != "    abc\tdef\n" {
		t.Errorf("got %q", out)
	}
}

func TestExpandHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: expand") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestExpandUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
