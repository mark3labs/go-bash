package od_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/od"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := od.New().Execute(context.Background(), append([]string{"od"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestOdHex(t *testing.T) {
	out, _, _ := run(t, "ABC", "-A", "x", "-t", "x1")
	if !strings.Contains(out, "41 42 43") {
		t.Errorf("got %q", out)
	}
}

func TestOdDec(t *testing.T) {
	out, _, _ := run(t, "AB", "-A", "d", "-t", "d1")
	if !strings.Contains(out, "65") || !strings.Contains(out, "66") {
		t.Errorf("got %q", out)
	}
}

func TestOdChar(t *testing.T) {
	out, _, _ := run(t, "a", "-A", "n", "-t", "c")
	if !strings.Contains(out, " a") {
		t.Errorf("got %q", out)
	}
}

func TestOdHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: od") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestOdUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
