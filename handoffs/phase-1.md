# go-bash Handoff — Phase 1 complete

- **Date:** 2026-06-17
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit)
- **Gate:** make ci → PASS (tidy + build CGO_ENABLED=0 + vet + test-race + staticcheck all green; coverage 66.2%)

## What this phase delivered

- `go.mod` — module `github.com/mark3labs/go-bash`, `go 1.22`, single dependency `mvdan.cc/sh/v3 v3.13.1` (latest tagged; pinned). `go.sum` checked in.
- `doc.go` — package-level godoc; documents Phase-1 scope and the host-disk caveat that Phase 3 will close.
- `bash.go` (SPEC §1.2, §1.3) — `Bash` struct (mu, env, cwd, limits, sleep, logger, trace, procInfo); `New(BashOptions) (*Bash, error)`; `(*Bash).Exec(ctx, script, ExecOptions) (BashExecResult, error)` stubbed through `syntax.NewParser().Parse` → `interp.New` → `runner.Run(ctx, file)`. Internal helpers `wireStdio`, `cloneEnv`, `mergeEnv`, `envSlice`, `exportedEnv`, `defaultProcessInfo`.
- `options.go` (SPEC §1.2) — `BashOptions` (Phase-1 subset of the spec'd field set; comment names the phases that will add the rest in place), `ExecOptions` (full §1.2 surface, Args declared but unwired until Phase 5), `ProcessInfo`, `SleepFunc`, `TraceFunc`, `InvokeToolFunc`, `TraceEvent`, `Logger`, `JavaScriptConfig`, `PythonConfig` (empty body; `Runtime PythonRuntime` deferred to Phase 16 since `PythonRuntime` is defined there per §15).
- `result.go` (SPEC §1.2) — `ExecResult`, `BashExecResult` (embeds `ExecResult`, adds `Env`, `Metadata`).
- `errors.go` (SPEC §2.2) — `ExecutionLimitError`, `ParseError`, `LexerError`, `SecurityViolationError`, `PosixFatalError`, `ArithmeticError`, `ExitError`, `AbortedError`; each with an `Error() string` method. Consumers are expected to use `errors.As` (typed-error rule from AGENTS.md / SPEC §0.1).
- `limits.go` (SPEC §2.1) — `ExecutionLimits` (all pointer fields), `ResolvedLimits` (matching non-pointer fields), `DefaultLimits()`, `ResolveLimits(*ExecutionLimits) ResolvedLimits`. Default values written verbatim from the SPEC §2.1 table.
- `bash_test.go`, `limits_test.go` — Phase-1 acceptance tests (TDD-first; they were written and committed-to-mind before the implementation files).

## Acceptance criteria (from SPEC.md §1.4)

- [x] **`Bash` constructs with no options** — `TestNewZeroOptions` passes; `New(BashOptions{})` returns non-nil `*Bash`, nil error.
- [x] **`echo hello` → Stdout `"hello\n"`, ExitCode 0** — `TestExecEchoHello` passes; bytes-exact match.
- [x] **`exit 7` → ExitCode 7 (no error)** — `TestExecExitCode` passes. The runner returns an `interp.ExitStatus` wrapping the byte 7; we unwrap via `errors.As` and report it as `BashExecResult.ExitCode` without surfacing an error (matches SPEC §0.1: non-zero exit is not a harness failure).
- [x] **Mid-script `context.Cancel` → `context.Canceled`** — `TestExecMidScriptCancellation` runs `while true; do :; done` and cancels after 50ms. We translate `runner.Run` errors satisfying `errors.Is(err, context.Canceled)` back to the bare sentinel, so callers can `errors.Is` against it.
- [x] **Default `ResolvedLimits` exactly matches the SPEC §2.1 table** — `TestDefaultResolvedLimits` compares `ResolveLimits(nil)` against a frozen literal. `TestResolveLimitsOverrides` covers the per-field override + fall-through.
- [x] **`make ci` passes** — tidy + CGO-disabled build + vet + race-test + staticcheck all green.

## Tests

- Package: `github.com/mark3labs/go-bash` — 7 tests, all pass, 66.2% statement coverage.
  - Uncovered: `ReplaceEnv=true` branch in `mergeEnv`, the `context.DeadlineExceeded` branch (Phase 2 will exercise via real timeouts), and the parse-error branch (Phase 4 will cover when the parser surface lands). All three are simple guards; intentional non-coverage for now.
- Comparison fixtures: N/A this phase (harness lands in Phase 19; fixtures begin in Phase 10).

## Decisions & gotchas discovered

- **`interp.IsExitStatus` is deprecated in mvdan/sh v3.13.1.** Staticcheck flagged `SA1019`. Switched to `errors.As(err, &interp.ExitStatus{})`. Documented choice — keep using `errors.As`; do not reintroduce the deprecated helper anywhere.
- **`expand.Variable.String()` returns the textual form** for both scalar and array variables in v3.13.1. Used it in `exportedEnv`. May need refinement in Phase 5 once we want array semantics in `BashExecResult.Env`; flagged for that phase.
- **BashOptions forward-references intentionally omitted.** SPEC §1.2 sketches `BashOptions` with `Files map[string]FileInit`, `FS fs.FileSystem`, `Fetch network.Doer`, `Network *network.Config`, `Commands []command.Name`, and `CustomCommands []command.Command` — but `FileInit`, the `fs`, `network`, and `command` packages do not exist yet, and the kickoff explicitly forbade pre-building them. I omitted those fields and added a top-of-struct comment naming the phase that will add each one in place. This is a pure additive change for future phases (no rename, no semantic shift).
- **`Bash` struct fields likewise pared back.** The spec sketches `registry`, `fetch`, `funcs`, `exported`, `jsBoot`, `invoke`, `plugins` fields whose types come from later phases. Added per-field comment naming the introducing phase. `fs` field is intentionally absent until Phase 3 (the kickoff said "nil FS placeholder" is fine — I treated absence as the placeholder and called it out in the struct comment + the package doc).
- **`PythonConfig` is currently `struct{}`.** SPEC §1.2 says it holds `Runtime PythonRuntime` but §15 owns `PythonRuntime`. The empty struct freezes the type name now so `BashOptions.Python` already takes `*PythonConfig`; Phase 16 will add the `Runtime` field. No rename will be needed.
- **`ExecOptions.Args` is declared but unwired.** Spec defines it ("appended to the first command bypassing parsing"); Phase 5 (interp bridge) is the right place to wire it. Left a TODO comment in the struct.
- **`time/tzdata` blank import deferred.** The resolved-decisions block (item 6) tells us to embed the zoneinfo DB in `bash.go`. Phase 1 has no TZ-touching code paths, so importing `_ "time/tzdata"` now would bloat binaries with no benefit and would be flagged by `go vet`/staticcheck as unused if we don't actually consume it. Will land alongside the `date` builtin (Phase 10 wave) per AGENTS.md "no scope creep."
- **Cancellation guard.** `interp.Runner.Run` already returns `ctx.Err()` on cancellation, but to be defensive against future wrapping we strip down to the raw `context.Canceled` / `context.DeadlineExceeded` sentinels at the Exec boundary so `errors.Is` in caller code is bulletproof.

## Open follow-ups (non-blocking)

- Add `ReplaceEnv=true` test coverage in Phase 2 when limits-driven env mutation lands.
- Revisit `exportedEnv` array semantics in Phase 5.
- Wire `ExecOptions.Args` in Phase 5.
- Add `_ "time/tzdata"` blank import when the first wall-clock-formatting builtin lands.
- Add `BashOptions.{Files,FS}` (Phase 3), `BashOptions.{Commands,CustomCommands}` (Phase 8), `BashOptions.{Fetch,Network}` (Phase 9), and the spec'd internal `Bash` fields as their phases land.
- Add `Bash.FS()` and `Bash.RegisterTransformPlugin` (declared in SPEC §1.2) in Phase 3 and Phase 13 respectively.

## NEXT PHASE: Phase 2 — Execution Limits

- **Goal:** port `src/limits.ts` exactly and wire enforcement into the interpreter (SPEC §2).
- **Spec to read:** SPEC.md Phase 2 (§2.1 limits table — already mirrored in `limits.go` — and §2.3 wiring rules), and the resolved-decisions block at the end of SPEC.md (especially items 4–5 since the limit counters thread through awk/glob in later phases).
- **Packages/files to create:**
  - Wire counter middleware around `interp.Runner` — likely a new `internal/ringbuf` package (`ringbuf.LimitedWriter`) plus runner-side hooks.
  - Expand `bash.go::Exec` to install the limited-writer wrappers around the stdout/stderr passed to `interp.StdIO` and to install the call-depth / command-count / loop-iteration counters.
  - Keep limit fields in `limits.go` unchanged; only the *enforcement* is new.
- **Public symbols to deliver:** no new public surface — `ExecutionLimits`, `ResolvedLimits`, `ResolveLimits`, and `ExecutionLimitError` are already exported. Enforcement is internal.
- **Acceptance (SPEC §2.4):**
  - `while true; do :; done` aborts with `*ExecutionLimitError{Limit:"MaxLoopIterations"}` (no ctx cancel).
  - A script writing > `MaxOutputSize` bytes aborts with `*ExecutionLimitError{Limit:"MaxOutputSize"}`.
  - Deep recursive shell function aborts with `*ExecutionLimitError{Limit:"MaxCallDepth"}`.
  - Defaults table assertion (already covered by `TestDefaultResolvedLimits`) stays green.
- **Prerequisites:** met by Phase 1.
- **Kickoff prompt for the next session:**

  > Implement Phase 2 of go-bash per SPEC.md §2. Read AGENTS.md and HANDOFF.md first, then `handoffs/phase-1.md` for the Phase-1 decisions (especially the `interp.ExitStatus` / `errors.As` pattern and the deferred struct fields). Phase 2 is execution-limit enforcement — `limits.go` already holds the types and `DefaultLimits`/`ResolveLimits` per §2.1, and the §2.2 error types are already exported; your job is to wire the counters into the `mvdan/sh` runner inside `bash.go::Exec`. Create `internal/ringbuf/` with a `LimitedWriter` that wraps the user-supplied (or buffered) stdout/stderr and returns `*ExecutionLimitError{Limit:"MaxOutputSize"}` once the cap is hit. Hook `interp.CallHandler` to bump a call-depth counter on function entry/exit and to bump a command counter on every command, both bound to the Phase-2 acceptance criteria in SPEC §2.4. Loop-iteration counting will likely need a `mvdan/sh` callback per iteration of `for`/`while`/`until` — if the upstream API doesn't expose one cleanly, document the workaround in `handoffs/phase-2.md` and propose an alternative (e.g. an AST walker that wraps loop bodies). Write the SPEC §2.4 tests first (`TestMaxLoopIterations`, `TestMaxOutputSize`, `TestMaxCallDepth`), then the implementation. The default-table assertion is already green from Phase 1; do not modify it. Run `make ci` until green, then call `finalize_phase` with `phase=2`, the drafted handoff markdown, the new root `HANDOFF.md` pointer, a conventional-commit subject like `feat(phase-2): execution limit enforcement`, and the kickoff prompt for Phase 3 (filesystem layer).
