package jobs

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestJobs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"jobs"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestJobsWithArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"jobs", "a"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
