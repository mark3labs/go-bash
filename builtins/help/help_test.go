package help_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/help"
	"github.com/mark3labs/go-bash/command"
)

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	c.Stdout = &o
	c.Stderr = &e
	res := help.New().Execute(context.Background(), append([]string{"help"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestHelpListsRegistry(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(command.Define("foo", nil))
	reg.Register(command.Define("bar", nil))
	c := &command.Context{Registry: reg}
	out, _, code := runCmd(t, c)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "bar\nfoo\n" {
		t.Errorf("out=%q want=bar\\nfoo\\n", out)
	}
}

func TestHelpDescriptions(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(command.Define("x", nil))
	c := &command.Context{Registry: reg}
	out, _, _ := runCmd(t, c, "-d")
	if !strings.Contains(out, "x") || !strings.Contains(out, "built-in command") {
		t.Errorf("out=%q", out)
	}
}

func TestHelpNamesFound(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(command.Define("x", nil))
	c := &command.Context{Registry: reg}
	out, _, code := runCmd(t, c, "x")
	if code != 0 || out != "x\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestHelpNamesMissing(t *testing.T) {
	reg := command.NewRegistry()
	c := &command.Context{Registry: reg}
	_, e, code := runCmd(t, c, "nope")
	if code != 1 || !strings.Contains(e, "no help topics") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestHelpNoRegistry(t *testing.T) {
	c := &command.Context{}
	_, e, code := runCmd(t, c)
	if code != 1 || !strings.Contains(e, "no registry") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestHelpHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: help") {
		t.Errorf("help out=%q", out)
	}
}

func TestHelpUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-z")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}
