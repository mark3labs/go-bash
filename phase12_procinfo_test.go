package gobash_test

import (
	"context"
	"io"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	_ "github.com/mark3labs/go-bash/builtins"
	"github.com/mark3labs/go-bash/fs"
)

// SPEC §12 — Process Info & Defaults acceptance tests.
//
// Phase 12 audits five behaviors that together emulate a deterministic,
// host-disconnected process identity:
//
//  1. $$  resolves to ProcessInfo.PID (default 1), NOT host os.Getpid().
//  2. $BASHPID starts at ProcessInfo.PID and increments per subshell.
//  3. /proc/self/status matches the SPEC §11 template byte-for-byte.
//  4. whoami always prints "user\n".
//  5. hostname reads /etc/hostname (default "localhost\n").
//
// Items (3), (4), (5) were wired in Phase 7 + Phase 10 Wave A; we pin
// them with explicit assertions here. Items (1) and (2) required new
// machinery in Phase 12 — see procinfo.go for the rewrite pass.

func runCapture(t *testing.T, opts gobash.BashOptions, script string) gobash.BashExecResult {
	t.Helper()
	b, err := gobash.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := b.Exec(context.Background(), script, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	return res
}

// TestPhase12DollarDollar locks $$ to procInfo.PID (NOT host PID).
func TestPhase12DollarDollar(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, "echo $$")
		if res.Stdout != "1\n" {
			t.Errorf("$$ default: got %q, want %q", res.Stdout, "1\n")
		}
	})
	t.Run("custom_pid", func(t *testing.T) {
		pi := gobash.ProcessInfo{PID: 4242, PPID: 1, UID: 1000, GID: 1000}
		res := runCapture(t, gobash.BashOptions{ProcessInfo: &pi}, "echo $$")
		if res.Stdout != "4242\n" {
			t.Errorf("$$ custom: got %q, want %q", res.Stdout, "4242\n")
		}
	})
	t.Run("inside_subshell", func(t *testing.T) {
		// $$ is the shell's PID, NOT the subshell's. Spec is silent
		// but real bash semantics + the just-bash port agree: $$
		// is constant across subshells; BASHPID is the per-subshell
		// pid.
		res := runCapture(t, gobash.BashOptions{}, "(echo $$)")
		if res.Stdout != "1\n" {
			t.Errorf("$$ in subshell: got %q, want %q", res.Stdout, "1\n")
		}
	})
	t.Run("inside_double_quote", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, `echo "pid=$$"`)
		if res.Stdout != "pid=1\n" {
			t.Errorf(`"pid=$$": got %q, want %q`, res.Stdout, "pid=1\n")
		}
	})
}

// TestPhase12PPID locks $PPID to procInfo.PPID (NOT host PPID).
func TestPhase12PPID(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, "echo $PPID")
		if res.Stdout != "0\n" {
			t.Errorf("$PPID default: got %q, want %q", res.Stdout, "0\n")
		}
	})
	t.Run("braced", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, "echo ${PPID}")
		if res.Stdout != "0\n" {
			t.Errorf("${PPID}: got %q, want %q", res.Stdout, "0\n")
		}
	})
	t.Run("custom_ppid", func(t *testing.T) {
		pi := gobash.ProcessInfo{PID: 1, PPID: 9999, UID: 1000, GID: 1000}
		res := runCapture(t, gobash.BashOptions{ProcessInfo: &pi}, "echo $PPID")
		if res.Stdout != "9999\n" {
			t.Errorf("$PPID custom: got %q, want %q", res.Stdout, "9999\n")
		}
	})
}

// TestPhase12BASHPID locks per-subshell increment semantics.
func TestPhase12BASHPID(t *testing.T) {
	t.Run("top_level_equals_pid", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, "echo $BASHPID")
		if res.Stdout != "1\n" {
			t.Errorf("$BASHPID top-level: got %q, want %q", res.Stdout, "1\n")
		}
	})
	t.Run("subshell_increments", func(t *testing.T) {
		// SPEC §12: "each subshell increments a counter". Two
		// sibling subshells must get distinct values, and the outer
		// reference must remain procInfo.PID.
		res := runCapture(t, gobash.BashOptions{},
			"echo $BASHPID; (echo $BASHPID); (echo $BASHPID); echo $BASHPID")
		want := "1\n2\n3\n1\n"
		if res.Stdout != want {
			t.Errorf("subshell sibling counter: got %q, want %q", res.Stdout, want)
		}
	})
	t.Run("nested_subshells", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{},
			"echo $BASHPID; (echo $BASHPID; (echo $BASHPID; echo $BASHPID); echo $BASHPID); echo $BASHPID")
		// outer=1, ss1=2, ss2=3, ss2=3 again, back to ss1=2, back to outer=1.
		want := "1\n2\n3\n3\n2\n1\n"
		if res.Stdout != want {
			t.Errorf("nested counter: got %q, want %q", res.Stdout, want)
		}
	})
	t.Run("custom_pid_seed", func(t *testing.T) {
		pi := gobash.ProcessInfo{PID: 100, PPID: 0, UID: 1000, GID: 1000}
		res := runCapture(t, gobash.BashOptions{ProcessInfo: &pi},
			"echo $BASHPID; (echo $BASHPID); echo $BASHPID")
		want := "100\n101\n100\n"
		if res.Stdout != want {
			t.Errorf("custom seed: got %q, want %q", res.Stdout, want)
		}
	})
	t.Run("braced", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, "echo ${BASHPID}")
		if res.Stdout != "1\n" {
			t.Errorf("${BASHPID}: got %q, want %q", res.Stdout, "1\n")
		}
	})
}

