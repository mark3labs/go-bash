package declare

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestDeclareSet(t *testing.T) {
	env := map[string]string{}
	r := New().Execute(context.Background(), []string{"declare", "X=1"}, &command.Context{Env: env})
	if r.ExitCode != 0 || env["X"] != "1" {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestDeclareX(t *testing.T) {
	env := map[string]string{"X": "1"}
	exp := map[string]string{}
	r := New().Execute(context.Background(), []string{"declare", "-x", "X"}, &command.Context{Env: env, ExportedEnv: exp})
	if r.ExitCode != 0 || exp["X"] != "1" {
		t.Errorf("exp=%v", exp)
	}
}

func TestDeclareNoArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"declare"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
