package uniq_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/uniq"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := uniq.New().Execute(context.Background(), append([]string{"uniq"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestUniqBasic(t *testing.T) {
	out, _, _ := run(t, "a\na\nb\nb\nc\n")
	if out != "a\nb\nc\n" {
		t.Errorf("got %q", out)
	}
}

func TestUniqCount(t *testing.T) {
	out, _, _ := run(t, "a\na\nb\n", "-c")
	if !strings.Contains(out, "2 a") || !strings.Contains(out, "1 b") {
		t.Errorf("got %q", out)
	}
}

func TestUniqDup(t *testing.T) {
	out, _, _ := run(t, "a\na\nb\nc\nc\n", "-d")
	if out != "a\nc\n" {
		t.Errorf("got %q", out)
	}
}

func TestUniqUnique(t *testing.T) {
	out, _, _ := run(t, "a\na\nb\nc\nc\n", "-u")
	if out != "b\n" {
		t.Errorf("got %q", out)
	}
}

func TestUniqIgnoreCase(t *testing.T) {
	out, _, _ := run(t, "a\nA\nb\n", "-i")
	if out != "a\nb\n" {
		t.Errorf("got %q", out)
	}
}

func TestUniqHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: uniq") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestUniqUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
