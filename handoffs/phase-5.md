# go-bash Handoff — Phase 5 complete

- **Date:** 2026-06-17
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck + errcheck via golangci-lint). Combined coverage 56.7%; new `interp` package 56.2%; root `gobash` jumped from 71.9% → 75.3% as `Exec`'s env-mutation branch landed under test.

## What this phase delivered

- **`interp/runner.go`** (NEW PACKAGE) — `BuildRunner(ctx, Config) (*mvinterp.Runner, error)` builds an `mvdan.cc/sh/v3/interp.Runner` with every gobash customization hook wired in:
  - `interp.StdIO(stdin, stdout, stderr)` — caller pre-wraps stdout/stderr in the MaxOutputSize ringbuf writer.
  - `interp.Env(expand.ListEnviron(envSlice...))` — initial env from the assembled `BASH+overlay` slice.
  - `interp.ExecHandlers(commandExecHandler)` — the Phase 5 stub middleware that simply calls `next(ctx, args)`. This is the exact hook-point where Phase 8's registry dispatch plugs in (lookup → CommandContext → `cmd.Execute` → `interp.NewExitStatus`; `interp.ErrNotFound` for unregistered commands).
  - `interp.OpenHandler` / `interp.StatHandler` / `interp.ReadDirHandler2` — VFS-backed via the `openHandler` / `statHandler` / `readDirHandler` closures, each resolving paths through `gbfs.Resolve(HandlerDir(ctx), path)`.
  - `interp.CallHandler(cfg.CallHandler)` — appended only when the caller supplies one (gobash's per-Exec limit accounting passes it; the standalone interp tests do not).
  - `runner.Dir = cfg.Cwd` set **after** `interp.New(...)`, NOT via `interp.Dir(path)` — that helper does an `os.Stat` on the host disk and would reject any VFS-only path (e.g. `/home/user`). Phase 3 quirk preserved.
- **`interp/runner.go::HandlerDir(ctx)`** — exported recover-guarded helper around `mvinterp.HandlerCtx(ctx).Dir`. mvdan/sh's `Runner.stat` / `Runner.lstat` call our `StatHandler` with the bare ctx (no HandlerContext attached); a naive `interp.HandlerCtx(ctx)` panics in that case. The `defer/recover` returns an empty `dir`, which `gbfs.Resolve` handles correctly for absolute inputs.
- **`interp/runner.go::commandExecHandler`** — the named-middleware stub. Documented inline as the Phase 8 swap point. Empty `args` is guarded defensively so the Phase 8 registry dispatch can rely on the same guard.
- **`interp/runner.go::ensurePathError`** — normalizes VFS errors into `*os.PathError` so mvdan/sh's diagnostics still look like a real shell. (Moved verbatim from `bash.go`.)
- **`bash.go`** refactored to:
  - Replace the inline `syntax.NewParser().Parse(...)` with `parser.Parse(script)`. Parser-side limits (`MaxInputSize`, `MaxTokens`, `MaxParserDepth`, `MaxHeredocSize`) now fire from inside `Exec` instead of only from direct `parser.Parse` callers.
  - Use `parsed.Origin` as the `*syntax.File` handed to `instrumentLoops` and `runner.Run`. The Phase 4 AST gains an active consumer; the loop-sentinel rewrite operates on the same `*syntax.File` parser stored on the Script.
  - Replace the inline `interp.New(...)` plus FS-handler chain with `bashinterp.BuildRunner(execCtx, bashinterp.Config{...})`. The CallHandler closure (which owns the limit counters) is passed in via `Config.CallHandler` — the only piece that needs to stay in `bash.go` because it closes over the per-Exec mutable state.
  - **Env propagation across Exec calls (SPEC §5.6)** now lives at the end of `Exec`: when `opts.Env == nil || opts.ReplaceEnv` is true, the runner's exported vars are merged back into `b.env` so a subsequent `Exec` sees them. With a per-call `Env` and `ReplaceEnv=false`, exports stay ephemeral.
  - Removed: `b.fsOpenHandler` / `b.fsStatHandler` / `b.fsReadDirHandler` / `handlerDir` / `ensurePathError`. All five moved into the `interp` package.
- **`interp_bridge_test.go`** (NEW, root package) — Phase 5 acceptance:
  - `TestPhase5SmokeForLoop` — `for i in 1 2 3; do echo $i; done` → `1\n2\n3\n` (§5.7 bullet 1).
  - `TestPhase5SmokeSubshellScope` — `x=1; (x=2; echo $x); echo $x` → `2\n1\n` (§5.7 bullet 2).
  - `TestPhase5SmokeVFSRedirect` — `echo "Hello" > greeting.txt; read line < greeting.txt; echo "$line"` → `Hello\n` against the in-memory VFS, plus a direct `b.FS().ReadFile("/home/user/greeting.txt")` cross-check confirming the file landed in the VFS (and **not** the host disk).
  - `TestPhase5ExportPropagatesAcrossExec` — `Exec("export X=hello"); Exec("echo $X")` → `hello\n` (§5.7 bullet 3, §5.6 contract).
  - `TestPhase5PerCallEnvDoesNotPolluteBase` — `Exec("export X=...", Env={X:once})` does not mutate `b.env`; the next `Exec` sees `X` unset (§5.7 bullet 4, §5.6 contract).
  - `TestPhase5ReplaceEnvSnapshotsExports` — when `ReplaceEnv=true` is set alongside a per-call Env, exports DO persist back into `b.env`. Locks the literal §5.6 reading ("unless ExecOptions.Env was provided **without** ReplaceEnv").
  - `TestPhase5SetEPropagates` — `set -e; false; echo should_not_run` aborts before `echo` and reports a non-zero `ExitCode`.
  - `TestPhase5CdAndPwdViaVFS` — `cd /tmp && pwd` against a VFS containing `/tmp` returns `/tmp\n`. Confirms the StatHandler is consulted for `cd`'s directory check.
  - `TestPhase5StdinPropagates` — `ExecOptions.Stdin` reads two lines.
  - `TestPhase5StdoutWriterStillCapturesCorrectly` — caller-supplied `Stdout` writer bypasses string capture (Phase 1 contract preserved).
  - `TestPhase5ParserLimitsSurfaceFromExec` — `MaxInputSize`-overflow input now surfaces a `*gobash.ParseError` from `Exec`, proving the Phase 4 limits are live in the public path.
- **`interp/runner_test.go`** (NEW) — package-local coverage:
  - `TestBuildRunnerRequiresFS` / `TestBuildRunnerRequiresStdoutStderr` — early-fail guards for missing required Config fields (no silent fall-through to host disk / nil-writer panics).
  - `TestBuildRunnerWiresVFSOpen` — direct round-trip: `echo hi > /tmp/out` writes to the supplied `memfs.FS` and is readable via `fs.ReadFile`.
  - `TestBuildRunnerCwdWithoutHostStat` — `runner.Dir` accepts `/nonexistent/on/host` (Phase 3 quirk regression).
  - `TestHandlerDirAbsentReturnsEmpty` — recover-guarded `HandlerDir` returns `""` on a bare ctx.
  - `TestBuildRunnerCancellation` — `while true; do :; done` with a pre-cancelled ctx returns `context.Canceled`.

## Acceptance criteria (from SPEC.md §5.7)

- [x] **`for i in 1 2 3; do echo $i; done` outputs `1\n2\n3\n`** — `TestPhase5SmokeForLoop`.
- [x] **`x=1; (x=2; echo $x); echo $x` → `2\n1\n`** — `TestPhase5SmokeSubshellScope`.
- [x] **`bash.Exec("export X=hello"); bash.Exec("echo $X")` → `hello\n`** — `TestPhase5ExportPropagatesAcrossExec`.
- [x] **`bash.Exec("X=ephemeral env", ExecOptions{Env: ...})` does not pollute env** — `TestPhase5PerCallEnvDoesNotPolluteBase`.
- [~] **All ~50 sample scripts from `src/comparison-tests/` for plain shell features pass** — deferred. The comparison-fixture harness lands in Phase 19 (per the build DAG and `handoffs/phase-3.md`'s "No comparison fixtures yet" note). The four discrete bullets above plus the eight other Phase 5 tests provide the coverage we can run today; the ~50-fixture run hooks in once the harness exists.
- [x] **Phase 2 limit tests pass unchanged** — `TestMaxLoopIterations*`, `TestMaxOutputSize*`, `TestMaxCallDepth`, `TestMaxCommandCount`, `TestLimitsDoNotImpactNormalScripts`, `TestLoopSentinelInvisible` all still green.
- [x] **Phase 3 FS contract suite passes unchanged** — every memfs / rwfs / overlayfs / mountfs contract still green; `TestBashOptionsFiles*`, `TestSeededFilesVisibleToScript`, `TestScriptCannotReadHostFile` unchanged.
- [x] **Phase 4 parser tests pass unchanged** — 18 tests in `parser` (acceptance shape, alias, four limits, syntax-error translation, redirections, heredoc body, etc.) plus 32 in `transform` (round-trip set + plugin-synth + nil guard).
- [x] **`make ci` green** — tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck. Combined coverage 56.7%.

## Tests

- `github.com/mark3labs/go-bash` — **33 tests** (22 from Phases 1–4 + 11 Phase 5 acceptance tests in `interp_bridge_test.go`). Coverage 75.3% (up from 71.9% in Phase 4).
- `github.com/mark3labs/go-bash/interp` — **6 tests**. Coverage 56.2%. The uncovered statements are the `ReplaceEnv`-mutating branch in `BuildRunner` (Stdin-nil fallback is exercised; the rest is straight-line wiring covered by the gobash-package tests too).
- `github.com/mark3labs/go-bash/ast` — 2 tests, no statements.
- `github.com/mark3labs/go-bash/fs` etc. — unchanged (8 packages).
- `github.com/mark3labs/go-bash/internal/ringbuf` — unchanged, 90.0%.
- `github.com/mark3labs/go-bash/parser` — 18 tests, 58.6% (unchanged).
- `github.com/mark3labs/go-bash/transform` — 32 tests, 55.9% (unchanged).
- Combined coverage: **56.7%** (up from 56.5%).
- Comparison fixtures: N/A this phase. Harness lands in Phase 19; fixtures begin in Phase 10.

## Decisions & gotchas discovered

- **`BuildRunner(ctx, Config)` instead of `BuildRunner(ctx, *Bash, ExecOptions)`.** The Phase 4 / Phase 5 kickoff prompts both quoted the signature `BuildRunner(ctx, *Bash, ExecOptions) (*interp.Runner, error)`. Implementing it literally would create the cycle `gobash → interp → gobash` (the new `interp` package would need to import the root `gobash` package for `*Bash` and `ExecOptions`, and the root package needs to import `interp` to call `BuildRunner`). Resolved by introducing a flat `interp.Config` struct that the caller (`gobash.(*Bash).Exec`) assembles from `*Bash` + `ExecOptions` before the call. The semantic intent — wire env/cwd/streams/FS/CallHandler into the runner — is identical, and the Phase 8 swap is one-line either way. Documented in the `interp` package header.
- **The literal SPEC.md §11 smoke test (`echo "Hello" > greeting.txt; cat greeting.txt`) is not asserted yet.** `cat` is not a mvdan/sh builtin, and the Phase 5 stub `commandExecHandler` falls through to `mvinterp.DefaultExecHandler` — which would `os/exec` host `/usr/bin/cat` and miss the VFS. The kickoff prompt called this "Phase 1's smoke test" but Phase 1's smoke test was `echo hello`. We exercise the VFS write side via `TestPhase5SmokeVFSRedirect` (which writes via `>` and reads back via `< / read`), and a direct `b.FS().ReadFile` cross-check confirms the file is in the VFS — but the `cat` form is gated on the Phase 10 `cat` built-in landing. Flagging this here so Phase 10 owns the literal-form regression.
- **Stub commandExecHandler still reaches the host for unknown commands.** Per §5.3 the *eventual* Phase 8 implementation returns `interp.ErrNotFound` for unregistered commands, never invoking `os/exec`. Today, with the pass-through middleware, mvdan/sh's `DefaultExecHandler` is the chain's terminator, so any unknown command (`cat`, `awk`, `grep`, …) will still shell out. This is an explicit Phase 5–7 sandbox gap; the `no-os-exec` lint rule lands in Phase 21 and Phase 8 closes the runtime gap. Recorded loudly in the `interp` package comment on `commandExecHandler`.
- **Env propagation: literal §5.6 reading honored, including `ReplaceEnv`.** §5.6 says "Copy back the runner's Vars into `Bash.env` (export-only) **unless** `ExecOptions.Env` was provided **without** `ReplaceEnv`." Implemented as `if opts.Env == nil || opts.ReplaceEnv { copyBack() }`. This makes `ReplaceEnv=true` plus a per-call `Env` map "start fresh from this env AND persist the script's exports as the new state" — which is the literal reading and what `TestPhase5ReplaceEnvSnapshotsExports` locks in. If a future cross-port check of just-bash TS reveals the TS code actually treats `ReplaceEnv=true` as ephemeral, this test will be the canary that flags the swap.
- **`parser.Parse` is now the single parse path.** Phase 4 put `MaxInputSize`/`MaxTokens`/etc. into the parser package but `bash.Exec` still used raw `syntax.NewParser().Parse`. As of Phase 5 those limits fire from inside `Exec` — the `TestPhase5ParserLimitsSurfaceFromExec` test makes this guarantee permanent. Side-effect: Phase 4's "MaxHeredocSize is practically unreachable because MaxInputSize dominates" caveat still applies to the public path.
- **`HandlerDir` is exported.** Phase 3 kept the recover-guarded helper private (`handlerDir`). Phase 5 lifts it into the `interp` package as `HandlerDir(ctx)` so the Phase 8 command registry — which will run inside the same package or a sibling — can re-use the same panic-safe extractor. The recover is still cheap; tightening to `HandlerCtxOk(ctx)` is contingent on a future mvdan/sh release exposing one.
- **The `interp` package name shadows `mvdan.cc/sh/v3/interp`.** Inside `bash.go` we import the new package as `bashinterp` to keep `interp` available for the mvdan symbol set used by `exportedEnv`, `countMvdanCallFrames` etc. Inside the new package itself, mvdan's interp is aliased to `mvinterp`. Mild aesthetic cost; pays for itself by making the file paths read naturally.
- **`runner.Dir` set after `interp.New`.** The Phase 3 quirk (`interp.Dir(path)` host-stats at runner-init) is preserved by setting `runner.Dir = cfg.Cwd` after `mvinterp.New(opts...)` returns. Repeated in inline comments in `BuildRunner`; future maintainers must NOT swap in `mvinterp.Dir(cfg.Cwd)` thinking it's equivalent.

## Open follow-ups (non-blocking)

- **Phase 10 owes the `cat greeting.txt` form.** Once `cat` is a registered built-in, add the literal SPEC §11 smoke test (`echo "Hello" > greeting.txt; cat greeting.txt` → `"Hello\n"`) to lock the end-to-end contract on its actual form. Today's `TestPhase5SmokeVFSRedirect` is the redirect-only stand-in.
- **Phase 8 closes the os/exec gap.** Replace the `commandExecHandler` middleware with the registry dispatch documented in §5.3. The replacement must return `interp.ErrNotFound` for unregistered commands and must NOT fall through to `DefaultExecHandler` — the lint rule arrives in Phase 21 but the sandbox guarantee should hold from Phase 8 onward.
- **Comparison-fixture harness (Phase 19) will retroactively exercise ~50 §5.7 fixtures.** When the harness lands, port the just-bash plain-shell fixtures and re-run; failures here become Phase 6 patch-via-hook tickets.
- **`interp` package coverage (56.2%) has room.** The remaining uncovered lines are mostly the path-error normalization branches and the Stdin-nil fallback. Phase 8's command tests will exercise them naturally; if not, add targeted unit tests when revisiting.
- **CallHandler is still a closure over `bash.go` state.** That's correct for Phase 5 (`MaxLoopIterations`, `MaxCommandCount`, `MaxCallDepth` are scoped per-Exec), but Phase 8 may want to lift parts of it into the `interp` package or a sibling so the registry's command-count contributions can be measured uniformly. Not blocking.
- **`exportedEnv` returns only string-typed vars.** Phase 4 noted that arrays / associative arrays should land "in Phase 5 alongside the interp bridge". Deferring to Phase 6 (expansion) where the array-aware `Vars` shape gets fleshed out — the §5.7 acceptance only exercises scalar exports.

## NEXT PHASE: Phase 6 — Expansion & Shell Features

- **Goal:** verify that the full §6.1–§6.8 expansion / shell-features matrix (parameter expansion, brace expansion, arithmetic, globbing, `[[ ]]`, redirection, control flow, command substitution, process substitution) works through the new bridge, and patch the few gaps via interp hooks or pre-Parse rewriting. Most of this is delivered "for free" by mvdan/sh per SPEC §6 — Phase 6 is a verification + targeted-patch pass, not a from-scratch implementation.
- **Read:** SPEC.md §6 in full (§6.1–§6.8), and review the Phase 5 handoff (this file). Especially note: the Phase 5 `commandExecHandler` is still a pass-through; any §6 test that needs an external command (`grep`, `awk`, `cat`) will exercise the os/exec fall-through and may behave inconsistently across CI hosts. Lean on shell builtins + redirects for Phase 6 verification.
- **Deliver:**
  - **Expansion-side runtime caps.** Wire `MaxStringLength`, `MaxBraceExpansionResults`, `MaxSubstitutionDepth`, `MaxArrayElements`, `MaxGlobOperations` from `ResolvedLimits` into the runtime path. mvdan/sh does not enforce these natively; the Phase 6 work is the hook layer that does.
  - **Acceptance per §6.x.** One acceptance test per subsection covering at minimum the SPEC-listed forms (parameter expansion ops, brace expansion shapes, arithmetic operators, glob patterns including globstar, `[[ ]]` test forms, redirection variants including process substitution if reachable, command substitution `$( )` / backticks, function-local vars).
  - **Patch the gaps.** Where mvdan/sh diverges from real bash (timezones in `printf %(...)T`, regex semantics, etc.) patch via pre-Parse rewriting or interp hooks. Cite the just-bash file:line you ported behavior from.
- **Acceptance (SPEC §6.x):** every form in §6.1–§6.8 has a Go test that runs it through `Bash.Exec`; the expansion-side hard limits trigger on pathological inputs; Phases 1–5 tests stay green.
- **Don't:**
  - Implement built-in commands (Phase 10) or the command registry (Phase 8) — verify that what mvdan/sh already provides works.
  - Reach for the host `os/exec` fall-through: §6 verification scripts must use only mvdan/sh builtins (`echo`, `printf`, `:`, `true`, `false`, `read`, `set`, `unset`, `declare`, etc.) plus redirections.
  - Lift the `commandExecHandler` stub — that's Phase 8's job.

- **Kickoff prompt for the Phase 6 session:**

  > Implement Phase 6 of go-bash per SPEC.md §6. Read AGENTS.md and HANDOFF.md first, then `handoffs/phase-5.md` for the interpreter bridge shape (`bashinterp.BuildRunner`, the pass-through `commandExecHandler`, the env-mutation rules, the recover-guarded `HandlerDir`). Phase 6 is a verification + targeted-patch pass: most of §6.1–§6.8 is delivered for free by mvdan/sh, so the deliverable is (a) one acceptance test per subsection covering the SPEC-listed forms (parameter expansion ops, brace expansion shapes, arithmetic, globbing, `[[ ]]`, redirection, command/process substitution, control flow), (b) wiring the expansion-side runtime caps (`MaxStringLength`, `MaxBraceExpansionResults`, `MaxSubstitutionDepth`, `MaxArrayElements`, `MaxGlobOperations`) from `ResolvedLimits` into the runtime path — mvdan/sh does not enforce these natively, so a CallHandler / OpenHandler / preParse interception is required; cite the file:line you ported from in just-bash, and (c) patching any divergences from real bash via interp hooks or pre-Parse rewriting (record swaps in DECISIONS.md). Lean on mvdan/sh builtins + redirects only — the Phase 5 stub `commandExecHandler` still falls through to host `os/exec` for unknown commands and that gap stays open until Phase 8. Run `make ci` until green, then call `finalize_phase` with `phase=6`, the drafted handoff markdown, the new root `HANDOFF.md` pointer, a conventional-commit subject like `feat(phase-6): expansion semantics and runtime caps`, and the kickoff prompt for Phase 7 (filesystem init & default layout).
