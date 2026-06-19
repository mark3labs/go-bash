package wait

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestWait(t *testing.T) {
	r := New().Execute(context.Background(), []string{"wait"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestWaitWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"wait", "1", "2"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
