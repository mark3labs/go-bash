# go-bash Handoff — Phase 14 complete

- **Date:** 2026-06-18
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit; previous HEAD Phase 13)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). `golangci-lint run ./...` → **0 issues**.

## What this phase delivered

- **`sqlite/sqlite.go` (NEW).** Optional sqlite3 runtime backed by
  `modernc.org/sqlite` (pure Go, no cgo). Surface:
  - `Options{Timeout time.Duration}` — 0 falls back to
    `ResolvedLimits.MaxSqliteTimeout` (5 s default from Phase 2).
  - `Register(b *gobash.Bash, opts Options) error` — installs the
    real `sqlite3` command on `b.Registry()`, overriding the Phase 10
    Wave H placeholder via last-writer-wins. Nil Bash returns
    `ErrNilBash`.
  - Argv parsing: `--help`, `-header`, `-noheader`, `-csv`, `-json`,
    `-line`, `-list`; unknown `-*` flags exit 1 with diagnostic.
    Positional `DATABASE [SQL...]`; missing SQL falls through to
    reading `c.Stdin`; empty trimmed query → "no SQL supplied".
  - `:memory:` (and empty string) opens an in-memory DB. Any other
    path is resolved via `builtinutil.ResolvePath(c.Cwd, dbArg)`,
    shuttled through `os.MkdirTemp` + `os.WriteFile`, then written
    back to the VFS via `c.FS.WriteFile` on cleanup. Missing source
    file is fine — sqlite creates it on first write.
  - Output formats:
    - **list** (default): `col0|col1|...|colN\n` per row. `-header`
      prepends a header row.
    - **csv**: `csv.Writer.UseCRLF = true` (RFC 4180 line endings).
      `-header` prepends header row.
    - **json**: single `json.Encode` of a `[]map[string]any` (Go
      sorts keys alphabetically — a documented divergence from
      column-ordered TS output).
    - **line**: `<width-padded name> = <value>\n` per column,
      blank-line-separated rows.
  - Multi-statement support via a manual semicolon-tokenizer
    (`splitStatements`) that respects single-quoted literals (with
    `''` escape), double-quoted identifiers, `--` line comments, and
    `/* ... */` block comments. SELECT/WITH/VALUES/PRAGMA/EXPLAIN
    statements route to `QueryContext`; everything else to
    `ExecContext`.
  - Timeout: `context.WithTimeout(ctx, timeout)` + a goroutine that
    calls `db.Close()` on `ctx.Done()`. A `done` channel races the
    happy-path so the cleanup goroutine exits without double-closing
    a still-good DB.
- **`sqlite/sqlite_test.go` (NEW).** 14 Go-level unit tests:
  Register nil Bash, list mode, header, CSV, JSON, line mode,
  CREATE-INSERT-SELECT, stdin-piped SQL, help, no-DB-arg usage
  error, unknown flag, file-DB round-trip via VFS, file-DB seed
  from VFS (cross-Bash), timeout interrupts a recursive CTE,
  PRAGMA, empty SQL rejection.
- **`sqlite/comparison_test.go` (NEW).** Local fixture runner that
  loads `*.json` from `internal/testdata/fixtures/sqlite3/` via
  `cmpfixture.Load` and runs them against a Bash with
  `sqlite.Register` pre-applied. `cmpfixture.RunDir` could not be
  reused as-is because it constructs the Bash internally and has no
  setup hook for `Register`.
- **`internal/testdata/fixtures/sqlite3/` (NEW: 5 fixtures).**
  `memory_header.json`, `memory_json.json`, `memory_line.json`,
  `memory_csv.json`, `create_insert_select.json`. The original
  stub fixture (`basic.json`) moved to a new sibling dir.
- **`internal/testdata/fixtures/sqlite3-stub/basic.json` (RENAMED).**
  Phase 10 Wave H stub fixture relocated so the stub and runtime
  comparison tests can coexist.
- **`builtins/sqlite3/sqlite3_comparison_test.go`.** Path updated
  to point at the new `sqlite3-stub/` directory.
- **`go.mod` / `go.sum`.** Added `modernc.org/sqlite v1.52.0`
  (pure Go) and its transitive deps: `modernc.org/libc`,
  `modernc.org/mathutil`, `modernc.org/memory`,
  `github.com/dustin/go-humanize`, `github.com/google/uuid`,
  `github.com/ncruces/go-strftime`,
  `github.com/remyoudompheng/bigfft`. All transitive deps are
  pure Go.
