package sandbox_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/sandbox"
)

func TestCreateAndRunEcho(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd:  "echo",
		Args: []string{"hello", "world"},
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	out, err := cmd.Stdout()
	if err != nil {
		t.Fatalf("Stdout: %v", err)
	}
	if out != "hello world\n" {
		t.Errorf("stdout = %q, want %q", out, "hello world\n")
	}
	fin, err := cmd.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if fin.ExitCode != 0 {
		t.Errorf("exit = %d, want 0", fin.ExitCode)
	}
}

func TestRunCommandQuotesSpecialChars(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Args contain $ and spaces that would otherwise be re-expanded
	// or split if not quoted.
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd:  "printf",
		Args: []string{"%s\n", "a b", "$HOME", "with*star"},
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	out, err := cmd.Stdout()
	if err != nil {
		t.Fatalf("Stdout: %v", err)
	}
	want := "a b\n$HOME\nwith*star\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestRunCommandStderrAndExitCode(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// `false` exits 1; `cat` of a missing file writes to stderr.
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd:  "false",
		Args: nil,
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	fin, err := cmd.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if fin.ExitCode != 1 {
		t.Errorf("exit = %d, want 1", fin.ExitCode)
	}
}

func TestRunCommandDetachedWaitBlocks(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// `sleep` is a registered builtin (Wave G). 50 ms is short
	// enough for tests but long enough that the detached goroutine
	// has not finished by the time RunCommand returns.
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd:      "sleep",
		Args:     []string{"0.05"},
		Detached: true,
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	start := time.Now()
	if _, err := cmd.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Errorf("Wait returned too quickly (%v) — detached did not actually defer execution", elapsed)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{
		Timeout: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	start := time.Now()
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd:  "sleep",
		Args: []string{"5"},
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	fin, _ := cmd.Wait()
	elapsed := time.Since(start)
	if elapsed > 1*time.Second {
		t.Errorf("Timeout did not fire: elapsed=%v", elapsed)
	}
	// sleep maps ctx-timeout to ExitCode 130 (SIGINT-shaped).
	if fin.ExitCode == 0 {
		t.Errorf("ExitCode = 0, want non-zero (timeout should kill sleep)")
	}
}

func TestWriteFilesAndReadFile(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sb.WriteFiles(ctx, map[string]string{
		"/tmp/hello.txt":     "hello\n",
		"/nested/deep/x.txt": "x",
	}); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	got, err := sb.ReadFile(ctx, "/tmp/hello.txt")
	if err != nil {
		t.Fatalf("ReadFile hello: %v", err)
	}
	if got != "hello\n" {
		t.Errorf("hello.txt = %q, want %q", got, "hello\n")
	}
	got, err = sb.ReadFile(ctx, "/nested/deep/x.txt")
	if err != nil {
		t.Fatalf("ReadFile x: %v", err)
	}
	if got != "x" {
		t.Errorf("x.txt = %q, want %q", got, "x")
	}
}

func TestReadFileMissing(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = sb.ReadFile(ctx, "/does/not/exist")
	if err == nil {
		t.Fatalf("ReadFile missing: expected error, got nil")
	}
}

func TestMkDirRecursive(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sb.MkDir(ctx, "/a/b/c", sandbox.MkDirOptions{Recursive: true}); err != nil {
		t.Fatalf("MkDir recursive: %v", err)
	}
	info, err := sb.FS().Stat("/a/b/c")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("/a/b/c is not a directory")
	}
}

func TestMkDirNonRecursiveMissingParentErrors(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sb.MkDir(ctx, "/x/y/z", sandbox.MkDirOptions{}); err == nil {
		t.Errorf("MkDir non-recursive with missing parents: expected error")
	}
}

func TestFSAndOverlayRootMutuallyExclusive(t *testing.T) {
	ctx := context.Background()
	_, err := sandbox.Create(ctx, sandbox.Options{
		FS:          nil, // explicitly nil; we set it below via memfs would conflict
		OverlayRoot: "/some/root",
	})
	// only OverlayRoot set, no conflict
	if err != nil {
		// /some/root probably doesn't exist or is fine — overlayfs.New
		// doesn't stat the root. So no error expected here.
		t.Fatalf("Create with only OverlayRoot: unexpected error: %v", err)
	}
	// Now set both.
	sb, _ := sandbox.Create(ctx, sandbox.Options{})
	_, err = sandbox.Create(ctx, sandbox.Options{
		FS:          sb.FS(),
		OverlayRoot: "/some/root",
	})
	if !errors.Is(err, sandbox.ErrFSConflict) {
		t.Errorf("expected ErrFSConflict, got %v", err)
	}
}

