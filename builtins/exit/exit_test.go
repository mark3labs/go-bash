package exit

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestExitDefault(t *testing.T) {
	r := New().Execute(context.Background(), []string{"exit"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestExitNumber(t *testing.T) {
	r := New().Execute(context.Background(), []string{"exit", "5"}, &command.Context{})
	if r.ExitCode != 5 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestExitNonNumeric(t *testing.T) {
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"exit", "abc"}, &command.Context{Stderr: &e})
	if r.ExitCode != 2 || !strings.Contains(e.String(), "numeric") {
		t.Errorf("exit=%d err=%q", r.ExitCode, e.String())
	}
}

func TestExitHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"exit", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || !strings.Contains(o.String(), "exit") {
		t.Errorf("help=%q", o.String())
	}
}
