package unexpand_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/unexpand"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := unexpand.New().Execute(context.Background(), append([]string{"unexpand"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestUnexpandLeading(t *testing.T) {
	out, _, _ := run(t, "        abc\n")
	if out != "\tabc\n" {
		t.Errorf("got %q", out)
	}
}

func TestUnexpandAll(t *testing.T) {
	out, _, _ := run(t, "a       b\n", "-a")
	if !strings.Contains(out, "\t") {
		t.Errorf("got %q", out)
	}
}

func TestUnexpandHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: unexpand") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestUnexpandUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
