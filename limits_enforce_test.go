package gobash_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
)

func intPtr(v int) *int { return &v }

// TestMaxLoopIterations covers SPEC §2.4: `while true; do :; done` aborts
// with *ExecutionLimitError{Limit:"MaxLoopIterations"} — not via context
// cancellation.
func TestMaxLoopIterations(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{
			// Small loop budget so the test runs fast. MaxCommandCount
			// is left at the default (10000) so it never trips first.
			MaxLoopIterations: intPtr(50),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = b.Exec(context.Background(), "while true; do :; done", gobash.ExecOptions{})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Exec err is a context sentinel; want *ExecutionLimitError: %v", err)
	}
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) {
		t.Fatalf("Exec err = %v (%T); want *gobash.ExecutionLimitError", err, err)
	}
	if ele.Limit != "MaxLoopIterations" {
		t.Errorf("Limit = %q; want %q", ele.Limit, "MaxLoopIterations")
	}
	if ele.Value != 50 {
		t.Errorf("Value = %d; want 50", ele.Value)
	}
}

// TestMaxLoopIterationsForLoop confirms `for`-style loops are also counted.
func TestMaxLoopIterationsForLoop(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{
			MaxLoopIterations: intPtr(3),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = b.Exec(context.Background(), "for i in 1 2 3 4 5; do :; done", gobash.ExecOptions{})
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) {
		t.Fatalf("Exec err = %v; want *gobash.ExecutionLimitError", err)
	}
	if ele.Limit != "MaxLoopIterations" {
		t.Errorf("Limit = %q; want MaxLoopIterations", ele.Limit)
	}
}

// TestMaxOutputSize covers SPEC §2.4: writing more than MaxOutputSize bytes
// to stdout aborts with *ExecutionLimitError{Limit:"MaxOutputSize"}.
func TestMaxOutputSize(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{
			MaxOutputSize: intPtr(64),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Emit ~200 bytes via printf's width spec to overshoot the 64-byte cap.
	script := "printf '%200s' x"
	_, err = b.Exec(context.Background(), script, gobash.ExecOptions{})
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) {
		t.Fatalf("Exec err = %v; want *gobash.ExecutionLimitError", err)
	}
	if ele.Limit != "MaxOutputSize" {
		t.Errorf("Limit = %q; want MaxOutputSize", ele.Limit)
	}
	if ele.Value != 64 {
		t.Errorf("Value = %d; want 64", ele.Value)
	}
}

// TestMaxOutputSizeCountsStderr confirms stderr writes share the same
// budget as stdout — total bytes across both streams is what's capped.
func TestMaxOutputSizeCountsStderr(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{
			MaxOutputSize: intPtr(32),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Write to stderr (>&2) to ensure the stderr writer is also wrapped.
	script := "printf '%100s' x >&2"
	_, err = b.Exec(context.Background(), script, gobash.ExecOptions{})
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) {
		t.Fatalf("Exec err = %v; want *gobash.ExecutionLimitError", err)
	}
	if ele.Limit != "MaxOutputSize" {
		t.Errorf("Limit = %q; want MaxOutputSize", ele.Limit)
	}
}

// TestMaxCallDepth covers SPEC §2.4: a deeply recursive shell function
// aborts with *ExecutionLimitError{Limit:"MaxCallDepth"}.
func TestMaxCallDepth(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{
			MaxCallDepth: intPtr(10),
			// Leave MaxCommandCount at default so it doesn't trip first.
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Plain infinite recursion. We expect MaxCallDepth to trip well before
	// either the Go goroutine stack explodes or MaxCommandCount catches up.
	script := "f() { f; }; f"
	_, err = b.Exec(context.Background(), script, gobash.ExecOptions{})
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) {
		t.Fatalf("Exec err = %v; want *gobash.ExecutionLimitError", err)
	}
	if ele.Limit != "MaxCallDepth" {
		t.Errorf("Limit = %q; want MaxCallDepth", ele.Limit)
	}
	if ele.Value != 10 {
		t.Errorf("Value = %d; want 10", ele.Value)
	}
}

// TestMaxCommandCount confirms the command counter trips when the script
// runs too many simple commands. Not in SPEC §2.4's explicit acceptance
// list but is part of §2.3's wiring requirement.
func TestMaxCommandCount(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{
			MaxCommandCount: intPtr(5),
			// Keep loop budget very high so it doesn't trip first.
			MaxLoopIterations: intPtr(1000000),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = b.Exec(context.Background(), "for i in 1 2 3 4 5 6 7 8 9 10; do echo $i; done", gobash.ExecOptions{})
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) {
		t.Fatalf("Exec err = %v; want *gobash.ExecutionLimitError", err)
	}
	if ele.Limit != "MaxCommandCount" {
		t.Errorf("Limit = %q; want MaxCommandCount", ele.Limit)
	}
}

// TestLimitsDoNotImpactNormalScripts is a regression guard: a tiny script
// that stays well under every limit must run cleanly through the
// instrumented runner.
func TestLimitsDoNotImpactNormalScripts(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := b.Exec(context.Background(), "for i in a b c; do echo $i; done", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	want := "a\nb\nc\n"
	if res.Stdout != want {
		t.Errorf("Stdout = %q; want %q", res.Stdout, want)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", res.ExitCode)
	}
}

// TestLoopSentinelInvisible guards that the loop-iteration sentinel calls
// we inject do not appear in the user-visible output of the script.
func TestLoopSentinelInvisible(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := b.Exec(context.Background(), "for i in 1 2; do printf 'iter\\n'; done", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if strings.Contains(res.Stdout, "__gobash") {
		t.Errorf("Stdout leaked sentinel name: %q", res.Stdout)
	}
	if res.Stdout != "iter\niter\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "iter\niter\n")
	}
}
