package colon

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestColon(t *testing.T) {
	r := New().Execute(context.Background(), []string{":"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestColonIgnoresArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{":", "a", "b"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
