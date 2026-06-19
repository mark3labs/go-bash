package gobash_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	gbfs "github.com/mark3labs/go-bash/fs"
)

// TestPhase5SmokeForLoop is the §5.7 acceptance: a vanilla for-loop
// prints 1\n2\n3\n via the in-memory bridge.
func TestPhase5SmokeForLoop(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		"for i in 1 2 3; do echo $i; done",
		gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "1\n2\n3\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "1\n2\n3\n")
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d; want 0", res.ExitCode)
	}
}

// TestPhase5SmokeSubshellScope is the §5.7 acceptance for the
// subshell-isolation case: assignments inside (…) must not leak to the
// outer scope.
func TestPhase5SmokeSubshellScope(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		"x=1; (x=2; echo $x); echo $x",
		gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "2\n1\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "2\n1\n")
	}
}

// TestPhase5SmokeVFSRedirect mirrors the SPEC.md §11 final smoke test
// against the in-memory VFS — write via `>`, read via `<`, all going
// through the OpenHandler installed by interp.BuildRunner. The literal
// `cat greeting.txt` form from SPEC.md line 1611 needs the `cat`
// built-in from Phase 10 (the stub commandExecHandler currently falls
// through to mvdan/sh's DefaultExecHandler, which would `os/exec` the
// host `cat` and miss the VFS). We exercise the redirect side here and
// defer the cat-builtin check to Phase 10's acceptance fixture.
func TestPhase5SmokeVFSRedirect(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		`echo "Hello" > greeting.txt; read line < greeting.txt; echo "$line"`,
		gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "Hello\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "Hello\n")
	}
	// The host disk must NOT have a greeting.txt at the runner Dir.
	got, err := b.FS().ReadFile("/home/user/greeting.txt")
	if err != nil {
		t.Fatalf("VFS ReadFile: %v", err)
	}
	if string(got) != "Hello\n" {
		t.Errorf("VFS contents = %q; want %q", got, "Hello\n")
	}
}

// TestPhase5ExportPropagatesAcrossExec covers the §5.6 + §5.7 contract:
// `export X=hello` then `echo $X` in a second Exec call prints hello.
func TestPhase5ExportPropagatesAcrossExec(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Exec(context.Background(), "export X=hello", gobash.ExecOptions{}); err != nil {
		t.Fatalf("Exec 1: %v", err)
	}
	res, err := b.Exec(context.Background(), "echo $X", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec 2: %v", err)
	}
	if res.Stdout != "hello\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "hello\n")
	}
}

// TestPhase5PerCallEnvDoesNotPolluteBase covers the §5.6 + §5.7 contract:
// when ExecOptions.Env is supplied (without ReplaceEnv), the per-call
// vars and any exports made during the call stay local to that call.
func TestPhase5PerCallEnvDoesNotPolluteBase(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// The script exports a new var; without the per-call env it would
	// normally persist into b.env. With opts.Env set (and ReplaceEnv
	// false), the export must be ephemeral.
	_, err = b.Exec(context.Background(),
		"export X=should_not_persist",
		gobash.ExecOptions{Env: map[string]string{"X": "once"}})
	if err != nil {
		t.Fatalf("Exec 1: %v", err)
	}
	res, err := b.Exec(context.Background(), "echo \"got=${X:-unset}\"", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec 2: %v", err)
	}
	if res.Stdout != "got=unset\n" {
		t.Errorf("per-call export leaked: Stdout = %q; want %q",
			res.Stdout, "got=unset\n")
	}
}

// TestPhase5ReplaceEnvSnapshotsExports verifies that when the caller
// passes ReplaceEnv=true with a per-call Env, the script's exports DO
// propagate back into b.env (matching the literal §5.6 reading: copy
// back UNLESS Env was set without ReplaceEnv).
func TestPhase5ReplaceEnvSnapshotsExports(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(context.Background(),
		"export Y=persists",
		gobash.ExecOptions{Env: map[string]string{"BASE": "1"}, ReplaceEnv: true})
	if err != nil {
		t.Fatalf("Exec 1: %v", err)
	}
	res, err := b.Exec(context.Background(), "echo \"y=${Y:-unset}\"", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec 2: %v", err)
	}
	if res.Stdout != "y=persists\n" {
		t.Errorf("ReplaceEnv+export did not persist: Stdout = %q", res.Stdout)
	}
}

// TestPhase5SetEPropagates is the §5.7 acceptance for `set -e`: after a
// non-zero command, the script aborts and ExitCode reflects the failure.
func TestPhase5SetEPropagates(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		"set -e; false; echo should_not_run",
		gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("ExitCode = 0; want non-zero (set -e abort)")
	}
	if strings.Contains(res.Stdout, "should_not_run") {
		t.Errorf("set -e did not abort: Stdout = %q", res.Stdout)
	}
}

// TestPhase5CdAndPwdViaVFS is the §5.7 acceptance: `cd` updates the
// runner's Dir, and `pwd` reports it. The path must exist in the VFS;
// otherwise mvdan/sh's `cd` builtin (which calls our StatHandler) will
// reject the directory.
func TestPhase5CdAndPwdViaVFS(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{"/tmp": {Dir: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		"cd /tmp && pwd",
		gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "/tmp\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "/tmp\n")
	}
}

// TestPhase5StdinPropagates re-asserts ExecOptions.Stdin wiring through
// the bridge (Phase 1 already covered this; we keep the assertion here
// so Phase 5's regression surface owns it explicitly).
func TestPhase5StdinPropagates(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		`read x; read y; echo "$x|$y"`,
		gobash.ExecOptions{Stdin: strings.NewReader("foo\nbar\n")})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "foo|bar\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "foo|bar\n")
	}
}

// TestPhase5StdoutWriterStillCapturesCorrectly confirms the
// caller-supplied Stdout writer keeps working through the bridge.
func TestPhase5StdoutWriterStillCapturesCorrectly(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	res, err := b.Exec(context.Background(),
		"echo via-writer",
		gobash.ExecOptions{Stdout: &buf})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if buf.String() != "via-writer\n" {
		t.Errorf("writer captured %q; want %q", buf.String(), "via-writer\n")
	}
	if res.Stdout != "" {
		t.Errorf("Stdout should be empty when writer supplied; got %q", res.Stdout)
	}
}

// TestPhase5ParserLimitsSurfaceFromExec confirms the parser-side hard
// limits (§4.2) now surface from Exec because Phase 5 routes parsing
// through gobash/parser.Parse instead of raw syntax.NewParser. We use
// the smallest reproducible limit (MaxInputSize) to keep the test cheap.
func TestPhase5ParserLimitsSurfaceFromExec(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("a", (1<<20)+1) // MaxInputSize + 1
	_, err = b.Exec(context.Background(), huge, gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected ParseError; got nil")
	}
	var pe *gobash.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %T %v; want *gobash.ParseError", err, err)
	}
}
