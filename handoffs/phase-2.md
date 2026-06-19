# go-bash Handoff — Phase 2 complete

- **Date:** 2026-06-17
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). Coverage: 77.1% root package, 90.0% `internal/ringbuf`, 79.3% total.

## What this phase delivered

- **`internal/ringbuf/ringbuf.go`** — `Tracker` (atomic-counter, sync.Once-gated overflow callback) + `LimitedWriter` (io.Writer wrapper). Multiple LimitedWriters may share one Tracker so MaxOutputSize accounts for stdout+stderr combined. Partial-write semantics: the call that crosses the budget still flushes up to the remaining budget, then returns the overflow error. Nil-tracker and zero-limit are passthrough.
- **`internal/ringbuf/ringbuf_test.go`** — under-limit happy path, exact-boundary trip, partial-write-on-overflow, sticky-error (single onOver invocation), shared-tracker semantics across two LimitedWriters, zero/nil-tracker passthrough.
- **`bash.go` — limit wiring inside `Exec`:**
  - `instrumentLoops(file)` walks the post-parse AST with `syntax.Walk` and prepends a `__gobash_loop_iter__` sentinel Stmt to every `*syntax.WhileClause` and `*syntax.ForClause` Do body.
  - A per-Exec `CallHandler`:
    1. If `args[0] == loopSentinelName`, bumps `loopIters`, checks `MaxLoopIterations`, rewrites args to `[":"]` (no-op builtin). Sentinel calls do NOT count toward `MaxCommandCount`.
    2. Else, bumps `cmdCount`, checks `MaxCommandCount`.
    3. If `runner.Funcs[args[0]] != nil` (the call resolves to a declared function), calls `countMvdanCallFrames()` and checks `MaxCallDepth`.
  - `MaxOutputSize` enforcement: a single `ringbuf.Tracker` shared by stdout/stderr `LimitedWriter`s. Because mvdan/sh's `r.out` discards `io.Writer` errors, the Tracker's `onOver` calls `cancel()` on an internal `execCtx` and stashes a `*ExecutionLimitError` in `limitErr`. After `runner.Run` returns, `limitErr` is surfaced before any context-sentinel translation — including when `runErr == nil` (the script finished cleanly even though the writer was truncated).
  - Handler-returned `*ExecutionLimitError`s also flow through unchanged via `errors.As` on `runErr` (defensive: today the handler path always returns the same instance we stashed in `limitErr`, but the extraction is kept so a future fatal-wrap by mvdan/sh would still surface our typed error).
- **`limits_enforce_test.go`** — TDD-first tests covering SPEC §2.4:
  - `TestMaxLoopIterations` — `while true; do :; done` with `MaxLoopIterations:50` ⇒ `*ExecutionLimitError{Limit:"MaxLoopIterations", Value:50}` (and explicitly not `context.Canceled`).
  - `TestMaxLoopIterationsForLoop` — `for i in 1 2 3 4 5; do :; done` with `MaxLoopIterations:3` ⇒ same error.
  - `TestMaxOutputSize` — `printf '%200s' x` with `MaxOutputSize:64` ⇒ `*ExecutionLimitError{Limit:"MaxOutputSize", Value:64}`.
  - `TestMaxOutputSizeCountsStderr` — `printf '%100s' x >&2` ⇒ stderr is also wrapped; same error type.
  - `TestMaxCallDepth` — `f() { f; }; f` with `MaxCallDepth:10` ⇒ `*ExecutionLimitError{Limit:"MaxCallDepth", Value:10}`.
  - `TestMaxCommandCount` — guard for §2.3 wiring.
  - `TestLimitsDoNotImpactNormalScripts` and `TestLoopSentinelInvisible` — regression guards confirming the sentinel doesn't leak into stdout and that under-limit scripts run unaffected.
- **`bash_test.go`** — `TestExecMidScriptCancellation` updated to set `MaxLoopIterations`/`MaxCommandCount` to `1<<30` so the ctx-cancel-during-busy-loop test isn't pre-empted by the now-enforced loop budget. All other Phase-1 acceptance tests pass unchanged.

## Acceptance criteria (from SPEC.md §2.4)

- [x] **`while true; do :; done` aborts with `ExecutionLimitError{Limit:"MaxLoopIterations"}`** — `TestMaxLoopIterations` passes; also asserts the error is NOT `context.Canceled` / `context.DeadlineExceeded`.
- [x] **Oversized stdout aborts with `ExecutionLimitError{Limit:"MaxOutputSize"}`** — `TestMaxOutputSize` passes; the writer truncation is observable in `result.Stdout`.
- [x] **Deep recursive shell function aborts on `MaxCallDepth`** — `TestMaxCallDepth` passes; an `f() { f; }; f` script with limit=10 fires before the goroutine stack expands meaningfully.
- [x] **Defaults exactly match SPEC §2.1 table** — `TestDefaultResolvedLimits` from Phase 1 is unchanged and still green.
- [x] **`make ci` passes** — tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck, 79.3% combined coverage.

