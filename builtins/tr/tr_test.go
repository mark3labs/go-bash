package tr_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/tr"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := tr.New().Execute(context.Background(), append([]string{"tr"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestTrTranslate(t *testing.T) {
	out, _, _ := run(t, "abc", "a-c", "x-z")
	if out != "xyz" {
		t.Errorf("got %q", out)
	}
}

func TestTrDelete(t *testing.T) {
	out, _, _ := run(t, "abc123", "-d", "[:digit:]")
	if out != "abc" {
		t.Errorf("got %q", out)
	}
}

func TestTrSqueeze(t *testing.T) {
	out, _, _ := run(t, "aaabbbccc", "-s", "abc")
	if out != "abc" {
		t.Errorf("got %q", out)
	}
}

func TestTrComplement(t *testing.T) {
	out, _, _ := run(t, "abc123", "-d", "-c", "[:alpha:]")
	if out != "abc" {
		t.Errorf("got %q", out)
	}
}

func TestTrUpper(t *testing.T) {
	out, _, _ := run(t, "abc", "[:lower:]", "[:upper:]")
	if out != "ABC" {
		t.Errorf("got %q", out)
	}
}

func TestTrEscape(t *testing.T) {
	out, _, _ := run(t, "a\tb", "\\t", " ")
	if out != "a b" {
		t.Errorf("got %q", out)
	}
}

func TestTrHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: tr") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestTrUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
