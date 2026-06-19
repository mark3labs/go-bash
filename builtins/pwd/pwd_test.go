package pwd_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/pwd"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestPwdLogical(t *testing.T) {
	var out bytes.Buffer
	res := pwd.New().Execute(context.Background(), []string{"pwd"}, &command.Context{Cwd: "/home/user", Stdout: &out})
	if res.ExitCode != 0 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if out.String() != "/home/user\n" {
		t.Errorf("got %q", out.String())
	}
}

func TestPwdLogicalFlag(t *testing.T) {
	var out bytes.Buffer
	res := pwd.New().Execute(context.Background(), []string{"pwd", "-L"}, &command.Context{Cwd: "/tmp", Stdout: &out})
	if res.ExitCode != 0 || out.String() != "/tmp\n" {
		t.Errorf("res=%+v out=%q", res, out.String())
	}
}

func TestPwdPhysicalResolvesSymlink(t *testing.T) {
	fs := memfs.New()
	if err := fs.MkdirAll("/real/dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fs.Symlink("/real/dir", "/link"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	res := pwd.New().Execute(context.Background(), []string{"pwd", "-P"}, &command.Context{FS: fs, Cwd: "/link", Stdout: &out})
	if res.ExitCode != 0 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if !strings.Contains(out.String(), "/real/dir") {
		t.Errorf("expected resolved /real/dir, got %q", out.String())
	}
}

func TestPwdUnknownOption(t *testing.T) {
	var err bytes.Buffer
	res := pwd.New().Execute(context.Background(), []string{"pwd", "-Z"}, &command.Context{Stderr: &err})
	if res.ExitCode != 2 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if !strings.Contains(err.String(), "usage:") {
		t.Errorf("stderr = %q", err.String())
	}
}

func TestPwdHelp(t *testing.T) {
	var out bytes.Buffer
	res := pwd.New().Execute(context.Background(), []string{"pwd", "--help"}, &command.Context{Stdout: &out})
	if res.ExitCode != 0 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if !strings.Contains(out.String(), "Usage: pwd") {
		t.Errorf("help missing: %q", out.String())
	}
}
