package sqlite_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/sqlite"
)

// newBash constructs a Bash with the real sqlite3 runtime registered.
// Phase 10's Wave H stub registers itself via init(); calling
// sqlite.Register overrides it on b.Registry().
func newBash(t *testing.T, opts sqlite.Options) *gobash.Bash {
	t.Helper()
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("gobash.New: %v", err)
	}
	if err := sqlite.Register(b, opts); err != nil {
		t.Fatalf("sqlite.Register: %v", err)
	}
	return b
}

func exec(t *testing.T, b *gobash.Bash, script string) gobash.BashExecResult {
	t.Helper()
	res, err := b.Exec(context.Background(), script, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	return res
}

func TestRegisterNilBash(t *testing.T) {
	if err := sqlite.Register(nil, sqlite.Options{}); err == nil {
		t.Fatalf("Register(nil) returned nil error")
	}
}

func TestMemoryListMode(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 :memory: "SELECT 1, 'two', 3.5"`)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	if res.Stdout != "1|two|3.5\n" {
		t.Errorf("stdout = %q; want %q", res.Stdout, "1|two|3.5\n")
	}
}

func TestMemoryHeader(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 -header :memory: "SELECT 1 AS a, 'two' AS b"`)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	want := "a|b\n1|two\n"
	if res.Stdout != want {
		t.Errorf("stdout = %q; want %q", res.Stdout, want)
	}
}

func TestCSVOutput(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 -csv -header :memory: "SELECT 1 AS a, 'has,comma' AS b"`)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	// encoding/csv uses CRLF line endings.
	want := "a,b\r\n1,\"has,comma\"\r\n"
	if res.Stdout != want {
		t.Errorf("stdout = %q; want %q", res.Stdout, want)
	}
}

func TestJSONOutput(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 -json :memory: "SELECT 1 AS a, 'two' AS b"`)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	// Expect a single-row array. The exact field order in JSON-encoded
	// maps is sorted alphabetically by Go's json package.
	want := `[{"a":1,"b":"two"}]` + "\n"
	if res.Stdout != want {
		t.Errorf("stdout = %q; want %q", res.Stdout, want)
	}
}

func TestLineMode(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 -line :memory: "SELECT 1 AS a, 'two' AS bb"`)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	// Width is max column-name length (2 from "bb").
	want := " a = 1\nbb = two\n"
	if res.Stdout != want {
		t.Errorf("stdout = %q; want %q", res.Stdout, want)
	}
}

func TestCreateInsertSelect(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	script := `sqlite3 -header :memory: "CREATE TABLE t(id INTEGER, name TEXT); INSERT INTO t VALUES (1,'alice'),(2,'bob'); SELECT * FROM t ORDER BY id"`
	res := exec(t, b, script)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	want := "id|name\n1|alice\n2|bob\n"
	if res.Stdout != want {
		t.Errorf("stdout = %q; want %q", res.Stdout, want)
	}
}

func TestStdinSQL(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	// SQL piped via stdin (no positional QUERY arg).
	res := exec(t, b, `echo "SELECT 42" | sqlite3 :memory:`)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	if res.Stdout != "42\n" {
		t.Errorf("stdout = %q; want %q", res.Stdout, "42\n")
	}
}

func TestHelp(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 --help`)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "Usage: sqlite3") {
		t.Errorf("help missing usage line: %q", res.Stdout)
	}
}

func TestNoDatabaseArg(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 2>&1; true`)
	if !strings.Contains(res.Stdout, "usage:") {
		t.Errorf("expected usage hint, got %q", res.Stdout)
	}
}

func TestUnknownFlag(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	// Capture stderr through redirection.
	res := exec(t, b, `sqlite3 -bogus :memory: "SELECT 1" 2>&1; echo exit=$?`)
	if !strings.Contains(res.Stdout, "unknown option") {
		t.Errorf("expected 'unknown option', got %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "exit=1") {
		t.Errorf("expected exit=1, got %q", res.Stdout)
	}
}

func TestFileDBRoundTrip(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	script := `cd /tmp 2>/dev/null || true
sqlite3 /tmp/test.db "CREATE TABLE t(id INTEGER); INSERT INTO t VALUES (1),(2),(3);"
sqlite3 /tmp/test.db "SELECT count(*) FROM t"`
	if err := b.FS().MkdirAll("/tmp", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	res := exec(t, b, script)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", res.ExitCode, res.Stderr, res.Stdout)
	}
	if res.Stdout != "3\n" {
		t.Errorf("stdout = %q; want %q", res.Stdout, "3\n")
	}
	// VFS now holds the persisted DB file.
	data, err := b.FS().ReadFile("/tmp/test.db")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("VFS DB is empty after write-back")
	}
	if !bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
		t.Errorf("VFS DB does not look like a SQLite file (prefix=%q)", string(data[:min(16, len(data))]))
	}
}

func TestFileDBSeedFromVFS(t *testing.T) {
	// Pre-seed a SQLite file in the VFS by creating one through a
	// throwaway Bash, then opening it from a fresh Bash and querying.
	src := newBash(t, sqlite.Options{})
	if err := src.FS().MkdirAll("/data", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	r := exec(t, src, `sqlite3 /data/seed.db "CREATE TABLE k(v); INSERT INTO k VALUES (99)"`)
	if r.ExitCode != 0 {
		t.Fatalf("seed exit=%d stderr=%q", r.ExitCode, r.Stderr)
	}
	dbBytes, err := src.FS().ReadFile("/data/seed.db")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	dst, err := gobash.New(gobash.BashOptions{
		Files: map[string]fs.FileInit{
			"/data/seed.db": {Content: dbBytes},
		},
	})
	if err != nil {
		t.Fatalf("gobash.New: %v", err)
	}
	if err := sqlite.Register(dst, sqlite.Options{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	res, err := dst.Exec(context.Background(), `sqlite3 /data/seed.db "SELECT v FROM k"`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	if res.Stdout != "99\n" {
		t.Errorf("stdout = %q; want %q", res.Stdout, "99\n")
	}
}

func TestTimeoutInterrupts(t *testing.T) {
	// 50 ms timeout vs an unconstrained recursive CTE that would
	// otherwise run for many seconds. The query must abort with the
	// timeout diagnostic.
	b := newBash(t, sqlite.Options{Timeout: 50 * time.Millisecond})
	script := `sqlite3 :memory: "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < 100000000) SELECT count(*) FROM c"`
	start := time.Now()
	res := exec(t, b, script)
	elapsed := time.Since(start)
	if res.ExitCode == 0 {
		t.Fatalf("expected non-zero exit on timeout, got exit=0 stdout=%q", res.Stdout)
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout did not interrupt: elapsed=%v", elapsed)
	}
}

func TestPragma(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 :memory: "PRAGMA encoding"`)
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		t.Errorf("PRAGMA encoding produced no output")
	}
}

func TestEmptySQL(t *testing.T) {
	b := newBash(t, sqlite.Options{})
	res := exec(t, b, `sqlite3 :memory: "" 2>&1; echo exit=$?`)
	if !strings.Contains(res.Stdout, "no SQL supplied") {
		t.Errorf("expected 'no SQL supplied', got %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "exit=1") {
		t.Errorf("expected exit=1, got %q", res.Stdout)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