- **`DECISIONS.md`.** New "Phase 14 — Optional SQLite runtime"
  section with 5 entries: file-DB shuttle, output-mode defaults
  (CSV CRLF + JSON key sort), timeout via `db.Close()`, stub
  fixture directory split, stdin-piped SQL.

## Acceptance criteria (from SPEC.md §14)

- [x] **`sqlite/` subpackage exports `Register(b *gobash.Bash, opts
  Options) error` and `Options{Timeout time.Duration}`.** Pinned by
  `TestRegisterNilBash` and every other test that calls Register.
- [x] **`sqlite3 :memory:` works.** Pinned by
  `TestMemoryListMode`, `TestMemoryHeader`, `TestCSVOutput`,
  `TestJSONOutput`, `TestLineMode`, `TestCreateInsertSelect`,
  `TestStdinSQL`, `TestPragma`, plus the comparison fixtures.
- [x] **`sqlite3 file.db "QUERY"` resolves through `Bash.FS()`.**
  Pinned by `TestFileDBRoundTrip` (writes to /tmp/test.db via the
  runtime, reads the persisted SQLite file back from the VFS and
  checks the magic prefix) and `TestFileDBSeedFromVFS`
  (cross-Bash: writes via Bash A, reads via Bash B seeded from the
  serialized bytes).
- [x] **Output modes match.** `-header`, `-noheader`, `-csv`,
  `-json`, `-line`, `-list` each have dedicated tests + fixtures.
- [x] **Timeout interrupts a long query.** Pinned by
  `TestTimeoutInterrupts` (50 ms timeout vs an unbounded recursive
  CTE — completes in < 2 s with non-zero exit).
- [x] **CGO_ENABLED=0 build stays green.** The Makefile `build`
  target enforces `CGO_ENABLED=0 go build ./...`; `make ci` passes.

## Tests

- **New test files:** `sqlite/sqlite_test.go` (14 tests),
  `sqlite/comparison_test.go` (5 sub-tests across 5 fixtures).
- **Race detector:** clean on the new subpackage
  (`go test -race ./sqlite/...` ~1.2 s).
- **Coverage:** `sqlite` package: **75.5%** of statements.
  Repo combined coverage **69.1%** (essentially unchanged: the new
  subpackage is small relative to the whole tree).
- **Existing stub test still green.** `builtins/sqlite3` keeps
  100% coverage; only the fixture dir path changed.

## Decisions & gotchas discovered

- **File DBs shuttle through `os.MkdirTemp`.** modernc.org/sqlite
  takes a real OS path; it cannot open against `io/fs`. The
  cleanup writes the host file bytes back to the VFS unconditionally
  (even on query error) so partial mutations are not silently lost.
  Documented in DECISIONS.md.
- **CSV uses CRLF line endings.** `csv.Writer.UseCRLF = true` so
  output matches `sqlite3 -csv`. Default Go `csv.Writer` uses LF.
- **JSON key order is sorted, not column-ordered.** Go's stdlib
  `encoding/json` sorts map keys alphabetically; the TS port
  emits column-ordered keys. Hosts that need column order should
  fall back to `-csv` or use `json_object()` in SQL.
- **Timeout = `context.WithTimeout` + `db.Close()` on done.**
  Closing the DB aborts in-flight `QueryContext` calls on modernc.
  `QueryContext` cancellation alone did NOT interrupt recursive
  CTEs in practice — the close is what trips the abort. The
  goroutine uses a `done` channel to exit cleanly on the happy path.
- **`done`/`closed` channel pair.** The closer goroutine
  always closes `closed` so the main path can wait for it before
  returning, preventing a leaked goroutine even when ctx never
  fires. Both `done` and `closed` are unbuffered and closed exactly
  once; no `select` race.
- **`isQuery` heuristic.** SELECT / WITH / VALUES / PRAGMA /
  EXPLAIN → `QueryContext`; everything else → `ExecContext`.
  False negatives (e.g. `CREATE ... RETURNING`) would lose output
  but not error out. Documented as a Phase-21-shaped hardening
  follow-up if it ever bites.
- **Stub fixture relocated.** The Phase 10
  `builtins/sqlite3/sqlite3_comparison_test.go` comparison test
  now reads from `internal/testdata/fixtures/sqlite3-stub/`. The
  original `sqlite3/` dir is reserved for real-runtime fixtures so
  the `sqlite/` package's own harness can run against them with
  `sqlite.Register` pre-applied.
