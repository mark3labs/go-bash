package gobash_test

import (
	"context"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/command"
)

// TestPhase8RegistryExposed asserts that New surfaces a non-nil
// *command.Registry via the public Registry() accessor (SPEC §8.2).
func TestPhase8RegistryExposed(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if b.Registry() == nil {
		t.Fatal("Bash.Registry() returned nil")
	}
}

// TestPhase8CustomCommandRegistered asserts BashOptions.CustomCommands
// land in the registry (SPEC §1.2 / §8.2).
func TestPhase8CustomCommandRegistered(t *testing.T) {
	cmd := command.Define("hello", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{Stdout: "hello\n"}
	})
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{cmd},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := b.Registry().Lookup("hello")
	if !ok {
		t.Fatal("Lookup(\"hello\") not found")
	}
	if got.Name() != "hello" {
		t.Errorf("got.Name() = %q; want %q", got.Name(), "hello")
	}
}

// TestPhase8CustomCommandDispatches is the dispatch-success path:
// a script calling a registered command must reach Execute and the
// command's stdout must reach the script's stdout via the ExecHandler
// middleware (SPEC §5.3, §8).
func TestPhase8CustomCommandDispatches(t *testing.T) {
	called := false
	cmd := command.Define("probe", func(_ context.Context, args []string, c *command.Context) command.Result {
		called = true
		if c == nil {
			t.Error("dispatch Context was nil")
			return command.Result{ExitCode: 1}
		}
		// Write through the writer so the runtime does not
		// double-emit via Result.Stdout fallback.
		_, _ = c.Stdout.Write([]byte("probe-said-" + strings.Join(args[1:], "-") + "\n"))
		return command.Result{ExitCode: 0}
	})
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{cmd},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), `probe one two`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !called {
		t.Error("dispatched command's Execute was not called")
	}
	if res.Stdout != "probe-said-one-two\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "probe-said-one-two\n")
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", res.ExitCode)
	}
}

// TestPhase8CustomCommandResultStdoutFallback covers the secondary
// dispatch path: a command that does NOT write to c.Stdout but instead
// returns Result.Stdout should still see its bytes reach the script.
// This is the Phase 10 plumbing for builtins that want to compute
// their output up-front.
func TestPhase8CustomCommandResultStdoutFallback(t *testing.T) {
	cmd := command.Define("greet", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{Stdout: "hi\n"}
	})
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{cmd},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), `greet`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "hi\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "hi\n")
	}
}

// TestPhase8CustomCommandExitCode confirms a non-zero Result.ExitCode
// propagates to BashExecResult.ExitCode.
func TestPhase8CustomCommandExitCode(t *testing.T) {
	cmd := command.Define("fail", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{ExitCode: 42}
	})
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{cmd},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), `fail`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 42 {
		t.Errorf("ExitCode = %d; want 42", res.ExitCode)
	}
}

// TestPhase8UnknownCommandIsNotFound is the SANDBOX REGRESSION test
// for SPEC §8: an unregistered command must NOT reach host os/exec
// and MUST surface as `<name>: command not found\n` on stderr plus
// ExitCode 127. The Phase 5–7 behavior (pass-through to mvdan/sh's
// DefaultExecHandler, which would host-exec) is what this test
// guarantees we no longer do.
func TestPhase8UnknownCommandIsNotFound(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// `definitely_no_such_command_42` is not registered and is not
	// an mvdan/sh builtin. If the os/exec fall-through were still
	// active, this would either run a host binary (potential leak)
	// or return mvdan/sh's "executable file not found in $PATH"
	// message rather than our `command not found`.
	res, err := b.Exec(context.Background(), `definitely_no_such_command_42`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 127 {
		t.Errorf("ExitCode = %d; want 127", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "definitely_no_such_command_42: command not found") {
		t.Errorf("Stderr = %q; want substring %q",
			res.Stderr, "definitely_no_such_command_42: command not found")
	}
	// And make sure os/exec's diagnostic shape did NOT leak through.
	if strings.Contains(res.Stderr, "executable file not found in $PATH") {
		t.Errorf("Stderr contains mvdan/sh DefaultExecHandler diagnostic — os/exec gap reopened: %q", res.Stderr)
	}
}

// TestPhase8CustomCommandOverridesBuiltinName mirrors SPEC §1.2's
// "CustomCommands override built-ins". Phase 10 will land real
// built-ins; today we simulate by registering the same name twice
// (a Phase 10 built-in plus a CustomCommand) via two BashOptions
// constructions to verify the CustomCommand wins. We exercise the
// override via a SUBSEQUENT Register call on the live registry.
func TestPhase8CustomCommandOverridesBuiltinName(t *testing.T) {
	first := command.Define("dup", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{Stdout: "FIRST\n"}
	})
	override := command.Define("dup", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{Stdout: "OVERRIDE\n"}
	})
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{first, override},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), `dup`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "OVERRIDE\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "OVERRIDE\n")
	}
}

