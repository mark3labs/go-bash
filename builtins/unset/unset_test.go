package unset

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestUnset(t *testing.T) {
	env := map[string]string{"X": "1"}
	r := New().Execute(context.Background(), []string{"unset", "X"}, &command.Context{Env: env})
	if r.ExitCode != 0 || env["X"] != "" {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestUnsetMissing(t *testing.T) {
	env := map[string]string{}
	r := New().Execute(context.Background(), []string{"unset", "MISSING"}, &command.Context{Env: env})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestUnsetFlags(t *testing.T) {
	env := map[string]string{"X": "1"}
	r := New().Execute(context.Background(), []string{"unset", "-v", "X"}, &command.Context{Env: env})
	if r.ExitCode != 0 || env["X"] != "" {
		t.Errorf("env=%v", env)
	}
}
