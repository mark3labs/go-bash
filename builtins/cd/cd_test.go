package cd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestCdValidDir(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("/tmp/x", 0o755)
	r := New().Execute(context.Background(), []string{"cd", "/tmp/x"}, &command.Context{FS: fs, Env: map[string]string{}})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestCdMissing(t *testing.T) {
	fs := memfs.New()
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"cd", "/nope"}, &command.Context{FS: fs, Stderr: &e, Env: map[string]string{}})
	if r.ExitCode == 0 || !strings.Contains(e.String(), "cd:") {
		t.Errorf("exit=%d err=%q", r.ExitCode, e.String())
	}
}

func TestCdHome(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("/home/user", 0o755)
	r := New().Execute(context.Background(), []string{"cd"}, &command.Context{FS: fs, Env: map[string]string{"HOME": "/home/user"}})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestCdHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"cd", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || !strings.Contains(o.String(), "cd") {
		t.Errorf("help=%q", o.String())
	}
}