// TestPhase12ProcSelfStatus pins the /proc/self/status template to the
// byte-exact SPEC §11 layout.
func TestPhase12ProcSelfStatus(t *testing.T) {
	t.Run("default_procinfo", func(t *testing.T) {
		b, err := gobash.New(gobash.BashOptions{})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got := readVFSFile(t, b, "/proc/self/status")
		want := "Name:\tbash\n" +
			"Pid:\t1\n" +
			"PPid:\t0\n" +
			"Uid:\t1000\t1000\t1000\t1000\n" +
			"Gid:\t1000\t1000\t1000\t1000\n"
		if got != want {
			t.Errorf("/proc/self/status default:\n got: %q\nwant: %q", got, want)
		}
	})
	t.Run("custom_procinfo", func(t *testing.T) {
		pi := gobash.ProcessInfo{PID: 42, PPID: 7, UID: 2001, GID: 3001}
		b, err := gobash.New(gobash.BashOptions{ProcessInfo: &pi})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got := readVFSFile(t, b, "/proc/self/status")
		want := "Name:\tbash\n" +
			"Pid:\t42\n" +
			"PPid:\t7\n" +
			"Uid:\t2001\t2001\t2001\t2001\n" +
			"Gid:\t3001\t3001\t3001\t3001\n"
		if got != want {
			t.Errorf("/proc/self/status custom:\n got: %q\nwant: %q", got, want)
		}
	})
	t.Run("readable_via_cat", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, "cat /proc/self/status")
		if !strings.Contains(res.Stdout, "Name:\tbash\n") || !strings.Contains(res.Stdout, "Pid:\t1\n") {
			t.Errorf("cat /proc/self/status missing template lines:\n%q", res.Stdout)
		}
	})
}

// TestPhase12Whoami locks `whoami` to the sandbox-default "user\n".
func TestPhase12Whoami(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, "whoami")
		if res.Stdout != "user\n" {
			t.Errorf("whoami: got %q, want %q", res.Stdout, "user\n")
		}
	})
	t.Run("ignores_uid_change", func(t *testing.T) {
		// SPEC §12: "whoami always prints `user`" — the literal is
		// not derived from procInfo.UID. Verify a non-default UID
		// does not alter the output.
		pi := gobash.ProcessInfo{PID: 1, PPID: 0, UID: 0, GID: 0}
		res := runCapture(t, gobash.BashOptions{ProcessInfo: &pi}, "whoami")
		if res.Stdout != "user\n" {
			t.Errorf("whoami w/ UID=0: got %q, want %q", res.Stdout, "user\n")
		}
	})
}

// TestPhase12Hostname locks `hostname` to read /etc/hostname.
func TestPhase12Hostname(t *testing.T) {
	t.Run("default_localhost", func(t *testing.T) {
		res := runCapture(t, gobash.BashOptions{}, "hostname")
		if res.Stdout != "localhost\n" {
			t.Errorf("hostname default: got %q, want %q", res.Stdout, "localhost\n")
		}
	})
	t.Run("custom_etc_hostname", func(t *testing.T) {
		// Caller-supplied Files takes precedence over the default
		// layout. A bare `hostname` value plus a literal newline is
		// what real /etc/hostname files look like; the read path
		// also tolerates files without the trailing newline.
		b, err := gobash.New(gobash.BashOptions{
			Files: map[string]fs.FileInit{
				"/etc/hostname": {Content: []byte("box-42\n")},
			},
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		res, err := b.Exec(context.Background(), "hostname", gobash.ExecOptions{})
		if err != nil {
			t.Fatalf("Exec: %v", err)
		}
		if res.Stdout != "box-42\n" {
			t.Errorf("hostname custom: got %q, want %q", res.Stdout, "box-42\n")
		}
	})
}

func readVFSFile(t *testing.T, b *gobash.Bash, path string) string {
	t.Helper()
	f, err := b.FS().OpenFile(path, 0, 0)
	if err != nil {
		t.Fatalf("OpenFile %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll %s: %v", path, err)
	}
	return string(data)
}
