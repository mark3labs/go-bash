package hash

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestHash(t *testing.T) {
	r := New().Execute(context.Background(), []string{"hash"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestHashWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"hash", "a", "b"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestHashHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"hash", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || o.Len() == 0 {
		t.Errorf("exit=%d out=%q", r.ExitCode, o.String())
	}
}
