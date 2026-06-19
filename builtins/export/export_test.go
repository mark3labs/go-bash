package export

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestExportSet(t *testing.T) {
	env := map[string]string{}
	exp := map[string]string{}
	r := New().Execute(context.Background(), []string{"export", "FOO=bar"}, &command.Context{Env: env, ExportedEnv: exp})
	if r.ExitCode != 0 || env["FOO"] != "bar" || exp["FOO"] != "bar" {
		t.Errorf("env=%v exp=%v exit=%d", env, exp, r.ExitCode)
	}
}

func TestExportP(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"export", "-p"}, &command.Context{
		ExportedEnv: map[string]string{"X": "1"},
		Stdout:      &o,
	})
	if r.ExitCode != 0 || !strings.Contains(o.String(), "declare -x X=\"1\"") {
		t.Errorf("out=%q", o.String())
	}
}

func TestExportN(t *testing.T) {
	exp := map[string]string{"A": "1"}
	r := New().Execute(context.Background(), []string{"export", "-n", "A"}, &command.Context{Env: map[string]string{}, ExportedEnv: exp})
	if r.ExitCode != 0 || exp["A"] != "" {
		t.Errorf("exit=%d exp=%v", r.ExitCode, exp)
	}
}
