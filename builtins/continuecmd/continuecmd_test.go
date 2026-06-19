package continuecmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestContinuecmd(t *testing.T) {
	r := New().Execute(context.Background(), []string{"continue"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestContinuecmdWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"continue", "a", "b"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestContinuecmdHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"continue", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || o.Len() == 0 {
		t.Errorf("exit=%d out=%q", r.ExitCode, o.String())
	}
}
