package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI is the in-process test driver. It wires a fresh IO struct
// around in-memory buffers and returns (exit, stdout, stderr).
func runCLI(t *testing.T, args []string, stdin string, hostCwd string, tty bool) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, IO{
		Stdin:      strings.NewReader(stdin),
		Stdout:     &stdout,
		Stderr:     &stderr,
		HostCwd:    hostCwd,
		StdinIsTTY: tty,
	})
	return code, stdout.String(), stderr.String()
}

func TestHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		code, out, _ := runCLI(t, []string{flag}, "", t.TempDir(), true)
		if code != 0 {
			t.Errorf("%s: exit = %d, want 0", flag, code)
		}
		if !strings.Contains(out, "Usage:") {
			t.Errorf("%s: help missing Usage: line; got %q", flag, out)
		}
	}
}

func TestVersion(t *testing.T) {
	for _, flag := range []string{"-v", "--version"} {
		code, out, _ := runCLI(t, []string{flag}, "", t.TempDir(), true)
		if code != 0 {
			t.Errorf("%s: exit = %d, want 0", flag, code)
		}
		if !strings.HasPrefix(out, "gobash ") {
			t.Errorf("%s: version line %q does not start with 'gobash '", flag, out)
		}
	}
}

func TestInlineScript(t *testing.T) {
	code, out, errOut := runCLI(t, []string{"-c", "echo hello"}, "", t.TempDir(), true)
	if code != 0 {
		t.Fatalf("exit = %d (stderr=%q)", code, errOut)
	}
	if out != "hello\n" {
		t.Errorf("stdout = %q, want %q", out, "hello\n")
	}
}

func TestExitCodeFromScript(t *testing.T) {
	code, _, _ := runCLI(t, []string{"-c", "exit 7"}, "", t.TempDir(), true)
	if code != 7 {
		t.Errorf("exit = %d, want 7", code)
	}
}

func TestErrexitCombinedShort(t *testing.T) {
	// `-ec 'set -e; false; echo not reached'` — same shape as the
	// just-bash example in the help text.
	code, out, _ := runCLI(t, []string{"-ec", "false; echo reached"}, "", t.TempDir(), true)
	if code == 0 {
		t.Errorf("exit = 0, want non-zero (errexit should abort)")
	}
	if strings.Contains(out, "reached") {
		t.Errorf("errexit did not abort: stdout = %q", out)
	}
}

func TestStdinScriptWhenNotTTY(t *testing.T) {
	code, out, _ := runCLI(t, nil, "echo from-stdin\n", t.TempDir(), false)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if out != "from-stdin\n" {
		t.Errorf("stdout = %q, want %q", out, "from-stdin\n")
	}
}

func TestStdinTTYPrintsHelp(t *testing.T) {
	// TTY + no -c + no file → print help, exit 1 (matches just-bash).
	code, out, _ := runCLI(t, nil, "", t.TempDir(), true)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("help missing; got %q", out)
	}
}

func TestEmptyScriptIsNoop(t *testing.T) {
	code, out, _ := runCLI(t, []string{"-c", "   \n  "}, "", t.TempDir(), true)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}
}

func TestJSONEnvelope(t *testing.T) {
	code, out, errOut := runCLI(t, []string{"--json", "-c", "echo a; echo b 1>&2; exit 3"}, "", t.TempDir(), true)
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
	if errOut != "" {
		t.Errorf("stderr should be empty in --json mode, got %q", errOut)
	}
	var env struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exitCode"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json: %v (raw: %q)", err, out)
	}
	if env.Stdout != "a\n" {
		t.Errorf("envelope.stdout = %q, want %q", env.Stdout, "a\n")
	}
	if env.Stderr != "b\n" {
		t.Errorf("envelope.stderr = %q, want %q", env.Stderr, "b\n")
	}
	if env.ExitCode != 3 {
		t.Errorf("envelope.exitCode = %d, want 3", env.ExitCode)
	}
}

func TestScriptFileFromMountedRoot(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "go.sh")
	if err := os.WriteFile(scriptPath, []byte("echo from-file\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	// Positional script-file resolved via the mount point.
	code, out, errOut := runCLI(t, []string{"--root", tmp, "go.sh"}, "", t.TempDir(), true)
	if code != 0 {
		t.Fatalf("exit = %d (stderr=%q)", code, errOut)
	}
	if out != "from-file\n" {
		t.Errorf("stdout = %q, want %q", out, "from-file\n")
	}
}

func TestScriptFileMissing(t *testing.T) {
	tmp := t.TempDir()
	code, _, errOut := runCLI(t, []string{"--root", tmp, "missing.sh"}, "", t.TempDir(), true)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "Cannot read script file") {
		t.Errorf("stderr missing 'Cannot read script file': %q", errOut)
	}
}

