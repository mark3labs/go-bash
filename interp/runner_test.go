package interp_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	mvinterp "mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/mark3labs/go-bash/command"
	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/memfs"
	bashinterp "github.com/mark3labs/go-bash/interp"
)

// parseFile parses src via mvdan/sh's syntax package — the runner only
// consumes *syntax.File so the gobash/parser layer is not needed here.
func parseFile(t *testing.T, src string) *syntax.File {
	t.Helper()
	f, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

// runOnFS is a tiny harness: build the runner with the given FS and run
// src against it, returning stdout and the runner exit.
func runOnFS(t *testing.T, fs gbfs.FileSystem, src string) (string, int) {
	t.Helper()
	var stdout bytes.Buffer
	r, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
		Cwd:    "/",
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: io.Discard,
		FS:     fs,
	})
	if err != nil {
		t.Fatalf("BuildRunner: %v", err)
	}
	runErr := r.Run(context.Background(), parseFile(t, src))
	exit := 0
	if runErr != nil {
		var status mvinterp.ExitStatus
		if errors.As(runErr, &status) {
			exit = int(status)
		} else {
			t.Fatalf("Run: %v", runErr)
		}
	}
	return stdout.String(), exit
}

// TestBuildRunnerRequiresFS guards against silent fall-throughs to the
// host disk when the caller forgets to wire in a VFS.
func TestBuildRunnerRequiresFS(t *testing.T) {
	_, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	if err == nil || !strings.Contains(err.Error(), "FS is required") {
		t.Fatalf("expected FS-required error, got %v", err)
	}
}

// TestBuildRunnerRequiresStdoutStderr guards against silent nil-writer
// crashes inside mvdan/sh.
func TestBuildRunnerRequiresStdoutStderr(t *testing.T) {
	_, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
		FS: memfs.New(),
	})
	if err == nil {
		t.Fatal("expected error for nil Stdout/Stderr")
	}
}

// TestBuildRunnerWiresVFSOpen demonstrates the OpenHandler routes a
// `>` redirect through the VFS — no host file is created.
func TestBuildRunnerWiresVFSOpen(t *testing.T) {
	fs := memfs.New()
	if err := fs.MkdirAll("/tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, exit := runOnFS(t, fs, `echo hi > /tmp/out`)
	_ = stdout
	_ = exit
	got, err := fs.ReadFile("/tmp/out")
	if err != nil {
		t.Fatalf("VFS ReadFile: %v", err)
	}
	if string(got) != "hi\n" {
		t.Errorf("VFS contents = %q; want %q", got, "hi\n")
	}
}

// TestBuildRunnerCwdWithoutHostStat is the Phase 3 quirk regression:
// runner.Dir must accept a VFS-only path even when that path does not
// exist on the host. mvdan/sh's interp.Dir() helper would os.Stat the
// path on host disk and fail; BuildRunner sets runner.Dir directly to
// bypass that.
func TestBuildRunnerCwdWithoutHostStat(t *testing.T) {
	fs := memfs.New()
	if err := fs.MkdirAll("/nonexistent/on/host", 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
		Cwd:    "/nonexistent/on/host",
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     fs,
	})
	if err != nil {
		t.Fatalf("BuildRunner: %v", err)
	}
	if r.Dir != "/nonexistent/on/host" {
		t.Errorf("runner.Dir = %q; want %q", r.Dir, "/nonexistent/on/host")
	}
}

// TestHandlerDirAbsentReturnsEmpty guards the recover-protected
// HandlerCtx extraction: calling it on a bare ctx must not panic and
// must return empty string.
func TestHandlerDirAbsentReturnsEmpty(t *testing.T) {
	if dir := bashinterp.HandlerDir(context.Background()); dir != "" {
		t.Errorf("HandlerDir(bare ctx) = %q; want %q", dir, "")
	}
}

// TestBuildRunnerCancellation confirms the ctx threaded into runner.Run
// causes a busy loop to terminate via context.Canceled.
func TestBuildRunnerCancellation(t *testing.T) {
	fs := memfs.New()
	r, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
		Stdout: io.Discard,
		Stderr: io.Discard,
		FS:     fs,
	})
	if err != nil {
		t.Fatalf("BuildRunner: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled so runner.Run sees ctx.Err() immediately
	runErr := r.Run(ctx, parseFile(t, "while true; do :; done"))
	if !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run err = %v; want context.Canceled", runErr)
	}
}

