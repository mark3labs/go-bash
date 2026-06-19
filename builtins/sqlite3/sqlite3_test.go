package sqlite3_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/sqlite3"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := sqlite3.New().Execute(context.Background(),
		append([]string{"sqlite3"}, args...),
		&command.Context{Cwd: "/", Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestStub(t *testing.T) {
	_, e, code := run(t, ":memory:", "SELECT 1")
	if code == 0 {
		t.Errorf("expected non-zero exit, got %d (stderr=%q)", code, e)
	}
	if !strings.Contains(e, "not enabled") {
		t.Errorf("expected 'not enabled' in stderr, got %q", e)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: sqlite3") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestNoArgs(t *testing.T) {
	// Even with no args, the stub fails — it's NOT enabled.
	_, e, code := run(t)
	if code == 0 {
		t.Errorf("expected non-zero exit, got %d (stderr=%q)", code, e)
	}
	if !strings.Contains(e, "sqlite3") {
		t.Errorf("expected diagnostic prefix, got %q", e)
	}
}
