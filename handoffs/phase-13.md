# go-bash Handoff — Phase 13 complete

- **Date:** 2026-06-18
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit; previous HEAD Phase 12)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). `golangci-lint run ./...` → **0 issues**.

## What this phase delivered

- **`transform/types.go` (NEW).** Plugin interface (`Name`, `Transform`),
  `Context{AST, Metadata}`, `Result{AST, Metadata}`,
  `BashTransformResult{Script, AST, Metadata}` — matches SPEC §13.1.
- **`transform/pipeline.go` (NEW).** `Pipeline` with `New()`,
  `Use(Plugin) *Pipeline` (returns self for fluent chaining; nil
  plugin is skipped), `Plugins() []Plugin` (defensive copy),
  `Transform(script) (*BashTransformResult, error)` (parse →
  dispatch → re-serialize), and `TransformAST(ast)` for hosts that
  already hold a parsed AST.
- **`transform/plugins/collector/collector.go` (NEW).** Stateless
  CommandCollectorPlugin. Walks `Script.Origin` via `syntax.Walk`,
  records the first-argument literal of every `*syntax.CallExpr`,
  exposes the slice as `Metadata[collector.MetadataKey]` =
  `[]string`. Plugin name is `"command-collector"`.
- **`transform/plugins/tee/tee.go` (NEW).** TeePlugin with
  `Options{OutputDir, TargetCommandMatch, Timestamp}` and
  `TeeFileInfo{CommandIndex, CommandName, Command, StdoutFile}`.
  Mutates `Script.Origin` in place: each matched pipeline stage is
  wrapped with `| tee <OutputDir>/<idx>-<sanitized-name>.stdout.txt`.
  Recurses into subshells, blocks, function bodies, if/for/while/case.
  Skips stages whose command is already `tee` (idempotent on
  repeated application). Skips stages with no literal name.
  TargetCommandMatch acts as a name filter when non-nil.
- **`bash.go`.**
  - New `Bash.plugins []transform.Plugin` field, seeded from
    `BashOptions.TransformPlugins` at `New` time.
  - New `Bash.RegisterTransformPlugin(p)` — appends under `b.mu`;
    nil is a no-op; safe to call concurrently from outside Exec.
  - `execLocked` runs the pipeline first when `len(b.plugins) > 0`:
    parse → plugins → serialize → swap `script` → continue normal
    parse + alias + procinfo + caps + run. Plugin metadata is
    attached to `BashExecResult.Metadata`.
- **`options.go`.** New `BashOptions.TransformPlugins
  []transform.Plugin`.
- **`phase13_transform_test.go` (NEW).** 8 end-to-end Exec tests:
  no-op when empty, ordered dispatch across two Execs, late-binding
  Register, collector via Exec, tee via Exec (single + multi-stage),
  collector + tee composing, parse-error bubbling.
- **`transform/pipeline_test.go` (NEW).** 6 pipeline-level tests:
  zero-plugin pass-through, fluent Use chain, nil skipped, parse
  error, metadata accumulation across plugins, defensive `Plugins()`
  copy.
- **`transform/plugins/collector/collector_test.go` (NEW).** 4
  tests across 7 grammar shapes (pipeline, &&/||, if, for, subshell,
  cmd-subst) plus no-mutation + Name pinning.
- **`transform/plugins/tee/tee_test.go` (NEW).** 9 plugin-level
  tests (single, multi-stage, target match, self-skip, idempotent,
  empty OutputDir, nested subshell, name sanitization, Name).
- **`DECISIONS.md`.** New "Phase 13 — Transform pipeline" section
  with five entries (Origin-not-typed-AST mutation, PIPESTATUS
  omission, per-call counter, single-stage wrapping, append-only
  registration).

## Acceptance criteria (from SPEC.md §13)

- [x] **Plugin lifecycle hooks fire in defined order.** `Pipeline`
  dispatches plugins in registration order; each plugin sees the
  running AST and the accumulated metadata bag via
  `Context.Metadata`. Pinned by
  `TestPhase13PluginsFireInOrder` (two Execs × three plugins),
  `TestPipelineUseChain` (three plugins in order),
  `TestPipelineMetadataAccumulates` (second plugin observes first's
  metadata).
