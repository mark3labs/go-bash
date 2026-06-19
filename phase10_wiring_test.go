package gobash_test

import (
	"context"
	"strings"
	"testing"
	"time"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/command"
)

// TestPhase10BuiltinsRegistered asserts that a default *Bash exposes
// every Wave A built-in via its Registry().
func TestPhase10BuiltinsRegistered(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{
		"basename", "clear", "dirname", "echo", "expr", "false",
		"hostname", "printf", "pwd", "seq", "sleep", "true",
		"which", "whoami",
	}
	for _, n := range wantNames {
		if !b.Registry().Has(n) {
			t.Errorf("Registry missing built-in %q", n)
		}
	}
}

// TestPhase10CommandsFilterIncludeOnly asserts BashOptions.Commands
// honors the allow-list (only `true` and `false` register).
func TestPhase10CommandsFilterIncludeOnly(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Commands: []command.Name{"true", "false"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !b.Registry().Has("true") || !b.Registry().Has("false") {
		t.Errorf("true/false missing despite explicit allow-list")
	}
	for _, blocked := range []string{"echo", "printf", "expr"} {
		if b.Registry().Has(blocked) {
			t.Errorf("Registry has %q despite filter; should be excluded", blocked)
		}
	}
}

// TestPhase10CustomOverridesBuiltin asserts CustomCommands win over
// built-ins of the same name (the filter skips already-registered).
func TestPhase10CustomOverridesBuiltin(t *testing.T) {
	customRan := false
	custom := command.Define("true", func(_ context.Context, _ []string, c *command.Context) command.Result {
		customRan = true
		_, _ = c.Stdout.Write([]byte("custom-true\n"))
		return command.Result{ExitCode: 0}
	})
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{custom},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := b.Registry().Lookup("true")
	if !ok {
		t.Fatal("Lookup(true) missed")
	}
	if c != custom {
		t.Errorf("custom did not win the override; got %v", c)
	}
	res, err := b.Exec(context.Background(), "/bin/true", gobash.ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !customRan {
		t.Errorf("custom command did not run")
	}
	if !strings.Contains(res.Stdout, "custom-true") {
		t.Errorf("stdout = %q; want custom-true", res.Stdout)
	}
}

// TestPhase10ContextSleepPlumbed asserts BashOptions.Sleep is
// delivered to command.Context.Sleep via the dispatch middleware.
// Uses a custom probe command that captures the field and runs the
// hook with a small duration.
func TestPhase10ContextSleepPlumbed(t *testing.T) {
	var seenDuration time.Duration
	sleepFn := func(_ context.Context, d time.Duration) error {
		seenDuration = d
		return nil
	}
	var capturedSleep command.SleepFunc
	probe := command.Define("probe", func(ctx context.Context, _ []string, c *command.Context) command.Result {
		capturedSleep = c.Sleep
		if c.Sleep != nil {
			_ = c.Sleep(ctx, 250*time.Millisecond)
		}
		return command.Result{}
	})
	b, err := gobash.New(gobash.BashOptions{
		Sleep:          sleepFn,
		CustomCommands: []command.Command{probe},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Exec(context.Background(), "probe", gobash.ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if capturedSleep == nil {
		t.Fatal("Context.Sleep was nil despite BashOptions.Sleep set")
	}
	if seenDuration != 250*time.Millisecond {
		t.Errorf("seenDuration = %v; want 250ms (Sleep hook not actually plumbed)", seenDuration)
	}
}

// TestPhase10ContextLimitsPlumbed asserts ResolvedLimits flows into
// command.Context.Limits at dispatch time. Wave A built-ins ignore
// it; the test uses a custom probe.
func TestPhase10ContextLimitsPlumbed(t *testing.T) {
	var seen command.Limits
	probe := command.Define("probe", func(_ context.Context, _ []string, c *command.Context) command.Result {
		seen = c.Limits
		return command.Result{}
	})
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{probe},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Exec(context.Background(), "probe", gobash.ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if seen.MaxCommandCount == 0 {
		t.Errorf("Context.Limits not plumbed (MaxCommandCount=0)")
	}
}
