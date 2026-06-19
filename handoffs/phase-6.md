# go-bash Handoff — Phase 6 complete

- **Date:** 2026-06-17
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). Combined coverage 58.7% (up from 56.7%). Root `gobash` package coverage 80.5% (up from 75.3%); new `caps.go` 80.5%; `interp` package 52.9% (~same, the new ReadDirHook branch is exercised from gobash-package tests).

## What this phase delivered

- **`caps.go`** (NEW, root package) — `enforceExpansionCaps(file, limits)` runs before `runner.Run` inside `Exec`. Three static-AST analyzers:
  - `checkBraceCap` walks every `*syntax.Word`, copies it, calls `syntax.SplitBraces` on the *copy* (the AST stays untouched so subsequent walks don't blow up on the unsupported `*syntax.BraceExp` node), and computes brace cardinality with `cap+1` saturation to short-circuit absurdly wide sequences like `{1..2000000000}`. Returns `*ExecutionLimitError{Limit: "MaxBraceExpansionResults"}` when the count exceeds the cap. SPEC §6.4.
  - `checkSubstDepth` walks `*syntax.CmdSubst` + `*syntax.ProcSubst` chains and bounds nesting depth via the recursive `walk(node, depth+1)` pattern. Returns `*ExecutionLimitError{Limit: "MaxSubstitutionDepth"}`. SPEC §2.1.
  - `checkArrayElems` walks `*syntax.ArrayExpr`, summing each element word's brace cardinality. Returns `*ExecutionLimitError{Limit: "MaxArrayElements"}`. SPEC §2.1.
  - Helpers: `wordBraceCardinality` (the non-mutating copy-then-split wrapper), `mayHaveBraces`, `braceCardinality`, `sequenceCardinality`, `wordToInt`, `wordToChar`, `wordLit`. All package-private.
- **`bash.go`** — three runtime caps wired into `Exec`:
  - **MaxStringLength** (SPEC §6 / §2.1): per-arg length check inside the existing `callHandler` closure. Fires before `cmdCount` bumps; reports `*ExecutionLimitError{Limit: "MaxStringLength"}`.
  - **MaxGlobOperations** (SPEC §6.6 / §2.1): a `readDirHook` closure passed into the new `interp.Config.ReadDirHook` field. Increments on every VFS ReadDir; reports `*ExecutionLimitError{Limit: "MaxGlobOperations"}` when exceeded.
  - **Atomic counters**: `cmdCount`, `loopIters`, `globOps` were re-typed to `atomic.Int64`. Process substitution (`<(cmd)`) runs the substituted command in an mvdan/sh-spawned goroutine that shares our CallHandler closure; without atomics the `-race` detector trips. The MaxCallDepth stack-walk path is unchanged (it's read-only).
  - `enforceExpansionCaps(file, b.limits)` is invoked right after `instrumentLoops`, so AST-side caps fire before any runtime allocation.
- **`interp/runner.go`** — `Config.ReadDirHook func(ctx context.Context, path string) error` field added. When non-nil, `readDirHandler` calls it before delegating to `cfg.FS.ReadDir`; a non-nil return aborts the readdir with that error. Documented in the field comment as the Phase 6 wire-up point for `MaxGlobOperations`.
- **`expansion_test.go`** (NEW, root package) — 27 §6.1–§6.12 acceptance tests through `Bash.Exec`. Per-subsection:
  - **§6.1 Variables / parameter expansion** — `TestPhase6ParamExpansionBasic` (12 subcases: `$VAR`, `${VAR}`, `${VAR:-default}`, `${VAR:=assign}`, `${VAR:+alt}`, substring `${VAR:offset:length}`, `${#VAR}`, `${VAR/pat/repl}`, `${VAR//pat/repl}`, `${VAR#pat}`/`${VAR##pat}`, `${VAR%pat}`/`${VAR%%pat}`, case `^^`/`^`/`,,`/`,`). `TestPhase6ParamExpansionErrorOp` (`${VAR:?error}`). `TestPhase6ParamExpansionArrays` (5 subcases: `${arr[i]}`, `${arr[@]}`, `${arr[*]}`, `${#arr[@]}`, `${!arr[@]}`).
  - **§6.2 Positional parameters** — `TestPhase6Positional` (6 subcases: `set --`, `$#`, `$@`, `shift`, `shift 2`, `$*` with IFS).
  - **§6.4 Brace expansion** — `TestPhase6BraceExpansion` (7 subcases: comma, sequence, sequence+step, char range, reversed range, prefix/suffix, nested).
  - **§6.5/§6.6 Globs** — `TestPhase6GlobBasic` (3 subcases: `*.txt`, `[ab].txt`, `[!c]*.txt`).
  - **§6.7 Arithmetic** — `TestPhase6Arithmetic` (22 subcases covering all listed operators) + `TestPhase6ArithmeticDivByZero`.
  - **§6.8 Conditionals** — `TestPhase6TestSingleBracket` (5 subcases), `TestPhase6TestDoubleBracket` (7 subcases incl. `=~` regex + `BASH_REMATCH`), `TestPhase6TestFileFlags` (6 subcases through the VFS).
  - **§6.9 Redirections** — `TestPhase6Redirections` covers `>`, `>>`, `<`, `<<EOF`, `<<<`, `2>&1`, `&>`. `TestPhase6HeredocStripsTabs` covers `<<-EOF`. `TestPhase6ProcessSubstitution` covers `< <(cmd)`.
  - **§6.10 Control flow** — `TestPhase6ControlFlow` (10 subcases: if/elif/else, for-in, C-style for, while, until, case, break, break N, continue, `&&`/`||`). `TestPhase6CommandSubstitution` (4 subcases: `$(...)`, nested, backticks, assignment).
  - **§6.12 Functions** — `TestPhase6Functions` (5 subcases: `()` form, `function` keyword form, `local`, `return N`, recursive arithmetic).
- **Runtime-cap tests** — `TestPhase6BraceExpansionCapTrips` (default cap, `{1..20000}`), `TestPhase6BraceExpansionCapNestedTrips` (product), `TestPhase6BraceExpansionAbsurdSequenceSaturates` (`{1..2000000000}` saturates without OOM), `TestPhase6GlobMaxGlobOperations` (override to 3), `TestPhase6SubstitutionDepthCapTrips` (override to 3) + `TestPhase6SubstitutionDepthShallowPasses`, `TestPhase6MaxStringLengthCap`, `TestPhase6MaxArrayElementsCap`, `TestPhase6CapsDoNotImpactNormalScripts`.
- **Divergence tests** (pin current mvdan/sh behavior) — `TestPhase6ParamExpansionAnchorDivergence` (`${X/#abc/XX}` is no-op).
- **`DECISIONS.md`** (NEW) — formal record of every Phase 6 swap and every observed mvdan/sh divergence from real bash. Five upstream divergences listed (A–E) with fix plans owned by Phases 10–11.

## Acceptance criteria (from SPEC.md §6)

- [x] **§6.1 parameter expansion ops covered** — `TestPhase6ParamExpansionBasic` (12 subcases) + `TestPhase6ParamExpansionErrorOp` + `TestPhase6ParamExpansionArrays` (5 subcases).
- [x] **§6.2 positional parameters covered** — `TestPhase6Positional` (6 subcases including IFS join).
- [x] **§6.4 brace expansion shapes covered** — `TestPhase6BraceExpansion` (7 subcases).
- [x] **§6.5/§6.6 globs covered** — `TestPhase6GlobBasic` (3 subcases through VFS).
- [x] **§6.7 arithmetic operators covered** — `TestPhase6Arithmetic` (22 subcases).
- [x] **§6.8 `[[ ]]` and `[ ]` covered** — three test functions (single bracket, double bracket including `=~` + BASH_REMATCH, file flags).
- [x] **§6.9 redirections covered** — `>`, `>>`, `<`, `<<EOF`, `<<-EOF`, `<<<`, `2>&1`, `&>`, process substitution `< <(...)`.
- [x] **§6.10 control flow + command substitution covered** — if/elif/else, for, C-style for, while, until, case, break/break N/continue, `&&`/`||`, `$(...)`/backticks/nested.
- [x] **§6.12 functions covered** — both define forms, `local`, `return N`, recursion.
- [x] **MaxBraceExpansionResults trips on pathological input** — three tests (single sequence, nested product, absurd saturation).
- [x] **MaxSubstitutionDepth trips on deep nesting** — `TestPhase6SubstitutionDepthCapTrips` with override to 3.
- [x] **MaxArrayElements trips on oversized literal** — `TestPhase6MaxArrayElementsCap`.
- [x] **MaxStringLength trips on oversized arg** — `TestPhase6MaxStringLengthCap`.
- [x] **MaxGlobOperations trips on heavy glob** — `TestPhase6GlobMaxGlobOperations`.
- [x] **Normal scripts unaffected** — `TestPhase6CapsDoNotImpactNormalScripts`.
- [x] **Phases 1–5 stay green** — `make ci` PASS; all Phase 1–5 tests still pass.
- [x] **Divergences recorded with fix plans** — `DECISIONS.md` items A–E (anchor patterns, IFS at assignment, `let` spacing, `$((1/0))` exit code, `$FUNCNAME`).

## Tests

- `github.com/mark3labs/go-bash` — **60 tests** (33 from Phases 1–5 + 27 Phase 6). Coverage 80.5% (up from 75.3%). New `caps.go` 80.5%.
- `github.com/mark3labs/go-bash/interp` — 6 tests, 52.9% (the new ReadDirHook field is covered indirectly via the gobash-package glob test).
- All other packages unchanged from Phase 5.
- Combined coverage: **58.7%** (up from 56.7%).
- Comparison fixtures: N/A this phase. Harness lands in Phase 19.

## Decisions & gotchas discovered

- **SplitBraces is destructive AND mvdan/sh's syntax.Walk panics on the resulting `*syntax.BraceExp` node.** Found via the very first test run — `splitAllBraces(file)` mutated the AST in place, then `checkSubstDepth`'s `syntax.Walk` died with "unexpected node type *syntax.BraceExp". Fixed by switching to `wordBraceCardinality(w)` which copies the Word before splitting. This matches mvdan/sh's own runtime path (`expand/expand.go::FieldsSeq` line 466 makes a `word := *word` copy before calling SplitBraces).
- **Process substitution races the CallHandler counters.** mvdan/sh runs the substituted command in a goroutine (see `interp/runner.go::fillExpandConfig.func2`). The shared `cmdCount`, `loopIters`, `globOps` integer counters tripped `-race`. Migrated to `atomic.Int64`. This will continue to apply to anything inside `&` background jobs once SPEC §6.11 lands; the atomic guards are forward-compatible.
- **MaxSubstitutionDepth vs MaxParserDepth.** The first version of `TestPhase6SubstitutionDepthCapTrips` chained 60 `$(...)` levels expecting the substitution cap to fire (default 50). It tripped the *parser* cap instead — `MaxParserDepth=200` is exceeded by ~50 substitutions because each `$(...)` adds several stack frames inside the syntax parser. Fixed by overriding `MaxSubstitutionDepth` to 3 in the test and using a 6-deep chain that stays well under MaxParserDepth.
- **Five mvdan/sh divergences from real bash are documented but unpatched.** Cataloged in `DECISIONS.md` (items A–E) with the regression test that pins each. Patch plans assigned to Phases 10–11 (the `let` and `$FUNCNAME` ones in particular pair with the builtin/interpreter-builtin work).
- **The MaxArrayElements check counts brace-expansion expansion product, not actual runtime cardinality.** `arr=({1..1000} a b)` is rejected because `{1..1000}` alone contributes 1000 elements; the array literal counts 1002 toward MaxArrayElements (default 100000). This is conservative — matches the just-bash intent of bounding allocation before it happens.
- **The MaxGlobOperations counter over-counts.** Every ReadDir contributes one tick, including non-glob ReadDirs (e.g. `cd`'s directory probe, completion). At the default 100k cap nothing real will hit it, but pathological scripts that do tens of thousands of `cd`/`pwd` operations would. Documented in DECISIONS.md item #4; not currently fixable without a per-call-site flag from mvdan/sh.
- **MaxStringLength is a CallHandler-side check, not an expansion-side one.** A script that builds a 100MB string into a variable but never passes it to a command escapes the cap. Closing this requires a fork of `expand.Config` — out of scope for Phase 6. The related DoS via stdout/stderr is bounded by MaxOutputSize (Phase 2, still load-bearing).
- **`echo {a..z..2}` is missing from the test matrix.** mvdan/sh doesn't accept the char-range-with-step form (the third Elem is treated as broken), so we don't assert it. The corresponding numeric form `{1..10..2}` does work and IS tested.
- **`select` (§6.10) is not covered.** SPEC §6.10 lists `select x in ...; do ...; done` but doesn't include it in the acceptance bullets — mvdan/sh supports it but exercising it requires synchronous prompt/read handling. Deferred to Phase 19's comparison fixtures, where the interactive shape is naturally covered.

## Open follow-ups (non-blocking)

- **Phase 8 closes the os/exec gap.** The `commandExecHandler` stub still passes through to mvdan/sh's `DefaultExecHandler` for unknown commands, so any §6 test that mentions a host binary (e.g. `cat`) would leak through. Phase 6 tests avoid this by using only mvdan/sh builtins + function definitions; the gap stays open until Phase 8.
- **DECISIONS.md items A–E are the Phase 10/11 patch backlog.** Anchor-pattern rewrite, IFS re-read at assignment, `let` whitespace normalization, `$((expr))` error→exit translation, and `$FUNCNAME` population. Each entry names the owning phase.
- **`expand.Config` shim.** A fuller fix for MaxStringLength (assignment side) and the IFS divergence requires shadowing the runner's expand pipeline. Cleanest path is a thin `interp.Config.ExpandHook` field landing in Phase 13 alongside the transform plugin work.
- **`caps.go` `errString` type / `Error` method is uncovered.** The error value is constructed once (`errNoLit`) but only checked via `err == nil` semantics in `wordToInt`. The Error() method is dead code today; staticcheck doesn't flag it because the type is package-private but still referenced. Leave it for now; revisit during Phase 21 polish.
- **The `select` and `coproc` shell forms are not exercised.** Both are in SPEC §6.10 / §6.x but skipped this phase (interactive). Hooks land naturally with the Phase 19 comparison harness.
- **MaxBraceExpansionResults treats char ranges optimistically.** A range like `{A..z}` (which in real bash includes the punctuation in between) is computed as `int(z)-int(A)+1 = 58` — correct for cardinality, but the *content* could surprise scripts that round-trip the names. Acceptance is on count, not content; document this if a comparison-fixture lights it up.

## NEXT PHASE: Phase 7 — Filesystem Init & Default Layout

- **Goal:** port `src/fs/init.ts`. When `New(BashOptions{})` is called with no `Cwd` and no `Files`, create the default layout: `/`, `/home`, `/home/user`, `/bin`, `/usr`, `/usr/bin`, `/tmp`, `/etc`, `/dev`, `/proc`, `/proc/self`, plus `/etc/hostname=localhost\n` and a templated `/proc/self/status` (uses ProcessInfo). For every registered command `X`, write a no-op stub at `/bin/X` (mode 0755). Set `$HOME=/home/user`, `$PATH=/usr/bin:/bin`.
- **Read:** SPEC.md §7, the Phase 5 handoff for the FS-init plumbing in `New()`, the Phase 6 handoff (this file) for the runtime-cap shape (the new layout needs to fit under MaxBraceExpansionResults / MaxArrayElements / MaxGlobOperations cleanly), and `bash.go::New` for the existing default-cwd behavior (`/home/user` when no Files).
- **Spec to read:** SPEC.md §7 (just one section). The `/bin/X` stubs depend on the command registry, which lands in Phase 8 — until then the stub list is empty.
- **Packages/files to create:** likely a new `internal/fsinit` package or `fs_init.go` at the root. Tests as `fs_init_test.go`.
- **Public symbols to deliver:** none new at the public surface — Phase 7 is internal plumbing called from `New(BashOptions{})`. The behavior change is observable via `bash.FS()` after construction.
- **Prerequisites:** met. The FS layer (Phase 3) plus the existing `New()` cwd-init logic in `bash.go` is the full backing surface.
- **Kickoff prompt for the next session:**

  > Implement Phase 7 of go-bash per SPEC.md §7. Read AGENTS.md and HANDOFF.md first, then `handoffs/phase-6.md` for the runtime-cap envelope around the FS init path (the default layout must fit under MaxBraceExpansionResults / MaxArrayElements / MaxGlobOperations) and the Phase 5 handoff for the existing `New()` cwd-init logic. Goal: when `New(BashOptions{})` is called with no `Cwd` and no `Files`, populate the in-memory FS with the SPEC §7 default layout — `/`, `/home`, `/home/user`, `/bin`, `/usr`, `/usr/bin`, `/tmp`, `/etc`, `/dev`, `/proc`, `/proc/self`, plus `/etc/hostname="localhost\n"` and a templated `/proc/self/status` using `Bash.procInfo`. Set `$HOME=/home/user` and `$PATH=/usr/bin:/bin` in the initial env (without overriding user-supplied values). The `/bin/X` per-command stub list is empty until Phase 8 lands the registry — design the helper so Phase 8 can extend it without restructuring. When `Cwd` is set or `Files` is provided, skip the default layout but still create the cwd (current behavior is fine). Write tests asserting (a) the layout is present after `New(BashOptions{})`, (b) the layout is NOT created when the caller provides Files or Cwd, (c) `/proc/self/status` interpolates the ProcessInfo fields, (d) user-supplied `Env["HOME"]` overrides the default. Run `make ci` until green, then call `finalize_phase` with `phase=7`, the drafted handoff markdown, the new root `HANDOFF.md` pointer, a conventional-commit subject like `feat(phase-7): default filesystem layout`, and the kickoff prompt for Phase 8 (command registry).
