package xargs_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	_ "github.com/mark3labs/go-bash/builtins/echo"
	"github.com/mark3labs/go-bash/builtins/xargs"
	"github.com/mark3labs/go-bash/command"
)

func newReg(t *testing.T) *command.Registry {
	t.Helper()
	r := command.NewRegistry()
	for _, c := range command.DefaultBuiltins() {
		r.Register(c)
	}
	return r
}

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := xargs.New().Execute(context.Background(), append([]string{"xargs"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
		Registry: newReg(t),
	})
	return o.String(), e.String(), res.ExitCode
}

func TestXargsBasic(t *testing.T) {
	out, _, _ := run(t, "a b c\n", "echo")
	if !strings.Contains(out, "a b c") {
		t.Errorf("got %q", out)
	}
}

func TestXargsN(t *testing.T) {
	out, _, _ := run(t, "a b c d\n", "-n", "2", "echo")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d (%q)", len(lines), out)
	}
}

func TestXargsI(t *testing.T) {
	out, _, _ := run(t, "x\ny\n", "-I", "{}", "echo", "[{}]")
	if !strings.Contains(out, "[x]") || !strings.Contains(out, "[y]") {
		t.Errorf("got %q", out)
	}
}

func TestXargsParallel(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\nd\n", "-n", "1", "-P", "2", "echo")
	// All 4 echoes should appear (order is non-deterministic in parallel).
	for _, w := range []string{"a", "b", "c", "d"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %s: %q", w, out)
		}
	}
}

func TestXargsHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: xargs") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestXargsUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
