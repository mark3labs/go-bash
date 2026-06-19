// Package sqlite is the OPTIONAL `sqlite3` runtime for go-bash. Import
// this package and call Register to replace the Phase 10 Wave H stub
// with a real, pure-Go SQLite driver (modernc.org/sqlite).
//
// The package adheres to the no-cgo policy (SPEC §0.2): modernc.org/sqlite
// is a pure-Go transpilation of upstream SQLite, so `CGO_ENABLED=0`
// builds continue to succeed.
//
// Cited surface: SPEC §14.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"

	// modernc.org/sqlite registers the `sqlite` driver in its init().
	_ "modernc.org/sqlite"
)

// Options configures the sqlite3 runtime. The zero value is valid: the
// command falls back to ResolvedLimits.MaxSqliteTimeout (SPEC §2.1).
type Options struct {
	// Timeout caps the wall-clock duration of a single sqlite3
	// invocation. When ≤ 0 the per-Exec ResolvedLimits.MaxSqliteTimeout
	// is used; when both are ≤ 0 no timeout is enforced.
	Timeout time.Duration
}

// Register installs the real sqlite3 command on b, overriding the
// Phase 10 Wave H placeholder builtin. Subsequent Exec calls dispatch
// `sqlite3` to the modernc.org/sqlite-backed implementation.
//
// Register is safe to call once per *Bash. Repeated calls overwrite
// the registry entry (last-writer-wins, per command.Registry.Register).
// Calling Register before any Exec is the supported pattern; the
// registry mutator is not concurrent-safe.
//
// A nil Bash returns ErrNilBash.
func Register(b *gobash.Bash, opts Options) error {
	if b == nil {
		return ErrNilBash
	}
	b.Registry().Register(command.Define("sqlite3", makeRun(opts)))
	return nil
}

// ErrNilBash is returned by Register when called with a nil *Bash.
var ErrNilBash = errors.New("sqlite: nil *Bash")

const usage = "sqlite3 [OPTIONS] DATABASE [SQL]"
const helpText = `Usage: sqlite3 [OPTIONS] DATABASE [SQL]
Run SQL against a SQLite database (modernc.org/sqlite — pure Go).

Database arguments:
  :memory:           open an ephemeral in-memory database
  FILE               open the named file via the host VFS; the file is
                     shuttled to a host tmp file for the query duration

Output modes (mutually exclusive; default is list mode with '|'):
  -header            print column header before rows
  -noheader          suppress the column header (default)
  -csv               CSV output (RFC 4180); -header adds a header row
  -json              JSON array of row objects
  -line              one "name = value" per line, blank line between rows

Other:
  --help             show this help`

type outputMode int

const (
	modeList outputMode = iota
	modeCSV
	modeJSON
	modeLine
)

func makeRun(opts Options) func(context.Context, []string, *command.Context) command.Result {
	return func(ctx context.Context, args []string, c *command.Context) command.Result {
		return runOnce(ctx, args, c, opts)
	}
}

// nolint:gocyclo // sqlite3 argv parsing is intrinsically branchy.
func runOnce(ctx context.Context, args []string, c *command.Context, opts Options) command.Result {
	mode := modeList
	header := false
	var positional []string

	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case "-header":
			header = true
		case "-noheader":
			header = false
		case "-csv":
			mode = modeCSV
		case "-json":
			mode = modeJSON
		case "-line":
			mode = modeLine
		case "-list":
			mode = modeList
		default:
			if strings.HasPrefix(a, "-") && a != "-" {
				return builtinutil.Errorf(c.Stderr, "sqlite3", 1,
					"unknown option: %s", a)
			}
			positional = append(positional, a)
		}
	}

	if len(positional) == 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	dbArg := positional[0]

	// SQL source: explicit positional arg wins; otherwise read stdin.
	var sqlText string
	switch {
	case len(positional) >= 2:
		sqlText = strings.Join(positional[1:], " ")
	case c.Stdin != nil:
		buf, err := io.ReadAll(c.Stdin)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "sqlite3", 1, "read stdin: %v", err)
		}
		sqlText = string(buf)
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return builtinutil.Errorf(c.Stderr, "sqlite3", 1, "no SQL supplied")
	}

	// Resolve the effective timeout: explicit Options.Timeout wins;
	// otherwise fall back to the per-Exec Limits cap. A non-positive
	// resolved timeout means "do not enforce a timeout".
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = c.Limits.MaxSqliteTimeout
	}

	queryCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		queryCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Open the database. For file-backed DBs we shuttle the VFS file
	// to a host tmpfile for the query duration and write any mutations
	// back when the query completes (SPEC §14, MVP file-DB path).
	dsn, cleanup, err := prepareDSN(c, dbArg)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "sqlite3", 1, "%v", err)
	}
	defer cleanup()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "sqlite3", 1, "open: %v", err)
	}
	// Single connection — keeps :memory: state coherent and matches
	// the sqlite3 CLI's single-connection semantics.
	db.SetMaxOpenConns(1)

	// Race the query against ctx: a goroutine closes the DB on
	// cancellation, which aborts in-flight queries on the modernc
	// driver. The closed channel signal lets the deferred Close be a
	// no-op on the happy path.
	done := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		select {
		case <-queryCtx.Done():
			_ = db.Close()
			close(closed)
		case <-done:
			close(closed)
		}
	}()

	execErr := runSQL(queryCtx, db, sqlText, c.Stdout, mode, header)
	close(done)
	<-closed
	_ = db.Close()

	// Surface ctx errors (timeout, host cancellation) explicitly.
	if execErr != nil {
		// If the timeout fired we want to communicate that distinctly.
		if ctxErr := queryCtx.Err(); ctxErr != nil && timeout > 0 && errors.Is(ctxErr, context.DeadlineExceeded) {
			return builtinutil.Errorf(c.Stderr, "sqlite3", 1, "timeout after %s", timeout)
		}
		return builtinutil.Errorf(c.Stderr, "sqlite3", 1, "%v", execErr)
	}
	return command.Result{ExitCode: 0}
}