## Tests

- Package `github.com/mark3labs/go-bash` — 14 tests, all pass, 77.1% coverage.
- Package `github.com/mark3labs/go-bash/internal/ringbuf` — 7 tests, all pass, 90.0% coverage.
- No comparison fixtures yet (harness lands in Phase 19).

## Decisions & gotchas discovered

- **Loop-iteration counting via AST instrumentation.** mvdan/sh exposes `interp.CallHandler` (per simple-command entry) but no per-loop-iteration callback. The cleanest workaround was to walk the AST post-parse and prepend a sentinel CallExpr (`__gobash_loop_iter__`) to every `WhileClause.Do` / `ForClause.Do`. The CallHandler intercepts the sentinel, bumps a counter, and rewrites args to `[":"]` so the no-op builtin runs. This counts only *successful* loop iterations (sentinel sits in `Do`, after the cond/header) and survives `continue` (sentinel runs at top of each Do execution, exactly like a real iteration marker). `break` short-circuits before the next sentinel fires — also correct. Sentinel calls are explicitly excluded from `MaxCommandCount` accounting.
  - **Alternative considered:** wrapping the runner's exec/call dispatch via reflection or forking mvdan/sh. Rejected — AST instrumentation is non-invasive and survives mvdan/sh version bumps.
- **Call-depth counting via runtime stack walk.** mvdan/sh has no function entry/exit pair — only `CallHandler` on entry. Without a decrement signal, a naive counter would overcount (consecutive non-nested calls would accumulate forever). Instead, `countMvdanCallFrames` uses `runtime.Callers` + `runtime.CallersFrames` to count nested `interp.(*Runner).call` frames at each handler invocation. The frame name match is done via `strings.HasSuffix(name, "interp.(*Runner).call")` so it survives the full qualified-path form (`mvdan.cc/sh/v3/interp.(*Runner).call`).
  - **Cost:** stack walk is O(stack depth). It only runs when `args[0]` is a known shell function, so straight-line scripts pay nothing.
  - **Caveat:** depends on the mvdan/sh internal method `call` keeping its name. If a future mvdan/sh release renames or splits that method we'd see depth=0 and `MaxCallDepth` would silently stop firing. **Mitigation:** `TestMaxCallDepth` is a tight canary — any version bump that breaks this would fail CI immediately.
  - **Alternative considered:** context-value-based depth tracking — rejected because mvdan/sh calls `r.stmt(ctx, body)` with the *original* ctx, not anything the handler returned, so we can't inject a depth marker into nested calls.
- **`MaxOutputSize` via cancel-and-stash.** mvdan/sh's `(*Runner).out` does `io.WriteString` and discards the returned error. So a `LimitedWriter.Write` returning our overflow error has no effect on the runner's flow. The fix: when the Tracker overflows, the `onOver` callback calls `cancelExec()` on an internal `context.WithCancel`-wrapped ctx and stashes the typed error. After `runner.Run` returns, `Exec` surfaces the stash *before* translating `runErr` — including when `runErr == nil` (the script finished cleanly even though the writer was truncated, e.g. a single `printf` that overshoots). Tests cover both the "truncate-then-finish-naturally" and "truncate-mid-script" cases via the printf paths.
- **Stdout + stderr share one MaxOutputSize budget.** SPEC §2.3 says "wraps writers in a `ringbuf.LimitedWriter` enforcing `MaxOutputSize`" — singular. Both writers wrap the same `Tracker`, so a script that writes 5 MiB to each stream trips the 10-MiB default cap. `TestMaxOutputSizeCountsStderr` covers stderr-only overflow.
- **Phase-1 cancellation test required a limits bump.** `TestExecMidScriptCancellation` runs `while true; do :; done` and cancels after 50ms; with default `MaxLoopIterations`/`MaxCommandCount` of 10000, the loop trips the limit in well under 50ms. Set both to `1<<30` in that test so ctx cancel remains the only failure mode. SPEC §2.1 defaults (and `TestDefaultResolvedLimits`) are untouched.
- **`MaxSourceDepth` not yet enforced.** SPEC §2.3 mentions it but §2.4 acceptance doesn't require it; `source` / `.` builtin coverage and `MaxSourceDepth` enforcement land alongside Phase 11 (interpreter built-ins) when we have the source/. dispatch under our own control. Recorded as a Phase-11 follow-up below.
- **Expansion-side limits deferred to Phase 4** (`MaxStringLength`, `MaxSubstitutionDepth`, `MaxBraceExpansionResults`, `MaxHeredocSize`, `MaxArrayElements`, `MaxGlobOperations`) — SPEC §2.3 already calls this out ("Limits on string growth … are enforced at the expansion layer — see Phase 4").
- **Timeout limits deferred** (`MaxPythonTimeout`, `MaxJsTimeout`, `MaxSqliteTimeout`) — land in Phases 14/15/16 alongside their runtimes.
- **`MaxFileDescriptors`, `MaxAwkIterations`, `MaxSedIterations`, `MaxJqIterations`** — land with their owning builtins (Phase 10 waves D and following).
- **Handler error path is double-belt-and-braces.** Even though CallHandler-returned `*ExecutionLimitError`s also call `trip` (and therefore `cancelExec`), the post-Run path tries `errors.As(runErr, &ele)` *and* falls back to `limitErr`. Today both paths return the same instance for handler-driven trips, but the redundancy means a future mvdan/sh that wraps the handler error opaquely would still surface the typed error.

