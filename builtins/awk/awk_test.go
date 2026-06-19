package awk_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/awk"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := awk.New().Execute(context.Background(),
		append([]string{"awk"}, args...),
		&command.Context{Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestBasic(t *testing.T) {
	out, _, code := run(t, "1 2\n3 4\n", "{ print $2 }")
	if code != 0 || out != "2\n4\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestFieldSep(t *testing.T) {
	out, _, _ := run(t, "a,b,c\n", "-F", ",", "{ print $2 }")
	if out != "b\n" {
		t.Errorf("got %q", out)
	}
}

func TestVar(t *testing.T) {
	out, _, _ := run(t, "x\n", "-v", "name=world", "{ print \"hello\", name }")
	if out != "hello world\n" {
		t.Errorf("got %q", out)
	}
}

func TestBegin(t *testing.T) {
	out, _, _ := run(t, "", "BEGIN { print \"hi\" }")
	if out != "hi\n" {
		t.Errorf("got %q", out)
	}
}

func TestSum(t *testing.T) {
	out, _, _ := run(t, "1\n2\n3\n", "{ s += $1 } END { print s }")
	if out != "6\n" {
		t.Errorf("got %q", out)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: awk") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}

func TestNoExec(t *testing.T) {
	// system() should fail because NoExec is set.
	_, e, code := run(t, "", "BEGIN { system(\"true\") }")
	if code == 0 {
		t.Errorf("expected exec to be blocked, got exit 0 stderr=%q", e)
	}
}