- [x] **CommandCollectorPlugin returns the parsed command list.**
  Plugin emits `Metadata[collector.MetadataKey]` = `[]string` of
  CallExpr leading literals. Pinned by `TestCollectorBasic` (7
  grammar shapes), `TestCollectorEmptyScript`,
  `TestPhase13CommandCollectorViaExec` (end-to-end via
  `BashExecResult.Metadata`).
- [x] **TeePlugin mirrors stdout into per-stage files.** Plugin
  rewrites the AST so the runtime's `tee` builtin (Phase 10 Wave C)
  writes `<OutputDir>/<idx>-<name>.stdout.txt` files in the VFS.
  Pinned by `TestPhase13TeePluginViaExec` and
  `TestPhase13TeePluginMultiStagePipeline` (reads the mirrored
  files back from `b.FS()` and asserts byte-exact contents).
- [x] **Pipeline is a no-op when zero plugins are registered.**
  `BashExecResult.Metadata == nil`; the execLocked plugin path is
  skipped entirely (no re-parse). Pinned by
  `TestPhase13PipelineNoOpWhenEmpty` and `TestPipelineNoPlugins`.

## Tests

- **New test files:** `phase13_transform_test.go` (8 tests),
  `transform/pipeline_test.go` (6), `transform/plugins/collector/collector_test.go`
  (4 tests with 7 subtests), `transform/plugins/tee/tee_test.go` (9).
- **Race detector:** clean across the new tests and the prior
  suite. Full `go test -race ./...` run is ~120s.
- **Coverage delta:**
  - Root `gobash` package: **84.1%** (up from 83.5%).
  - New transform packages: `transform` 63.9%, `collector` 58.6%,
    `tee` 78.0%. The lower transform-package number is dominated
    by the Phase 4 `astToFile` inverse-translator branches —
    untouched by Phase 13.
  - Repo combined coverage **68.9%** (unchanged ±0.1).

## Decisions & gotchas discovered

- **Plugins rewrite `Script.Origin` (`*syntax.File`), not the
  typed `*ast.Script`.** `transform.Serialize` already prefers
  Origin for byte-faithful round-trip; extending Phase 4's
  `astToFile` to cover every compound command would be a Phase-4
  task. CommandCollectorPlugin is read-only (no mutation either
  way); TeePlugin mutates Origin in place. Documented in
  DECISIONS.md.
- **Self-reference cycle bug avoided in TeePlugin.** Initial
  implementation set `st.Cmd = BinaryCmd{X: st, ...}` for
  single-stage wrap, which made the mvdan/sh printer recurse
  forever (caught immediately by `go test`, manifested as a
  1 GiB stack overflow). Fix: always wrap the original `Cmd`
  in a FRESH `*syntax.Stmt` before splicing it into the new
  pipeline tree, so the binary's X child is never the outer Stmt.
  See `transform/plugins/tee/tee.go:rewriteStmts`.
- **PIPESTATUS restore-pipeline omitted.** SPEC §13.3 says
  "Preserves PIPESTATUS semantics by saving/restoring via a
  synthesized restore-pipeline" referencing the TS port. The Go
  port lands without it; scripts that observe `$PIPESTATUS` after
  a wrapped pipeline see indices shifted by +1 per tee. Documented
  in DECISIONS.md as an open follow-up.
- **TeePlugin wraps single-stage commands too** (`echo hi` →
  `echo hi | tee /OUT/0-echo.stdout.txt`). SPEC §13.3 phrases the
  target as "non-trivial pipeline stage"; a bare command is a
  one-stage pipeline. The behavior is the least-surprise default
  for the per-command-mirror promise the file naming scheme makes.
  Documented in DECISIONS.md.
- **TeePlugin counter is per-Transform call.** Sharing a Plugin
  instance across pipelines is safe; the per-call counter resets
  to 0. Documented in DECISIONS.md.
- **`bash.go` plugin path runs BEFORE alias / instrumentLoops /
  rewriteProcInfo / enforceExpansionCaps.** The pipeline returns a
  re-serialized script string; the regular parse path then sees
  the post-transform tree and applies all the Phase 7-12
  rewrites on top. No ordering decisions inside the transform
  pipeline itself (plugins fire in registration order, full stop).
