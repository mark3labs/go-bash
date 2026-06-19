package sort_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/sort"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := sort.New().Execute(context.Background(), append([]string{"sort"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestSortDefault(t *testing.T) {
	out, _, _ := run(t, "c\na\nb\n")
	if out != "a\nb\nc\n" {
		t.Errorf("got %q", out)
	}
}

func TestSortNumeric(t *testing.T) {
	out, _, _ := run(t, "10\n2\n1\n", "-n")
	if out != "1\n2\n10\n" {
		t.Errorf("got %q", out)
	}
}

func TestSortReverse(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\n", "-r")
	if out != "c\nb\na\n" {
		t.Errorf("got %q", out)
	}
}

func TestSortUnique(t *testing.T) {
	out, _, _ := run(t, "a\nb\nb\na\n", "-u")
	if out != "a\nb\n" {
		t.Errorf("got %q", out)
	}
}

func TestSortKey(t *testing.T) {
	out, _, _ := run(t, "a 3\nb 1\nc 2\n", "-k", "2", "-n")
	if out != "b 1\nc 2\na 3\n" {
		t.Errorf("got %q", out)
	}
}

func TestSortField(t *testing.T) {
	out, _, _ := run(t, "a:3\nb:1\nc:2\n", "-t", ":", "-k", "2", "-n")
	if out != "b:1\nc:2\na:3\n" {
		t.Errorf("got %q", out)
	}
}

func TestSortVersion(t *testing.T) {
	out, _, _ := run(t, "v1.10\nv1.2\nv1.1\n", "-V")
	if out != "v1.1\nv1.2\nv1.10\n" {
		t.Errorf("got %q", out)
	}
}

func TestSortHuman(t *testing.T) {
	out, _, _ := run(t, "1M\n1K\n1G\n", "-h")
	if out != "1K\n1M\n1G\n" {
		t.Errorf("got %q", out)
	}
}

func TestSortIgnoreCase(t *testing.T) {
	out, _, _ := run(t, "B\na\nC\n", "-f")
	if !strings.Contains(out, "a") {
		t.Errorf("got %q", out)
	}
}

func TestSortHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: sort") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestSortUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
