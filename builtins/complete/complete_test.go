package complete

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestComplete(t *testing.T) {
	r := New().Execute(context.Background(), []string{"complete"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestCompleteWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"complete", "a", "b"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestCompleteHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"complete", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || o.Len() == 0 {
		t.Errorf("exit=%d out=%q", r.ExitCode, o.String())
	}
}
