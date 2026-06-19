package bash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash/builtins/bash"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	c.Stdout = &o
	c.Stderr = &e
	res := gobash.New().Execute(context.Background(), append([]string{"bash"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestBashDashCRunsScript(t *testing.T) {
	var seenScript string
	c := &command.Context{
		Exec: func(_ context.Context, script string, _ command.SubExecOptions) (command.Result, error) {
			seenScript = script
			return command.Result{ExitCode: 0}, nil
		},
	}
	_, _, code := runCmd(t, c, "-c", "echo hi")
	if code != 0 || seenScript != "echo hi" {
		t.Errorf("script=%q code=%d", seenScript, code)
	}
}

func TestBashDashE(t *testing.T) {
	var seenScript string
	c := &command.Context{
		Exec: func(_ context.Context, script string, _ command.SubExecOptions) (command.Result, error) {
			seenScript = script
			return command.Result{ExitCode: 0}, nil
		},
	}
	_, _, _ = runCmd(t, c, "-e", "-c", "true")
	if !strings.HasPrefix(seenScript, "set -e\n") {
		t.Errorf("script=%q", seenScript)
	}
}

func TestBashBundledFlags(t *testing.T) {
	var seenScript string
	c := &command.Context{
		Exec: func(_ context.Context, script string, _ command.SubExecOptions) (command.Result, error) {
			seenScript = script
			return command.Result{ExitCode: 0}, nil
		},
	}
	_, _, _ = runCmd(t, c, "-ex", "-c", "true")
	if !strings.Contains(seenScript, "set -e") || !strings.Contains(seenScript, "set -x") {
		t.Errorf("script=%q", seenScript)
	}
}

func TestBashDashN(t *testing.T) {
	c := &command.Context{}
	_, _, code := runCmd(t, c, "-n", "-c", "echo hi")
	if code != 0 {
		t.Errorf("code=%d want=0", code)
	}
	_, e, code := runCmd(t, c, "-n", "-c", "echo `unclosed")
	if code != 2 || e == "" {
		t.Errorf("expected parse error: code=%d stderr=%q", code, e)
	}
}

func TestBashDashS(t *testing.T) {
	var seenScript string
	c := &command.Context{
		Stdin: strings.NewReader("echo from stdin"),
		Exec: func(_ context.Context, script string, _ command.SubExecOptions) (command.Result, error) {
			seenScript = script
			return command.Result{ExitCode: 0}, nil
		},
	}
	_, _, _ = runCmd(t, c, "-s")
	if seenScript != "echo from stdin" {
		t.Errorf("script=%q", seenScript)
	}
}

func TestBashScriptFile(t *testing.T) {
	fsys := memfs.New()
	if err := fsys.WriteFile("/s.sh", []byte("echo file"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seenScript string
	c := &command.Context{
		FS:  fsys,
		Cwd: "/",
		Exec: func(_ context.Context, script string, _ command.SubExecOptions) (command.Result, error) {
			seenScript = script
			return command.Result{ExitCode: 0}, nil
		},
	}
	_, _, code := runCmd(t, c, "s.sh")
	if code != 0 || seenScript != "echo file" {
		t.Errorf("script=%q code=%d", seenScript, code)
	}
}

func TestBashMaxSourceDepth(t *testing.T) {
	c := &command.Context{
		SourceDepth: 5,
		Limits:      command.Limits{MaxSourceDepth: 5},
		Exec: func(_ context.Context, _ string, _ command.SubExecOptions) (command.Result, error) {
			t.Fatal("Exec should not be called")
			return command.Result{}, nil
		},
	}
	_, e, code := runCmd(t, c, "-c", "echo x")
	if code != 1 || !strings.Contains(e, "MaxSourceDepth") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestBashDepthBumped(t *testing.T) {
	var seenDepth int
	c := &command.Context{
		SourceDepth: 2,
		Exec: func(_ context.Context, _ string, opts command.SubExecOptions) (command.Result, error) {
			seenDepth = opts.SourceDepth
			return command.Result{ExitCode: 0}, nil
		},
	}
	_, _, _ = runCmd(t, c, "-c", "true")
	if seenDepth != 3 {
		t.Errorf("depth=%d want=3", seenDepth)
	}
}

func TestBashHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: bash") {
		t.Errorf("help out=%q", out)
	}
}

func TestBashNoExecHook(t *testing.T) {
	c := &command.Context{}
	_, e, code := runCmd(t, c, "-c", "x")
	if code != 1 || !strings.Contains(e, "sub-shell exec") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestBashUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-Q")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d", code)
	}
}
