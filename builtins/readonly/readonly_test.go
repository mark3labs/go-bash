package readonly

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestReadonlyAssign(t *testing.T) {
	env := map[string]string{}
	r := New().Execute(context.Background(), []string{"readonly", "X=1"}, &command.Context{Env: env})
	if r.ExitCode != 0 || env["X"] != "1" {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestReadonlyPrint(t *testing.T) {
	r := New().Execute(context.Background(), []string{"readonly", "-p"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