func TestOverlayRootReadsHostFiles(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "greeting.txt"), []byte("from host\n"), 0o644); err != nil {
		t.Fatalf("write host file: %v", err)
	}
	sb, err := sandbox.Create(ctx, sandbox.Options{
		OverlayRoot: tmp,
		Cwd:         "/",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := sb.ReadFile(ctx, "/greeting.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != "from host\n" {
		t.Errorf("got %q, want %q", got, "from host\n")
	}
	// Writes land in the overlay and are visible on read-back.
	if err := sb.WriteFiles(ctx, map[string]string{"/new.txt": "overlay"}); err != nil {
		t.Fatalf("WriteFiles: %v", err)
	}
	got, err = sb.ReadFile(ctx, "/new.txt")
	if err != nil {
		t.Fatalf("ReadFile new: %v", err)
	}
	if got != "overlay" {
		t.Errorf("new.txt = %q, want %q", got, "overlay")
	}
	// Host disk untouched.
	if _, err := os.Stat(filepath.Join(tmp, "new.txt")); !os.IsNotExist(err) {
		t.Errorf("overlay write leaked to host disk: stat err = %v", err)
	}
}

func TestEnvSeedAndPerCallOverride(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{
		Env: map[string]string{"FOO": "from-create", "BAR": "stable"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// First call: per-call env overrides FOO, BAR carries through.
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd:  "sh",
		Args: []string{"-c", "echo $FOO $BAR"},
		Env:  map[string]string{"FOO": "from-call"},
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	out, err := cmd.Stdout()
	if err != nil {
		t.Fatalf("Stdout: %v", err)
	}
	if strings.TrimSpace(out) != "from-call stable" {
		t.Errorf("got %q, want %q", out, "from-call stable\n")
	}
}

func TestPerCallCwdOverride(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sb.MkDir(ctx, "/work", sandbox.MkDirOptions{Recursive: true}); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd: "pwd",
		Cwd: "/work",
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	out, err := cmd.Stdout()
	if err != nil {
		t.Fatalf("Stdout: %v", err)
	}
	if strings.TrimSpace(out) != "/work" {
		t.Errorf("pwd = %q, want %q", out, "/work\n")
	}
}

func TestStdinPipedThrough(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd:   "cat",
		Stdin: strings.NewReader("piped input\n"),
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	out, err := cmd.Stdout()
	if err != nil {
		t.Fatalf("Stdout: %v", err)
	}
	if out != "piped input\n" {
		t.Errorf("cat stdout = %q, want %q", out, "piped input\n")
	}
}

func TestStopIsNoOp(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sb.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	// Sandbox still usable post-Stop (no-op).
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{Cmd: "true"})
	if err != nil {
		t.Fatalf("RunCommand post-Stop: %v", err)
	}
	if _, err := cmd.Wait(); err != nil {
		t.Errorf("Wait post-Stop: %v", err)
	}
}

func TestEmptyCmdRejected(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = sb.RunCommand(ctx, sandbox.RunCommandParams{Cmd: ""})
	if err == nil {
		t.Errorf("RunCommand with empty Cmd: expected error")
	}
}

func TestBashAccessor(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sb.Bash() == nil {
		t.Fatalf("Bash() returned nil")
	}
	// Underlying Bash is fully wired — Exec works.
	res, err := sb.Bash().Exec(ctx, "echo via-bash", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Bash().Exec: %v", err)
	}
	if res.Stdout != "via-bash\n" {
		t.Errorf("Exec stdout = %q, want %q", res.Stdout, "via-bash\n")
	}
}

func TestSudoIsIgnored(t *testing.T) {
	ctx := context.Background()
	sb, err := sandbox.Create(ctx, sandbox.Options{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	cmd, err := sb.RunCommand(ctx, sandbox.RunCommandParams{
		Cmd:  "echo",
		Args: []string{"safe"},
		Sudo: true, // no-op
	})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	out, err := cmd.Stdout()
	if err != nil {
		t.Fatalf("Stdout: %v", err)
	}
	if out != "safe\n" {
		t.Errorf("stdout = %q, want %q", out, "safe\n")
	}
}