func TestSecondPositionalIsRoot(t *testing.T) {
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "g.sh")
	if err := os.WriteFile(scriptPath, []byte("echo second-pos\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	// just-bash: `just-bash script.sh /path/to/root` — second
	// positional is the root.
	code, out, errOut := runCLI(t, []string{"g.sh", tmp}, "", "/", true)
	if code != 0 {
		t.Fatalf("exit = %d (stderr=%q)", code, errOut)
	}
	if out != "second-pos\n" {
		t.Errorf("stdout = %q, want %q", out, "second-pos\n")
	}
}

func TestReadOnlyByDefault(t *testing.T) {
	tmp := t.TempDir()
	// Without --allow-write, writes into the mount point are
	// rejected by the read-only overlayfs.
	code, _, errOut := runCLI(t, []string{
		"--root", tmp,
		"-c", "echo nope > /home/user/project/x.txt",
	}, "", t.TempDir(), true)
	if code == 0 {
		t.Errorf("exit = 0, want non-zero (write should be rejected)")
	}
	if errOut == "" {
		t.Errorf("expected stderr diagnostic, got empty")
	}
	// Host disk untouched.
	if _, err := os.Stat(filepath.Join(tmp, "x.txt")); !os.IsNotExist(err) {
		t.Errorf("write leaked to host disk")
	}
}

func TestAllowWriteLandsInOverlay(t *testing.T) {
	tmp := t.TempDir()
	// With --allow-write, the write succeeds but stays in the
	// in-memory overlay — the host disk is still untouched.
	code, _, errOut := runCLI(t, []string{
		"--allow-write",
		"--root", tmp,
		"-c", "echo overlaid > /home/user/project/x.txt && cat /home/user/project/x.txt",
	}, "", t.TempDir(), true)
	if code != 0 {
		t.Fatalf("exit = %d (stderr=%q)", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(tmp, "x.txt")); !os.IsNotExist(err) {
		t.Errorf("overlay write leaked to host disk")
	}
}

func TestHostRootIsReadable(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "greet.txt"), []byte("hi from host\n"), 0o644); err != nil {
		t.Fatalf("seed host file: %v", err)
	}
	code, out, errOut := runCLI(t, []string{
		"--root", tmp,
		"-c", "cat /home/user/project/greet.txt",
	}, "", t.TempDir(), true)
	if code != 0 {
		t.Fatalf("exit = %d (stderr=%q)", code, errOut)
	}
	if out != "hi from host\n" {
		t.Errorf("stdout = %q, want %q", out, "hi from host\n")
	}
}

func TestCwdOverride(t *testing.T) {
	tmp := t.TempDir()
	code, out, errOut := runCLI(t, []string{
		"--root", tmp,
		"--cwd", "/tmp",
		"-c", "pwd",
	}, "", t.TempDir(), true)
	if code != 0 {
		t.Fatalf("exit = %d (stderr=%q)", code, errOut)
	}
	if strings.TrimSpace(out) != "/tmp" {
		t.Errorf("pwd = %q, want %q", out, "/tmp\n")
	}
}

func TestCwdDefaultIsMountPoint(t *testing.T) {
	tmp := t.TempDir()
	code, out, errOut := runCLI(t, []string{
		"--root", tmp,
		"-c", "pwd",
	}, "", t.TempDir(), true)
	if code != 0 {
		t.Fatalf("exit = %d (stderr=%q)", code, errOut)
	}
	if strings.TrimSpace(out) != defaultMountPoint {
		t.Errorf("pwd = %q, want %q", out, defaultMountPoint+"\n")
	}
}

func TestUnknownFlag(t *testing.T) {
	code, _, errOut := runCLI(t, []string{"--bogus"}, "", t.TempDir(), true)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "Unknown option") {
		t.Errorf("stderr missing 'Unknown option': %q", errOut)
	}
}

func TestMissingArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"-c missing value", []string{"-c"}, "-c requires"},
		{"--root missing value", []string{"--root"}, "--root requires"},
		{"--cwd missing value", []string{"--cwd"}, "--cwd requires"},
		{"--network-allow missing value", []string{"--network-allow"}, "--network-allow requires"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errOut := runCLI(t, tc.args, "", t.TempDir(), true)
			if code != 2 {
				t.Errorf("exit = %d, want 2", code)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr = %q, want substring %q", errOut, tc.want)
			}
		})
	}
}

func TestCombinedShortFlagsUnknown(t *testing.T) {
	code, _, errOut := runCLI(t, []string{"-eX"}, "", t.TempDir(), true)
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "Unknown option") {
		t.Errorf("stderr = %q, want 'Unknown option'", errOut)
	}
}

func TestNoNetworkExplicit(t *testing.T) {
	// --no-network is the default; supplying it explicitly should
	// be a no-op. We can't test network reach without an HTTP
	// server, but we can assert curl reports network disabled.
	code, _, errOut := runCLI(t, []string{
		"--no-network",
		"-c", "curl https://example.com",
	}, "", t.TempDir(), true)
	if code == 0 {
		t.Errorf("exit = 0, want non-zero (network disabled)")
	}
	if !strings.Contains(errOut, "network") {
		t.Errorf("stderr should mention network, got %q", errOut)
	}
}

func TestPythonJavascriptFlagsAccepted(t *testing.T) {
	// These flags are accepted for just-bash compat but produce
	// no runtime change (python/jsexec subpackages aren't linked
	// into the default CLI binary). The script itself should run.
	code, out, errOut := runCLI(t, []string{
		"--python", "--javascript",
		"-c", "echo opted-in",
	}, "", t.TempDir(), true)
	if code != 0 {
		t.Fatalf("exit = %d (stderr=%q)", code, errOut)
	}
	if out != "opted-in\n" {
		t.Errorf("stdout = %q, want %q", out, "opted-in\n")
	}
}
