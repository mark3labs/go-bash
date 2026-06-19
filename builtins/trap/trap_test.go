package trap

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestTrap(t *testing.T) {
	r := New().Execute(context.Background(), []string{"trap"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestTrapWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"trap", "a", "b"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestTrapHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"trap", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || o.Len() == 0 {
		t.Errorf("exit=%d out=%q", r.ExitCode, o.String())
	}
}
