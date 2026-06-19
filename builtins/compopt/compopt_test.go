package compopt

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestCompopt(t *testing.T) {
	r := New().Execute(context.Background(), []string{"compopt"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestCompoptWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"compopt", "a", "b"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestCompoptHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"compopt", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || o.Len() == 0 {
		t.Errorf("exit=%d out=%q", r.ExitCode, o.String())
	}
}