## Open follow-ups (non-blocking)

- Enforce `MaxSourceDepth` in Phase 11 when the `source` / `.` builtin lands under our dispatch.
- Add expansion-side limits in Phase 4 (`MaxStringLength` etc.).
- Add `MaxFileDescriptors` enforcement when the FD-tracking layer lands (Phase 3+).
- Tighten the `interp.(*Runner).call` frame matcher if mvdan/sh ever exposes a public call-lifecycle hook — replace `runtime.Callers` with the upstream API.
- Consider whether `MaxOutputSize` should be split into per-stream caps once we have richer output semantics (today: shared budget, which matches the conservative just-bash default; revisit if comparison fixtures require divergence).
- Carry forward all Phase-1 open follow-ups not addressed here (Args wiring, `time/tzdata`, etc.).

## NEXT PHASE: Phase 3 — Virtual Filesystem

- **Goal:** introduce the `fs` package and the four FileSystem implementations (`InMemoryFs`, `OverlayFs`, `ReadWriteFs`, `MountableFs`) per SPEC §3. Stand up the path utilities and the FS contract test suite. Wire `Bash.FS()` and `BashOptions.{Files, FS}`; replace mvdan/sh's default open/stat/readdir handlers so script-side file ops hit the VFS instead of the host disk.
- **Read:** SPEC.md §3 in full (§3.1 interface, §3.2 path utils, §3.3 memfs, §3.4 realfs helpers, §3.5 overlayfs, §3.6 rwfs, §3.7 mountfs, §3.8 contract tests), plus the resolved-decisions block at the end of SPEC.md (TZ handling and globber overrides hint at where realfs/memfs need to plug in later).
- **Deliver:**
  - `fs/fs.go` — `FileSystem` interface (full method set from SPEC §3.1).
  - `fs/path.go` — clean/join/validate helpers; **null-byte rejection** is mandatory.
  - `fs/memfs/` — in-memory implementation with directories, regular files, symlinks, permissions, mtime.
  - `fs/realfs/` — shared TOCTOU-safe helpers (O_NOFOLLOW, fstatat) used by overlay+rwfs.
  - `fs/overlayfs/`, `fs/rwfs/`, `fs/mountfs/` — the remaining three modes.
  - `internal/testutil` (or `fs/fstest`) — FS contract test suite the four implementations all pass.
  - `bash.go` — add `fs` field, `Bash.FS()` accessor, `BashOptions.Files`, `BashOptions.FS`; replace mvdan/sh's default `OpenHandler`, `StatHandler`, `ReadDirHandler2` with VFS-backed ones; honor `BashOptions.Files` at construction time.
- **Acceptance (SPEC §3.8):** the contract test suite runs against all four FS implementations and passes for the common subset; per-impl divergences (e.g. RWFs's host-disk writes) are gated by capability flags. `make ci` green. `BashOptions.Files` round-trips through `Bash.FS().Read(...)`.
- **Kickoff prompt for the next session:**

  > Implement Phase 3 of go-bash per SPEC.md §3. Read AGENTS.md and HANDOFF.md first, then `handoffs/phase-2.md` for the loop-instrumentation pattern, the `runtime.Callers` call-depth workaround, and the MaxOutputSize cancel-and-stash trick — these stay in force when the VFS lands. Phase 3 introduces the virtual filesystem: stand up `fs/fs.go` (FileSystem interface), `fs/path.go` (path utils with mandatory null-byte rejection), `fs/memfs/` (InMemoryFs), `fs/realfs/` (TOCTOU-safe helpers shared by overlay+rwfs), `fs/overlayfs/`, `fs/rwfs/`, and `fs/mountfs/`. Add the FS contract test suite (SPEC §3.8) and make all four implementations pass the common subset. Wire `Bash.fs` field, `Bash.FS()` accessor, `BashOptions.Files` and `BashOptions.FS`, and override mvdan/sh's default `OpenHandler`/`StatHandler`/`ReadDirHandler2` so script-side file operations stop hitting the host disk. Honor `BashOptions.Files` at `New` time. Write the contract tests first per the TDD rule, then implement; the four limit enforcement tests from Phase 2 must still pass unchanged. Run `make ci` until green, then call `finalize_phase` with `phase=3`, the drafted handoff markdown, the new root `HANDOFF.md` pointer, a conventional-commit subject like `feat(phase-3): virtual filesystem layer`, and the kickoff prompt for Phase 4 (parser / AST public surface).