- **`cmpfixture` had no setup hook.** Rather than mutate the
  shared harness, the `sqlite/comparison_test.go` test file
  inlines a small fixture-runner that reuses `cmpfixture.Load`
  and `cmpfixture.Fixture` but constructs the Bash itself so
  `sqlite.Register` runs before Exec. If Phases 15/16 need the
  same shape, the next iteration of cmpfixture should grow a
  `WithSetup(func(*Bash) error)` knob.
- **modernc.org/sqlite pulls in ~7 transitive deps.** All are
  pure Go (modernc.org/libc is a transpiled glibc shim). No
  cgo. `make ci` build target verified.

## Open follow-ups (non-blocking)

- **JSON column-order divergence.** Either swap to a custom
  encoder that preserves column order (mirror the TS port) or
  document permanently. Currently documented in DECISIONS.md.
- **`isQuery` heuristic miss for `... RETURNING` and CTE-driven
  DML.** Add a try-query-then-exec fallback or parse the
  statement properly.
- **File-DB shuttle has no concurrency control.** Two parallel
  sqlite3 invocations against the same VFS path race the
  cleanup write-back; last writer wins. Phase 17 sandbox could
  add a per-path lock.
- **Per-statement output formatting.** Multiple row-producing
  statements in one query string all dump to stdout consecutively
  with no delimiter. Real sqlite3 CLI also does this; documented
  here for the record.
- **MaxJqIterations-style query limit for sqlite.** SPEC lists
  `MaxSqliteTimeout` only — no row / step cap. Hosts that need
  defense against unbounded result sets must wrap with `LIMIT` in
  their own SQL.
- **Phase 13 follow-ups carried forward** (still open):
  PIPESTATUS-restore pipeline in TeePlugin; richer plugin
  metadata shape.
- **Phase 12 / 11 follow-ups carried forward** (still open).

## NEXT PHASE: Phase 15 — Optional JavaScript runtime (`jsexec`)

- **Goal:** Subpackage `github.com/mark3labs/go-bash/jsexec`
  registers a `js-exec` builtin (with `node` alias) backed by
  `github.com/dop251/goja`. SPEC §15 freezes the surface.
- **Spec to read:** `SPEC.md` §15. Register signature is
  `Register(b *gobash.Bash, cfg gobash.JavaScriptConfig) error`
  (NB: `gobash.JavaScriptConfig` does not yet exist — the public
  type lives in the root package per SPEC §15; Phase 15 lands it
  alongside the subpackage). Features: `-c CODE`, FILE, `-m`
  ESM emulation via require shim, `console.{log,error,warn}`,
  `fetch` (gated on `cfg.InvokeTool` and the Phase 9 network
  Doer), `Buffer` / `URL` / `URLSearchParams` shims, `fs` subset
  (`readFileSync`, `writeFileSync`, `readdirSync`, `statSync`,
  `existsSync`, `mkdirSync`, `rmSync`), `path` (`join`,
  `resolve`, `dirname`, `basename`, `extname`, `relative`,
  `normalize`), `child_process` (`execSync` / `spawnSync` routed
  through `Context.Exec`), `process` (argv, cwd, exit, env,
  platform="linux", version="v18.0.0"), and a `tools` global
  proxy that builds a dot path and calls
  `Context.InvokeTool(ctx, path, argsJSON)` when set.
- **Timeout:** Use `goja.Runtime.Interrupt(reason)` after
  `MaxJsTimeout` (default 10 s, from `ResolvedLimits`).
- **Packages/files to create:**
  - `jsexec/jsexec.go` — `Register(b, cfg) error`.
  - `jsexec/console.go`, `jsexec/fs.go`, `jsexec/path.go`,
    `jsexec/process.go`, `jsexec/child_process.go`,
    `jsexec/fetch.go`, `jsexec/buffer.go`, `jsexec/url.go`,
    `jsexec/tools.go` — one shim per Node-ish global.
  - Tests: `jsexec/*_test.go` + fixtures under
    `internal/testdata/fixtures/js-exec/`.
- **Prerequisites:** met — `command.Context` already has
  `Fetch`, `Exec`, `InvokeTool`. The `JavaScriptConfig` type
  needs to be added to the root `gobash` package (one struct
  in `options.go`).
- **Kickoff prompt for the next session:** see the
  `kickoff_prompt` argument passed to `finalize_phase`.
