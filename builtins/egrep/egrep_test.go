package egrep_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/egrep"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := egrep.New().Execute(context.Background(),
		append([]string{"egrep"}, args...),
		&command.Context{Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestExtendedByDefault(t *testing.T) {
	out, _, code := run(t, "foo\nbar\nbaz\n", "ba(r|z)")
	if code != 0 || out != "bar\nbaz\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage:") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}
