package sh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/sh"
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
	res := sh.New().Execute(context.Background(), append([]string{"sh"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestShDashC(t *testing.T) {
	var seenScript string
	c := &command.Context{
		Exec: func(_ context.Context, script string, _ command.SubExecOptions) (command.Result, error) {
			seenScript = script
			return command.Result{ExitCode: 0}, nil
		},
	}
	_, _, code := runCmd(t, c, "-c", "true")
	if code != 0 || seenScript != "true" {
		t.Errorf("script=%q code=%d", seenScript, code)
	}
}

func TestShHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: sh") {
		t.Errorf("help out=%q", out)
	}
}
