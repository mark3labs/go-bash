package nl_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/nl"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := nl.New().Execute(context.Background(), append([]string{"nl"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestNlDefault(t *testing.T) {
	out, _, _ := run(t, "a\nb\n")
	if !strings.Contains(out, "     1\ta") || !strings.Contains(out, "     2\tb") {
		t.Errorf("got %q", out)
	}
}

func TestNlAll(t *testing.T) {
	out, _, _ := run(t, "a\n\nb\n", "-b", "a")
	if !strings.Contains(out, "     2\t") || !strings.Contains(out, "     3\tb") {
		t.Errorf("got %q", out)
	}
}

func TestNlRZ(t *testing.T) {
	out, _, _ := run(t, "a\n", "-n", "rz", "-w", "3")
	if !strings.Contains(out, "001\ta") {
		t.Errorf("got %q", out)
	}
}

func TestNlSep(t *testing.T) {
	out, _, _ := run(t, "a\n", "-s", " | ")
	if !strings.Contains(out, " | a") {
		t.Errorf("got %q", out)
	}
}

func TestNlHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: nl") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestNlUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