// TestRegistryDispatchHit covers the registry-dispatch happy path at
// the interp layer: a registered command is invoked and its stdout
// reaches the script's stdout.
func TestRegistryDispatchHit(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(command.Define("hi", func(_ context.Context, _ []string, c *command.Context) command.Result {
		_, _ = c.Stdout.Write([]byte("hello\n"))
		return command.Result{}
	}))
	var stdout bytes.Buffer
	r, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
		Stdin:    strings.NewReader(""),
		Stdout:   &stdout,
		Stderr:   io.Discard,
		FS:       memfs.New(),
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("BuildRunner: %v", err)
	}
	if err := r.Run(context.Background(), parseFile(t, "hi")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.String() != "hello\n" {
		t.Errorf("stdout = %q; want %q", stdout.String(), "hello\n")
	}
}

// TestRegistryDispatchMissNoOSExec is the sandbox regression at the
// interp layer: with a nil registry an unregistered command must NOT
// invoke os/exec; it must produce ExitStatus(127) and write `command
// not found` to stderr.
func TestRegistryDispatchMissNoOSExec(t *testing.T) {
	var stderr bytes.Buffer
	r, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		Stderr: &stderr,
		FS:     memfs.New(),
		// Registry intentionally nil — every non-mvdan-builtin
		// command must resolve to "command not found" without
		// reaching host os/exec.
	})
	if err != nil {
		t.Fatalf("BuildRunner: %v", err)
	}
	runErr := r.Run(context.Background(), parseFile(t, "definitely_no_such_command_99"))
	var status mvinterp.ExitStatus
	if !errors.As(runErr, &status) {
		t.Fatalf("Run err = %v; want ExitStatus", runErr)
	}
	if int(status) != 127 {
		t.Errorf("exit = %d; want 127", int(status))
	}
	if !strings.Contains(stderr.String(), "definitely_no_such_command_99: command not found") {
		t.Errorf("stderr = %q; want command-not-found diagnostic", stderr.String())
	}
	if strings.Contains(stderr.String(), "executable file not found in $PATH") {
		t.Errorf("stderr leaked mvdan/sh's DefaultExecHandler diagnostic: %q", stderr.String())
	}
}

// TestRegistryDispatchAbsoluteBinPath verifies the /bin/<name>
// basename-resolution branch in lookupCommand.
func TestRegistryDispatchAbsoluteBinPath(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(command.Define("abs", func(_ context.Context, _ []string, c *command.Context) command.Result {
		_, _ = c.Stdout.Write([]byte("ok\n"))
		return command.Result{}
	}))
	var stdout bytes.Buffer
	r, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
		Stdin:    strings.NewReader(""),
		Stdout:   &stdout,
		Stderr:   io.Discard,
		FS:       memfs.New(),
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("BuildRunner: %v", err)
	}
	if err := r.Run(context.Background(), parseFile(t, "/usr/bin/abs")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.String() != "ok\n" {
		t.Errorf("stdout = %q; want %q", stdout.String(), "ok\n")
	}
}

// TestRegistryDispatchExitCode verifies clampExit produces sensible
// ExitStatus values for non-zero ExitCode.
func TestRegistryDispatchExitCode(t *testing.T) {
	reg := command.NewRegistry()
	reg.Register(command.Define("fail", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{ExitCode: 7}
	}))
	reg.Register(command.Define("weird", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{ExitCode: 999} // out-of-range → clamped to 1
	}))
	check := func(script string, want int) {
		r, err := bashinterp.BuildRunner(context.Background(), bashinterp.Config{
			Stdin:    strings.NewReader(""),
			Stdout:   io.Discard,
			Stderr:   io.Discard,
			FS:       memfs.New(),
			Registry: reg,
		})
		if err != nil {
			t.Fatalf("BuildRunner: %v", err)
		}
		runErr := r.Run(context.Background(), parseFile(t, script))
		var status mvinterp.ExitStatus
		if !errors.As(runErr, &status) {
			t.Fatalf("%s: Run err = %v; want ExitStatus", script, runErr)
		}
		if int(status) != want {
			t.Errorf("%s: exit = %d; want %d", script, int(status), want)
		}
	}
	check("fail", 7)
	check("weird", 1)
}