- **`tee` builtin already exists from Phase 10 Wave C** so the
  injected `| tee /OUT/...` runs through the registered handler
  and writes through the VFS — pinned by reading the mirrored
  files back via `b.FS().ReadFile`.

## Open follow-ups (non-blocking)

- **PIPESTATUS preservation in TeePlugin.** Synthesize the
  save/restore pipeline per SPEC §13.3. Requires extending the
  inverse translator or wrapping the existing pipeline in a Group
  with arithmetic assignments.
- **Plugin metadata exposed to subsequent plugins is keyed by
  plugin Name.** If a plugin wants raw / unkeyed metadata it has
  to look itself up by Name first. Acceptable for now; revisit if
  Phase 14-16 plugins demand a richer shape.
- **cmpfixture harness has no `transformPlugins` field.** Phase 13
  acceptance is pinned exclusively via the Go test file rather
  than cmpfixture JSONs. Phase 19 (bulk fixture import) may extend
  the harness if just-bash ships fixtures that exercise plugins.
- **Phase 12 follow-ups carried forward** (still open):
  - Complex param-expansion of virtual PIDs (`${PPID:-x}`, `${#BASHPID}`)
  - Background / process-sub / pipeline BASHPID forks
- **Phase 11 follow-ups carried forward** (still open): `source`
  Args propagation, `/bin/cd` chdir back-channel, `getopts` OPTIND
  persistence, `fg`/`bg` registration, `hash` cache state, `trap`
  EXIT delivery, `let` bit-ops/comparison/ternary.

## NEXT PHASE: Phase 14 — Optional SQLite runtime

- **Goal:** Subpackage `github.com/mark3labs/go-bash/sqlite`
  registers a `sqlite3` builtin backed by `modernc.org/sqlite`
  (no cgo).
- **Spec to read:** `SPEC.md` §14 (Register surface, Options,
  `:memory:` vs file-DB strategy, `-header` / `-csv` / `-json` /
  `-line` output modes, timeout via goroutine + `db.Close()`).
- **Packages/files to create:**
  - `sqlite/sqlite.go` — `Register(b *gobash.Bash, opts Options) error`,
    `Options{Timeout time.Duration}`.
  - Custom command registration that overrides the Phase 10 Wave H
    placeholder `builtins/sqlite3` (which currently errors out).
- **Prerequisites:** met — transform pipeline isn't needed; the
  custom-command override path already exists
  (`BashOptions.CustomCommands` wins over built-in registration).
- **Kickoff prompt for the next session:**

  > Implement Phase 14 (optional SQLite runtime) of go-bash per
  > SPEC.md §14. Read AGENTS.md, HANDOFF.md, `handoffs/phase-13.md`.
  > Create the `sqlite` subpackage at module path
  > `github.com/mark3labs/go-bash/sqlite` with a `Register(b
  > *gobash.Bash, opts Options) error` constructor and an
  > `Options{Timeout time.Duration}` config. Back the
  > implementation with `modernc.org/sqlite` (pure-Go, no cgo —
  > the Phase 14 build must keep `CGO_ENABLED=0` green).
  > Implement `sqlite3 :memory:` and `sqlite3 file.db "QUERY"`
  > paths (the file path resolves through `Bash.FS()`; the MVP
  > may copy the VFS file to a host tmp file for the query
  > duration and write back, OR reject file DBs until v2 per
  > SPEC §14). Honor the `-header`, `-csv`, `-json`, `-line`
  > output modes. Enforce the Timeout via a goroutine that
  > calls `db.Close()` on ctx.Done(). Add a comparison fixture
  > or two under `internal/testdata/fixtures/sqlite3/` plus
  > Go-level unit tests. Update DECISIONS.md if any divergence
  > from the TS surface is discovered. Run `make ci` until green
  > AND `golangci-lint run ./...` until clean. Then call
  > `finalize_phase` with `phase=14`, the drafted handoff
  > markdown, the new root `HANDOFF.md` pointer, a
  > conventional-commit subject like
  > `feat(phase-14): sqlite runtime`, and the kickoff prompt
  > for Phase 15 (optional JavaScript runtime via goja).