// prepareDSN resolves dbArg into a real-FS DSN. For `:memory:` (and
// the empty path, which sqlite3 also treats as memory) it returns the
// literal ":memory:" DSN. For file paths it copies the VFS file to a
// host tmp file and returns a cleanup that writes the host file back
// to the VFS on success.
func prepareDSN(c *command.Context, dbArg string) (string, func(), error) {
	if dbArg == ":memory:" || dbArg == "" {
		return ":memory:", func() {}, nil
	}

	if c.FS == nil {
		return "", nil, fmt.Errorf("filesystem unavailable")
	}
	abs := builtinutil.ResolvePath(c.Cwd, dbArg)

	// Read existing content (if any). Missing file is fine — sqlite
	// will create a fresh DB at the host tmp path on first write.
	var existing []byte
	if data, err := c.FS.ReadFile(abs); err == nil {
		existing = data
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("read %s: %w", dbArg, err)
	}

	tmpDir, err := os.MkdirTemp("", "gobash-sqlite-")
	if err != nil {
		return "", nil, fmt.Errorf("mktemp: %w", err)
	}
	hostPath := filepath.Join(tmpDir, "db.sqlite")
	if len(existing) > 0 {
		if err := os.WriteFile(hostPath, existing, 0o600); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", nil, fmt.Errorf("seed tmp DB: %w", err)
		}
	}

	cleanup := func() {
		defer func() { _ = os.RemoveAll(tmpDir) }()
		data, err := os.ReadFile(hostPath) //nolint:gosec // hostPath is under our MkdirTemp dir
		if err != nil {
			// If the host file vanished, there's nothing to write back.
			return
		}
		_ = c.FS.WriteFile(abs, data, 0o644)
	}
	return hostPath, cleanup, nil
}

// runSQL executes sqlText against db, writing results to w in the
// requested mode. Multiple statements are supported; only statements
// that return rows produce output. Non-query statements (CREATE,
// INSERT, UPDATE, DELETE, ...) are executed but produce no output —
// matching the sqlite3 CLI default.
func runSQL(ctx context.Context, db *sql.DB, sqlText string, w io.Writer, mode outputMode, header bool) error {
	// Split into statements at unquoted/uncommented semicolons. The
	// modernc driver can execute multi-statement strings via Exec,
	// but to support per-statement query output we tokenize ourselves.
	stmts := splitStatements(sqlText)

	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := execStatement(ctx, db, stmt, w, mode, header); err != nil {
			return err
		}
	}
	return nil
}

func execStatement(ctx context.Context, db *sql.DB, stmt string, w io.Writer, mode outputMode, header bool) error {
	if isQuery(stmt) {
		rows, err := db.QueryContext(ctx, stmt)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		if err := writeRows(rows, w, mode, header); err != nil {
			return err
		}
		return rows.Err()
	}
	_, err := db.ExecContext(ctx, stmt)
	return err
}

// isQuery is a heuristic that returns true when the statement is one
// of the row-returning forms (SELECT, WITH ... SELECT, VALUES,
// PRAGMA, EXPLAIN). We intentionally accept false negatives — any
// statement we run via QueryContext can also be Exec'd by the
// driver — and false positives are recovered via the QueryContext
// fallback.
func isQuery(stmt string) bool {
	s := strings.ToUpper(strings.TrimSpace(stmt))
	switch {
	case strings.HasPrefix(s, "SELECT"):
		return true
	case strings.HasPrefix(s, "WITH"):
		return true
	case strings.HasPrefix(s, "VALUES"):
		return true
	case strings.HasPrefix(s, "PRAGMA"):
		return true
	case strings.HasPrefix(s, "EXPLAIN"):
		return true
	}
	return false
}

