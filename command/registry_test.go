package command_test

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

// TestRegistryRegisterLookup is the spec acceptance:
// Register puts a command into the registry; Lookup retrieves it.
func TestRegistryRegisterLookup(t *testing.T) {
	r := command.NewRegistry()
	cmd := command.Define("hello", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{Stdout: "world\n"}
	})
	r.Register(cmd)

	got, ok := r.Lookup("hello")
	if !ok {
		t.Fatalf("Lookup(\"hello\"): not found")
	}
	if got.Name() != "hello" {
		t.Errorf("got.Name() = %q; want %q", got.Name(), "hello")
	}
	if !got.Trusted() {
		t.Errorf("Define-built commands must be Trusted() == true")
	}
}

// TestRegistryLookupMissing covers the negative path.
func TestRegistryLookupMissing(t *testing.T) {
	r := command.NewRegistry()
	if _, ok := r.Lookup("nope"); ok {
		t.Errorf("Lookup of unregistered name returned ok=true")
	}
}

// TestRegistryNamesSorted asserts the Names() return is sorted —
// The spec's /bin stub list depends on this for reproducibility.
func TestRegistryNamesSorted(t *testing.T) {
	r := command.NewRegistry()
	for _, n := range []string{"foo", "bar", "baz"} {
		r.Register(command.Define(n, func(_ context.Context, _ []string, _ *command.Context) command.Result {
			return command.Result{}
		}))
	}
	got := r.Names()
	want := []command.Name{"bar", "baz", "foo"}
	if len(got) != len(want) {
		t.Fatalf("Names() len = %d; want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Names()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// TestRegistryRegisterOverwrites is the "CustomCommands override
// built-ins" the spec contract — a later Register on the same Name
// replaces the previous entry.
func TestRegistryRegisterOverwrites(t *testing.T) {
	r := command.NewRegistry()
	first := command.Define("dup", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{Stdout: "first"}
	})
	second := command.Define("dup", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{Stdout: "second"}
	})
	r.Register(first)
	r.Register(second)

	got, _ := r.Lookup("dup")
	res := got.Execute(context.Background(), nil, nil)
	if res.Stdout != "second" {
		t.Errorf("override Stdout = %q; want %q", res.Stdout, "second")
	}
}

// TestRegistryHasSkipsAlreadyPresent is the helper Phase 10's
// built-in bootstrap will use: CustomCommands win because the
// built-in loop calls Has() and skips registered names.
func TestRegistryHasSkipsAlreadyPresent(t *testing.T) {
	r := command.NewRegistry()
	r.Register(command.Define("a", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{}
	}))
	if !r.Has("a") {
		t.Errorf("Has(\"a\") after Register == false")
	}
	if r.Has("b") {
		t.Errorf("Has(\"b\") with no Register == true")
	}
}

// TestRegistryNilSafe ensures a nil Registry is read-safe so a
// caller that forgot to construct one (e.g. an interp.Config with
// Registry==nil) doesn't blow up.
func TestRegistryNilSafe(t *testing.T) {
	var r *command.Registry
	if _, ok := r.Lookup("anything"); ok {
		t.Errorf("nil Registry Lookup returned ok=true")
	}
	if names := r.Names(); names != nil {
		t.Errorf("nil Registry Names() = %v; want nil", names)
	}
	if r.Has("anything") {
		t.Errorf("nil Registry Has() = true; want false")
	}
}

// TestRegisterNilSkips guards against a panic from registering
// nil. The runtime currently doesn't pass nil but a defensive check
// is cheap.
func TestRegisterNilSkips(t *testing.T) {
	r := command.NewRegistry()
	r.Register(nil) // must not panic
	if names := r.Names(); len(names) != 0 {
		t.Errorf("Names() after Register(nil) = %v; want empty", names)
	}
}

// TestDefineExecutesUserFunc round-trips through Define + Execute.
func TestDefineExecutesUserFunc(t *testing.T) {
	called := false
	cmd := command.Define("probe", func(_ context.Context, args []string, _ *command.Context) command.Result {
		called = true
		return command.Result{ExitCode: len(args)}
	})
	res := cmd.Execute(context.Background(), []string{"probe", "one", "two"}, nil)
	if !called {
		t.Fatal("Define fn was not invoked")
	}
	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d; want 3", res.ExitCode)
	}
}

// TestRegisterBuiltinAppends asserts RegisterBuiltin grows the
// package-level slice (Phase 10's Wave A registration mechanism).
func TestRegisterBuiltinAppends(t *testing.T) {
	// Capture the pre-call snapshot so this test is order-independent
	// against the rest of the suite (which also pulls in the real
	// builtins via gobash blank imports — but this is the command
	// package's own test binary, which DOES NOT, so we start near 0).
	before := len(command.DefaultBuiltins())
	c1 := command.Define("test-rb-1", nil)
	c2 := command.Define("test-rb-2", nil)
	command.RegisterBuiltin(c1)
	command.RegisterBuiltin(c2)
	command.RegisterBuiltin(nil) // nil is ignored
	after := command.DefaultBuiltins()
	if len(after) != before+2 {
		t.Errorf("DefaultBuiltins grew by %d, want 2", len(after)-before)
	}
}

// TestDefaultBuiltinsSnapshot asserts the returned slice is a copy —
// mutating it MUST NOT mutate subsequent calls.
func TestDefaultBuiltinsSnapshot(t *testing.T) {
	a := command.DefaultBuiltins()
	if a == nil {
		a = []command.Command{}
	}
	// Mutating a should not affect b.
	if len(a) > 0 {
		a[0] = nil
	}
	b := command.DefaultBuiltins()
	if len(a) != len(b) {
		t.Errorf("len changed under snapshot: %d vs %d", len(a), len(b))
	}
	for i := range b {
		if b[i] == nil {
			t.Errorf("snapshot %d is nil — leak from prior mutation", i)
		}
	}
}
