package source

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestSourceBasic(t *testing.T) {
	fs := memfs.New()
	_ = fs.WriteFile("/s.sh", []byte("echo hi"), 0o644)
	var seenScript string
	c := &command.Context{
		FS: fs, Cwd: "/",
		Exec: func(_ context.Context, script string, _ command.SubExecOptions) (command.Result, error) {
			seenScript = script
			return command.Result{ExitCode: 0}, nil
		},
	}
	r := NewSource().Execute(context.Background(), []string{"source", "s.sh"}, c)
	if r.ExitCode != 0 || seenScript != "echo hi" {
		t.Errorf("exit=%d script=%q", r.ExitCode, seenScript)
	}
}

func TestSourceMissingFile(t *testing.T) {
	fs := memfs.New()
	var e bytes.Buffer
	c := &command.Context{FS: fs, Cwd: "/", Stderr: &e,
		Exec: func(_ context.Context, _ string, _ command.SubExecOptions) (command.Result, error) {
			return command.Result{}, nil
		}}
	r := NewSource().Execute(context.Background(), []string{"source", "nope"}, c)
	if r.ExitCode == 0 {
		t.Errorf("expected non-zero")
	}
}

func TestSourceMaxDepth(t *testing.T) {
	fs := memfs.New()
	_ = fs.WriteFile("/s.sh", []byte("echo"), 0o644)
	var e bytes.Buffer
	c := &command.Context{
		FS: fs, Cwd: "/", Stderr: &e,
		SourceDepth: 3, Limits: command.Limits{MaxSourceDepth: 3},
		Exec: func(_ context.Context, _ string, _ command.SubExecOptions) (command.Result, error) {
			t.Fatal("Exec should not be called")
			return command.Result{}, nil
		},
	}
	r := NewSource().Execute(context.Background(), []string{"source", "s.sh"}, c)
	if r.ExitCode != 1 || !strings.Contains(e.String(), "MaxSourceDepth") {
		t.Errorf("exit=%d err=%q", r.ExitCode, e.String())
	}
}

func TestSourceBumpsDepth(t *testing.T) {
	fs := memfs.New()
	_ = fs.WriteFile("/s.sh", []byte("echo"), 0o644)
	var seenDepth int
	c := &command.Context{
		FS: fs, Cwd: "/", SourceDepth: 1,
		Exec: func(_ context.Context, _ string, opts command.SubExecOptions) (command.Result, error) {
			seenDepth = opts.SourceDepth
			return command.Result{ExitCode: 0}, nil
		},
	}
	_ = NewSource().Execute(context.Background(), []string{"source", "s.sh"}, c)
	if seenDepth != 2 {
		t.Errorf("depth=%d want=2", seenDepth)
	}
}

func TestDotAlias(t *testing.T) {
	if NewDot().Name() != "." {
		t.Errorf("name=%v", NewDot().Name())
	}
}
