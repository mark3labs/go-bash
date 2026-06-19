package breakcmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestBreakcmd(t *testing.T) {
	r := New().Execute(context.Background(), []string{"break"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestBreakcmdWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"break", "a", "b"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestBreakcmdHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"break", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || o.Len() == 0 {
		t.Errorf("exit=%d out=%q", r.ExitCode, o.String())
	}
}
