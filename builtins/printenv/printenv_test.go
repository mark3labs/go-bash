package printenv_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/printenv"
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
	res := printenv.New().Execute(context.Background(), append([]string{"printenv"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestPrintenvAll(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1", "B": "2"}}
	out, _, code := runCmd(t, c)
	if code != 0 || out != "A=1\nB=2\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestPrintenvSpecificName(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1", "B": "2"}}
	out, _, code := runCmd(t, c, "A")
	if code != 0 || out != "1\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestPrintenvMissing(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1"}}
	out, _, code := runCmd(t, c, "MISSING")
	if code != 1 || out != "" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestPrintenvMixed(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1"}}
	out, _, code := runCmd(t, c, "A", "MISSING")
	if code != 1 || out != "1\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestPrintenvNull(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1"}}
	out, _, _ := runCmd(t, c, "-0", "A")
	if out != "1\x00" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintenvHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: printenv") {
		t.Errorf("help out=%q", out)
	}
}

func TestPrintenvUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-z")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d", code)
	}
}

func TestPrintenvPrefersEnv(t *testing.T) {
	c := &command.Context{
		Env:         map[string]string{"X": "1"},
		ExportedEnv: map[string]string{"Y": "2"},
	}
	out, _, _ := runCmd(t, c)
	if out != "X=1\n" {
		t.Errorf("out=%q (should prefer Env over ExportedEnv)", out)
	}
}

func TestPrintenvFallsBackToExportedEnv(t *testing.T) {
	c := &command.Context{
		ExportedEnv: map[string]string{"Y": "2"},
	}
	out, _, _ := runCmd(t, c)
	if out != "Y=2\n" {
		t.Errorf("out=%q", out)
	}
}
