package eval

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestEvalRuns(t *testing.T) {
	var seenScript string
	c := &command.Context{
		Exec: func(_ context.Context, script string, _ command.SubExecOptions) (command.Result, error) {
			seenScript = script
			return command.Result{ExitCode: 0}, nil
		},
	}
	r := New().Execute(context.Background(), []string{"eval", "echo", "hi"}, c)
	if r.ExitCode != 0 || seenScript != "echo hi" {
		t.Errorf("exit=%d script=%q", r.ExitCode, seenScript)
	}
}

func TestEvalEmpty(t *testing.T) {
	r := New().Execute(context.Background(), []string{"eval"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestEvalNoExec(t *testing.T) {
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"eval", "echo"}, &command.Context{Stderr: &e})
	if r.ExitCode != 1 || !strings.Contains(e.String(), "exec hook") {
		t.Errorf("exit=%d err=%q", r.ExitCode, e.String())
	}
}

func TestEvalMaxDepth(t *testing.T) {
	var e bytes.Buffer
	c := &command.Context{
		SourceDepth: 2, Limits: command.Limits{MaxSourceDepth: 2}, Stderr: &e,
		Exec: func(_ context.Context, _ string, _ command.SubExecOptions) (command.Result, error) {
			t.Fatal("should not call Exec")
			return command.Result{}, nil
		},
	}
	r := New().Execute(context.Background(), []string{"eval", "echo"}, c)
	if r.ExitCode != 1 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
