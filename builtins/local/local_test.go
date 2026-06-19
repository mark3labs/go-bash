package local

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestLocalAssign(t *testing.T) {
	env := map[string]string{}
	r := New().Execute(context.Background(), []string{"local", "X=1"}, &command.Context{Env: env})
	if r.ExitCode != 0 || env["X"] != "1" {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestLocalNoArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"local"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
