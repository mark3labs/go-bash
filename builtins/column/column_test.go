package column_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/column"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := column.New().Execute(context.Background(), append([]string{"column"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestColumnTable(t *testing.T) {
	out, _, _ := run(t, "name age\nalice 30\nbob 25\n", "-t")
	if !strings.Contains(out, "name") || !strings.Contains(out, "alice") {
		t.Errorf("got %q", out)
	}
	// Ensure alignment (two-space sep)
	if !strings.Contains(out, "name   age") {
		t.Errorf("not aligned: %q", out)
	}
}

func TestColumnSep(t *testing.T) {
	out, _, _ := run(t, "a:b:c\nx:y:z\n", "-t", "-s", ":")
	if !strings.Contains(out, "a  b  c") {
		t.Errorf("got %q", out)
	}
}

func TestColumnHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: column") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestColumnUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
