package gobash_test

import (
	"context"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	_ "github.com/mark3labs/go-bash/builtins"
)

// TestAliasExpansionPhase11 verifies alias parse-time expansion fires
// when `shopt expand_aliases` is on. Phase 11 acceptance.
func TestAliasExpansionPhase11(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b.Shopt().Set("expand_aliases", true)
	b.Aliases().Set("greet", "echo hello")
	res, err := b.Exec(context.Background(), "greet world", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "hello world\n" {
		t.Errorf("stdout=%q want=%q", res.Stdout, "hello world\n")
	}
}

func TestAliasExpansionOff(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b.Aliases().Set("greet", "echo hello")
	// expand_aliases is OFF by default; alias should NOT expand.
	res, _ := b.Exec(context.Background(), "greet world", gobash.ExecOptions{})
	// `greet` is not a registered command; expect "command not found".
	if res.ExitCode != 127 {
		t.Errorf("expected 127 (command not found), got %d", res.ExitCode)
	}
}