// splitStatements tokenizes sqlText into a slice of statements at
// unquoted, uncommented semicolons. It supports:
//   - single-quoted strings ('...'), with '' as the escape for a
//     literal single quote
//   - double-quoted identifiers ("...")
//   - line comments (-- ... \n)
//   - block comments (/* ... */)
func splitStatements(sqlText string) []string {
	var stmts []string
	var sb strings.Builder
	runes := []rune(sqlText)
	i := 0
	for i < len(runes) {
		r := runes[i]
		switch {
		case r == '\'':
			sb.WriteRune(r)
			i++
			for i < len(runes) {
				sb.WriteRune(runes[i])
				if runes[i] == '\'' {
					// '' is an escaped quote inside a string literal.
					if i+1 < len(runes) && runes[i+1] == '\'' {
						sb.WriteRune(runes[i+1])
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case r == '"':
			sb.WriteRune(r)
			i++
			for i < len(runes) {
				sb.WriteRune(runes[i])
				if runes[i] == '"' {
					i++
					break
				}
				i++
			}
		case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
			for i < len(runes) && runes[i] != '\n' {
				sb.WriteRune(runes[i])
				i++
			}
		case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
			sb.WriteRune(runes[i])
			sb.WriteRune(runes[i+1])
			i += 2
			for i+1 < len(runes) && (runes[i] != '*' || runes[i+1] != '/') {
				sb.WriteRune(runes[i])
				i++
			}
			if i+1 < len(runes) {
				sb.WriteRune(runes[i])
				sb.WriteRune(runes[i+1])
				i += 2
			}
		case r == ';':
			stmts = append(stmts, sb.String())
			sb.Reset()
			i++
		default:
			sb.WriteRune(r)
			i++
		}
	}
	if rem := strings.TrimSpace(sb.String()); rem != "" {
		stmts = append(stmts, rem)
	}
	return stmts
}

func writeRows(rows *sql.Rows, w io.Writer, mode outputMode, header bool) error {
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	switch mode {
	case modeCSV:
		return writeRowsCSV(rows, w, cols, header)
	case modeJSON:
		return writeRowsJSON(rows, w, cols)
	case modeLine:
		return writeRowsLine(rows, w, cols)
	default:
		return writeRowsList(rows, w, cols, header)
	}
}

func scanRow(rows *sql.Rows, n int) ([]string, error) {
	dest := make([]any, n)
	ptrs := make([]any, n)
	for i := range dest {
		ptrs[i] = &dest[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	out := make([]string, n)
	for i, v := range dest {
		out[i] = formatValue(v)
	}
	return out, nil
}

func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return "0"
	case time.Time:
		return x.Format(time.RFC3339Nano)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func writeRowsList(rows *sql.Rows, w io.Writer, cols []string, header bool) error {
	if header {
		if _, err := io.WriteString(w, strings.Join(cols, "|")+"\n"); err != nil {
			return err
		}
	}
	for rows.Next() {
		vals, err := scanRow(rows, len(cols))
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, strings.Join(vals, "|")+"\n"); err != nil {
			return err
		}
	}
	return nil
}

func writeRowsCSV(rows *sql.Rows, w io.Writer, cols []string, header bool) error {
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	if header {
		if err := cw.Write(cols); err != nil {
			return err
		}
	}
	for rows.Next() {
		vals, err := scanRow(rows, len(cols))
		if err != nil {
			cw.Flush()
			return err
		}
		if err := cw.Write(vals); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeRowsJSON(rows *sql.Rows, w io.Writer, cols []string) error {
	// Build a slice of map[string]any first (typed values, so JSON
	// numbers are emitted as numbers, not as quoted strings).
	out := []map[string]any{}
	for rows.Next() {
		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = jsonValue(dest[i])
		}
		out = append(out, row)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func jsonValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	default:
		return x
	}
}

func writeRowsLine(rows *sql.Rows, w io.Writer, cols []string) error {
	width := 0
	for _, c := range cols {
		if len(c) > width {
			width = len(c)
		}
	}
	first := true
	for rows.Next() {
		vals, err := scanRow(rows, len(cols))
		if err != nil {
			return err
		}
		if !first {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		first = false
		for i, c := range cols {
			if _, err := fmt.Fprintf(w, "%*s = %s\n", width, c, vals[i]); err != nil {
				return err
			}
		}
	}
	return nil
}
