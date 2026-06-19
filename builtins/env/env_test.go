package env_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/env"
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
	res := env.New().Execute(context.Background(), append([]string{"env"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestEnvListSorted(t *testing.T) {
	c := &command.Context{Env: map[string]string{"B": "2", "A": "1", "C": "3"}}
	out, _, code := runCmd(t, c)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "A=1\nB=2\nC=3\n"
	if out != want {
		t.Errorf("out=%q want=%q", out, want)
	}
}

func TestEnvIgnoreEnvironment(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1"}}
	out, _, code := runCmd(t, c, "-i")
	if code != 0 || out != "" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestEnvUnset(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1", "B": "2"}}
	out, _, _ := runCmd(t, c, "-u", "A")
	if out != "B=2\n" {
		t.Errorf("out=%q", out)
	}
}

func TestEnvSetAndList(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1"}}
	out, _, _ := runCmd(t, c, "B=2")
	if !strings.Contains(out, "A=1\n") || !strings.Contains(out, "B=2\n") {
		t.Errorf("out=%q", out)
	}
}

func TestEnvIWithSet(t *testing.T) {
	c := &command.Context{Env: map[string]string{"A": "1"}}
	out, _, _ := runCmd(t, c, "-i", "X=Y")
	if out != "X=Y\n" {
		t.Errorf("out=%q", out)
	}
}

func TestEnvNameValueProg(t *testing.T) {
	reg := command.NewRegistry()
	var sawEnv map[string]string
	reg.Register(command.Define("probe", func(_ context.Context, _ []string, c *command.Context) command.Result {
		sawEnv = c.Env
		return command.Result{}
	}))
	c := &command.Context{
		Env:      map[string]string{"A": "1"},
		Registry: reg,
	}
	_, _, code := runCmd(t, c, "B=2", "probe")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if sawEnv["A"] != "1" || sawEnv["B"] != "2" {
		t.Errorf("envseen=%v", sawEnv)
	}
}

func TestEnvProgNotFound(t *testing.T) {
	reg := command.NewRegistry()
	c := &command.Context{Registry: reg}
	_, e, code := runCmd(t, c, "no_such_command")
	if code != 127 {
		t.Errorf("code=%d", code)
	}
	if !strings.Contains(e, "command not found") {
		t.Errorf("stderr=%q", e)
	}
}

func TestEnvHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: env") {
		t.Errorf("help out=%q code=%d", out, code)
	}
}

func TestEnvUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-z")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestEnvDashDash(t *testing.T) {
	c := &command.Context{Env: map[string]string{}}
	out, _, _ := runCmd(t, c, "--", "FOO=BAR")
	if out != "FOO=BAR\n" {
		t.Errorf("out=%q", out)
	}
}

func TestEnvUsesEnvBeforeExportedEnv(t *testing.T) {
	c := &command.Context{
		Env:         map[string]string{"X": "from_env"},
		ExportedEnv: map[string]string{"Y": "from_exported"},
	}
	out, _, _ := runCmd(t, c)
	if !strings.Contains(out, "X=from_env") || strings.Contains(out, "Y=from_exported") {
		t.Errorf("out=%q (should prefer Env over ExportedEnv)", out)
	}
}

func TestEnvFallsBackToExportedEnv(t *testing.T) {
	c := &command.Context{
		ExportedEnv: map[string]string{"Y": "from_exported"},
	}
	out, _, _ := runCmd(t, c)
	if out != "Y=from_exported\n" {
		t.Errorf("out=%q", out)
	}
}
