package gobash_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gobash "github.com/mark3labs/go-bash"
)

// TestNewZeroOptions covers the Phase 1 acceptance criterion that
// Bash constructs with no options (SPEC §1.4).
func TestNewZeroOptions(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New(BashOptions{}) returned error: %v", err)
	}
	if b == nil {
		t.Fatal("New returned nil *Bash")
	}
}

// TestExecEchoHello covers the Phase 1 acceptance criterion that
// `echo hello` returns Stdout: "hello\n", ExitCode: 0 (SPEC §1.4).
func TestExecEchoHello(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := b.Exec(context.Background(), "echo hello", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if res.Stdout != "hello\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "hello\n")
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", res.ExitCode)
	}
}

// TestExecExitCode covers the Phase 1 acceptance criterion that
// `exit 7` reports ExitCode: 7 with no error (SPEC §1.4).
func TestExecExitCode(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := b.Exec(context.Background(), "exit 7", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("ExitCode = %d; want 7", res.ExitCode)
	}
}

// TestExecMidScriptCancellation covers the Phase 1 acceptance criterion
// that mid-script context cancellation surfaces context.Canceled (SPEC §1.4).
//
// Phase 2 enforces MaxLoopIterations / MaxCommandCount; this test bumps
// them well past anything the busy loop could reach in 50ms so the only
// failure mode is the deliberate ctx cancel.
func TestExecMidScriptCancellation(t *testing.T) {
	huge := 1 << 30
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{
			MaxLoopIterations: &huge,
			MaxCommandCount:   &huge,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	// Tight busy loop; mvdan/sh's interp checks ctx between commands.
	// MaxLoopIterations / MaxCommandCount are bumped above so the loop
	// only terminates on cancellation.
	_, err = b.Exec(ctx, "while true; do :; done", gobash.ExecOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Exec err = %v; want context.Canceled", err)
	}
}

// TestExecStdoutWriterPassthrough confirms that a caller-provided Stdout
// writer bypasses string capture, and that result.Stdout is empty in that
// case. This guards the io.Writer-first stream contract (SPEC §0.1).
func TestExecStdoutWriterPassthrough(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var buf strings.Builder
	res, err := b.Exec(context.Background(), "echo world", gobash.ExecOptions{Stdout: &buf})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if got := buf.String(); got != "world\n" {
		t.Errorf("writer captured %q; want %q", got, "world\n")
	}
	if res.Stdout != "" {
		t.Errorf("Stdout should be empty when caller supplies Stdout writer; got %q", res.Stdout)
	}
}

// TestExecStdinReader confirms stdin is wired from an io.Reader.
func TestExecStdinReader(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := b.Exec(
		context.Background(),
		"read line; echo \"got=$line\"",
		gobash.ExecOptions{Stdin: strings.NewReader("ping\n")},
	)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "got=ping\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "got=ping\n")
	}
}
