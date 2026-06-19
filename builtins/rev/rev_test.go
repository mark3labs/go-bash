package rev_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/rev"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := rev.New().Execute(context.Background(), append([]string{"rev"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestRev(t *testing.T) {
	out, _, _ := run(t, "abc\nxyz\n")
	if out != "cba\nzyx\n" {
		t.Errorf("got %q", out)
	}
}

func TestRevUnicode(t *testing.T) {
	out, _, _ := run(t, "héllo\n")
	if out != "olléh\n" {
		t.Errorf("got %q", out)
	}
}

func TestRevHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: rev") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestRevUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
