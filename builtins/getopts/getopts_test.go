package getopts

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestGetoptsNoOpts(t *testing.T) {
	r := New().Execute(context.Background(), []string{"getopts", "ab:", "OPT"}, &command.Context{})
	if r.ExitCode != 1 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestGetoptsUsage(t *testing.T) {
	r := New().Execute(context.Background(), []string{"getopts"}, &command.Context{})
	if r.ExitCode != 2 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