// TestPhase8BinStubsMaterialized covers the SPEC §7 ↔ §8 hand-off:
// every name in the registry produces a /bin/<name> stub file with
// mode 0o755 after New(BashOptions{}).
func TestPhase8BinStubsMaterialized(t *testing.T) {
	cmds := []command.Command{
		command.Define("alpha", func(_ context.Context, _ []string, _ *command.Context) command.Result {
			return command.Result{}
		}),
		command.Define("beta", func(_ context.Context, _ []string, _ *command.Context) command.Result {
			return command.Result{}
		}),
	}
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: cmds,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		fi, err := b.FS().Stat("/bin/" + name)
		if err != nil {
			t.Errorf("Stat(/bin/%s): %v", name, err)
			continue
		}
		if fi.IsDir() {
			t.Errorf("/bin/%s is a directory", name)
		}
		if mode := fi.Mode().Perm(); mode != 0o755 {
			t.Errorf("/bin/%s mode = %o; want 0755", name, mode)
		}
	}
}

// TestPhase8AbsoluteBinPathDispatches asserts a script invoking a
// command via its absolute /bin/<name> path routes through the
// registry — NOT through the sentinel binStubBody contents. SPEC §7
// guarantees the stub presence; SPEC §8 guarantees the dispatch
// hits the real command implementation.
func TestPhase8AbsoluteBinPathDispatches(t *testing.T) {
	cmd := command.Define("shout", func(_ context.Context, _ []string, c *command.Context) command.Result {
		_, _ = c.Stdout.Write([]byte("HEY\n"))
		return command.Result{}
	})
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{cmd},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), `/bin/shout`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "HEY\n" {
		t.Errorf("Stdout = %q; want %q (registry should resolve /bin/<name> by basename)",
			res.Stdout, "HEY\n")
	}
}

// TestPhase8BinStubsAbsentWhenLayoutSuppressed asserts the §7/§8
// hand-off is gated by the same Cwd/Files suppression as the rest of
// the default layout: a Cwd-built Bash gets no /bin stubs even when
// CustomCommands are registered.
func TestPhase8BinStubsAbsentWhenLayoutSuppressed(t *testing.T) {
	cmd := command.Define("nope", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{}
	})
	b, err := gobash.New(gobash.BashOptions{
		Cwd:            "/work",
		CustomCommands: []command.Command{cmd},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Registry still contains the command — CustomCommands register
	// regardless of layout suppression.
	if _, ok := b.Registry().Lookup("nope"); !ok {
		t.Fatal("CustomCommand registration was skipped under Cwd-suppressed layout")
	}
	// But the /bin stub must NOT exist.
	if _, err := b.FS().Stat("/bin/nope"); err == nil {
		t.Errorf("/bin/nope exists despite default layout suppression by Cwd")
	}
}

// TestPhase8EmptyRegistryNoBinStubs asserted (under Phase 8) that
// no built-ins => no /bin stubs. Phase 10 invalidates the premise:
// the builtins meta-package now blank-imports every Wave A command,
// so a default *Bash has a non-empty registry. The test is repurposed
// to assert that BashOptions.Commands=[]command.Name{} (empty, NOT
// nil) filters every built-in out and yields an empty /bin again.
func TestPhase8EmptyRegistryNoBinStubs(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Commands: []command.Name{}, // explicit empty allow-list
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := b.FS().ReadDir("/bin")
	if err != nil {
		t.Fatalf("ReadDir /bin: %v", err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("/bin entries = %v; want empty (BashOptions.Commands=[] filtered all)", names)
	}
}
