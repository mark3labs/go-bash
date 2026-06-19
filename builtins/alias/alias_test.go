package alias_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/alias"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/runtimestate"
)

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	if c.Aliases == nil {
		c.Aliases = runtimestate.NewAliasTable()
	}
	c.Stdout = &o
	c.Stderr = &e
	res := alias.New().Execute(context.Background(), append([]string{"alias"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestAliasSetAndList(t *testing.T) {
	c := &command.Context{Aliases: runtimestate.NewAliasTable()}
	_, _, code := runCmd(t, c, "ll=ls -l", "la=ls -A")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	out, _, _ := runCmd(t, c)
	want := "alias la='ls -A'\nalias ll='ls -l'\n"
	if out != want {
		t.Errorf("out=%q want=%q", out, want)
	}
}

func TestAliasGetMissing(t *testing.T) {
	_, e, code := runCmd(t, nil, "nope")
	if code != 1 || !strings.Contains(e, "not found") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestAliasGetExisting(t *testing.T) {
	c := &command.Context{Aliases: runtimestate.NewAliasTable()}
	c.Aliases.Set("ll", "ls -l")
	out, _, code := runCmd(t, c, "ll")
	if code != 0 || out != "alias ll='ls -l'\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestAliasQuoteEscaping(t *testing.T) {
	c := &command.Context{Aliases: runtimestate.NewAliasTable()}
	_, _, _ = runCmd(t, c, "x=echo 'hi'")
	out, _, _ := runCmd(t, c, "x")
	want := "alias x='echo '\\''hi'\\'''\n"
	if out != want {
		t.Errorf("out=%q want=%q", out, want)
	}
}

func TestAliasHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: alias") {
		t.Errorf("help out=%q", out)
	}
}

func TestAliasUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-z")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d", code)
	}
}

func TestAliasInvalidName(t *testing.T) {
	_, e, code := runCmd(t, nil, "bad/name=x")
	if code != 1 || !strings.Contains(e, "invalid alias name") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}
