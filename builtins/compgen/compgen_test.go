package compgen

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestCompgen(t *testing.T) {
	r := New().Execute(context.Background(), []string{"compgen"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestCompgenWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"compgen", "a", "b"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestCompgenHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"compgen", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || o.Len() == 0 {
		t.Errorf("exit=%d out=%q", r.ExitCode, o.String())
	}
}
