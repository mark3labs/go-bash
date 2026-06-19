package unalias_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/unalias"
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
	res := unalias.New().Execute(context.Background(), append([]string{"unalias"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestUnaliasRemove(t *testing.T) {
	c := &command.Context{Aliases: runtimestate.NewAliasTable()}
	c.Aliases.Set("ll", "ls -l")
	_, _, code := runCmd(t, c, "ll")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, ok := c.Aliases.Get("ll"); ok {
		t.Error("alias not removed")
	}
}

func TestUnaliasMissing(t *testing.T) {
	_, e, code := runCmd(t, nil, "nope")
	if code != 1 || !strings.Contains(e, "not found") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestUnaliasAll(t *testing.T) {
	c := &command.Context{Aliases: runtimestate.NewAliasTable()}
	c.Aliases.Set("ll", "ls -l")
	c.Aliases.Set("la", "ls -A")
	_, _, code := runCmd(t, c, "-a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if names := c.Aliases.Names(); len(names) != 0 {
		t.Errorf("names=%v want=[]", names)
	}
}

func TestUnaliasNoArgs(t *testing.T) {
	_, e, code := runCmd(t, nil)
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d", code)
	}
}

func TestUnaliasHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: unalias") {
		t.Errorf("help out=%q", out)
	}
}

func TestUnaliasUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-z")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d", code)
	}
}
