package returncmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestReturn(t *testing.T) {
	r := New().Execute(context.Background(), []string{"return", "4"}, &command.Context{})
	if r.ExitCode != 4 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestReturnDefault(t *testing.T) {
	r := New().Execute(context.Background(), []string{"return"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestReturnNonNumeric(t *testing.T) {
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"return", "x"}, &command.Context{Stderr: &e})
	if r.ExitCode != 2 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
