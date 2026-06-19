# go-bash Handoff — Phase 12 complete

- **Date:** 2026-06-18
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit; previous HEAD ec91c56)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). `golangci-lint run ./...` → **0 issues**.

## What this phase delivered

- **`procinfo.go` (NEW, 137 lines).** Pre-Run AST rewrite that
  virtualizes `$$`, `$PPID`, and `$BASHPID` against `Bash.procInfo`.
  Replaces the "simple" form `*syntax.ParamExp` (no Length / Width /
  Index / Slice / Repl / Exp / Names / IsSet / Excl / NestedParam /
  Modifiers / Flags) inside `*syntax.Word` and `*syntax.DblQuoted`
  parents with a `*syntax.Lit` carrying the desired literal value.
  Subshell scope for `$BASHPID` is computed by enumerating every
  `*syntax.Subshell` node in DFS pre-order, assigning each a unique
  counter (`pid+1`, `pid+2`, …), and resolving any `$BASHPID` position
  to its innermost enclosing subshell via `Pos.Offset()` containment.
- **`bash.go` execLocked.** Hooks `rewriteProcInfo(file, b.procInfo.PID,
  b.procInfo.PPID)` immediately after `instrumentLoops` and before
  `enforceExpansionCaps`, so the runtime expansion path always sees
  the virtualized values.
- **`phase12_procinfo_test.go` (NEW).** Six top-level acceptance tests
  with 16 subtests across all five §12 acceptance points:
  - `$$` default / custom / inside-subshell / inside-double-quote
  - `$PPID` default / `${PPID}` / custom
  - `$BASHPID` top-level / sibling-increment / nested / custom-seed /
    `${BASHPID}` braced form
  - `/proc/self/status` default / custom / cat-readable
  - `whoami` default / `whoami` with `UID=0` (ignored, always "user")
  - `hostname` default `localhost\n` / custom-VFS `/etc/hostname`
- **`phase12_fixtures_test.go` (NEW).** Wires the new
  `internal/testdata/fixtures/procinfo/` directory into the
  comparison-fixture harness.
- **Fixtures (NEW, 7 files):**
  - `internal/testdata/fixtures/procinfo/dollar_dollar.json`
  - `internal/testdata/fixtures/procinfo/ppid.json`
  - `internal/testdata/fixtures/procinfo/bashpid_top.json`
  - `internal/testdata/fixtures/procinfo/bashpid_subshell.json`
  - `internal/testdata/fixtures/procinfo/proc_self_status.json`
  - `internal/testdata/fixtures/hostname/etc_hostname_default.json`
  - `internal/testdata/fixtures/hostname/custom.json` (with seeded
    `/etc/hostname` content "box-42\n")
- **`DECISIONS.md`.** New "Phase 12 — Process info & defaults" section
  documenting (a) the AST rewrite (with mvdan/sh `vars.go:174`
  citation), (b) BASHPID per-subshell counter rules, (c) `$$` staying
  constant inside `(...)`, and (d) the four §12 items already wired in
  earlier phases (default ProcessInfo, /proc/self/status, whoami,
  hostname) cross-linked to their pinning tests.

## Acceptance criteria (from SPEC.md Phase 12)

- [x] **`$$` = `procInfo.PID` (never host PID).** Default = `1`;
  custom `ProcessInfo.PID=4242` → "4242"; constant across `(...)`
  subshells. Pinned by `TestPhase12DollarDollar` + fixture
  `procinfo/dollar_dollar.json`.
- [x] **`BASHPID` starts at virtual pid; subshell increments counter.**
  Top-level = `PID`; sibling `(...)` subshells get `PID+1`, `PID+2`;
  nested `(...)` get `PID+1`, `PID+2`, ...; references inside use the
  innermost subshell's value. Pinned by `TestPhase12BASHPID` (5
  subtests) + fixtures `procinfo/bashpid_top.json` and
  `procinfo/bashpid_subshell.json`.
- [x] **`/proc/self/status` byte-exact §11 template.** Phase 7's
  `applyDefaultLayout` writes the template; Phase 12 pins the exact
  bytes by reading the file directly and via `cat`. Pinned by
  `TestPhase12ProcSelfStatus` (default + custom-ProcessInfo
  variants) + fixture `procinfo/proc_self_status.json`.
- [x] **`whoami` always prints `user`.** Phase 10 Wave A
  `builtins/whoami` writes the literal regardless of `procInfo.UID`.
  Pinned by `TestPhase12Whoami` (including `UID=0` regression
  guard).
- [x] **`hostname` reads `/etc/hostname` (default `localhost`).**
  Phase 10 Wave A `builtins/hostname` reads via `c.FS`. Default layout
  seeds `localhost\n`; caller-supplied `Files["/etc/hostname"]` wins.
  Pinned by `TestPhase12Hostname` + fixtures
  `hostname/etc_hostname_default.json` and `hostname/custom.json`.

## Tests

- **New tests:** `phase12_procinfo_test.go` (16 subtests across 6
  top-level tests), `phase12_fixtures_test.go` (5 fixture subtests).
- **New fixtures:** 7 JSON fixtures (5 under `procinfo/`, 2 under
  `hostname/`). All pass the existing comparison harness.
- **Existing tests:** all green. Race detector clean. Total run:
  ~120s.
- **Coverage delta (root `gobash` package):** 83.5% (up from 81.0%).
  `procinfo.go`: `rewriteProcInfo` 95.8%, `rewriteProcInfoParts`
  100%, `isSimpleParamExp` 100%. Repo combined coverage **68.8%**
  (+0.1 vs Phase 11).

## Decisions & gotchas discovered

- **mvdan/sh hardcodes `$$` and `$PPID` to host PIDs at
  `interp/vars.go:174` (v3.13.1).** The `lookupVar` switch fires
  BEFORE env consultation, so we cannot virtualize these via
  `interp.Env(expand.ListEnviron(...))`. Confirmed by inspecting the
  vendored sources and by a probe test before the fix (host PIDs
  ~3.4M were visible to the script). The AST-rewrite hook is the
  only correct option short of forking mvdan. See DECISIONS.md
  "Phase 12 — Process info & defaults".
- **`BASHPID` was already falling through to env lookup** (it's not
  in mvdan's switch), but a single env value cannot satisfy "each
  subshell increments a counter" because mvdan's subshell clones env
  and we have no hook to mutate it on entry. AST rewrite is uniform
  with `$$`/`$PPID` and simpler than injecting `BASHPID=N` synthetic
  assignments at every subshell body start.
- **`$$` stays constant inside `(...)` — pinned regression.** Real
  bash semantics: `$$` is the calling shell's pid, not the
  subshell's. `$BASHPID` is the per-subshell-fork pid. SPEC §12 only
  states "$$ = virtualPid (never the host PID)" without spelling out
  the subshell behavior; we followed real bash + just-bash
  semantics. Pinned by `TestPhase12DollarDollar/inside_subshell`.
- **Subshell scope = lexical `*syntax.Subshell` only.** Background
  `&`, process substitution `<(cmd)`, and pipeline stages (all of
  which fork in real bash) do NOT bump `$BASHPID` under this rule.
  No fixtures require the broader behavior; documented in
  DECISIONS.md as an explicit narrow-by-design choice. Phase 21 may
  revisit if a real-world script breaks on it.
- **Complex param-expansion forms (`${BASHPID:-x}`, `${#PPID}`,
  `${PPID:0:1}`, etc.) are NOT rewritten.** They fall through to
  mvdan's expander, which then hits the hardcoded host-pid path for
  `$PPID` and the env path for `$BASHPID`. The `isSimpleParamExp`
  predicate mirrors mvdan's own unexported
  `(*ParamExp).simple()` — same surface area. Documented; no
  fixtures currently use these forms.
- **Coverage of `rewriteProcInfo` is 95.8%, not 100%,** because the
  `if file == nil` nil-guard is dead under the current call site
  (parser.Parse never returns nil + nil). Left in defensively.

## Open follow-ups (non-blocking)

- **Complex param-expansion of virtual PIDs.** If a fixture demands
  `${PPID:-default}` semantics, extend `rewriteProcInfo` to walk into
  `pe.Exp`/`pe.Slice`/`pe.Repl` and rewrite the inner Word's parts.
- **Background / process-substitution / pipeline BASHPID forks.** If
  parity audit demands real-bash behavior, switch from "lexical
  `Subshell` scope" to "any runtime fork point". Would require either
  AST rewrites for those cases or a mvdan/sh-level hook.
- **Phase 11 follow-ups carried forward** (still open):
  - `source` Args propagation
  - `/bin/cd` chdir back-channel
  - `getopts` OPTIND persistence
  - `fg`/`bg` registration
  - `hash` cache state
  - `trap` EXIT delivery
  - `let` bit-ops/comparison/ternary

## NEXT PHASE: Phase 13 — Transform API

- **Goal:** Port `src/transform/` to Go. Plugin pipeline that observes
  and (optionally) rewrites Exec inputs/outputs.
- **Spec to read:** `SPEC.md` §13 (13.1 types, 13.2 pipeline, 13.3
  built-in plugins — CommandCollectorPlugin & TeePlugin, 13.4
  integration with `Bash.Exec`).
- **Packages/files to create:**
  - `transform/types.go` — Plugin interface, hook surface types
  - `transform/pipeline.go` — registration / dispatch
  - `transform/plugins/collector/` — CommandCollectorPlugin
  - `transform/plugins/tee/` — TeePlugin
  - Wire into `Bash.Exec` via a new `Bash.plugins` field
- **Public symbols to deliver:** `transform.Plugin`, hook types
  (BeforeExec, AfterExec, BeforeCommand, AfterCommand, …),
  `transform.NewPipeline`, the two built-in plugin constructors. Exact
  names in SPEC §13.1–§13.4.
- **Prerequisites:** met — Phase 12 closed all process-identity gaps;
  transform pipeline depends on the registry (Phase 8) and Exec
  shape (Phase 5), both stable.
- **Kickoff prompt for the next session:**
  > Implement Phase 13 (transform API) of go-bash per SPEC.md §13.
  > Read AGENTS.md, HANDOFF.md, `handoffs/phase-12.md`. Port
  > `src/transform/` to Go: `transform/types.go` (Plugin interface
  > and hook surface), `transform/pipeline.go` (registration +
  > dispatch), and the two built-in plugins
  > `transform/plugins/collector/` (CommandCollectorPlugin) and
  > `transform/plugins/tee/` (TeePlugin). Wire the pipeline into
  > `Bash.Exec` per SPEC §13.4 — likely via a new `Bash.plugins
  > []transform.Plugin` field populated from
  > `BashOptions.TransformPlugins`. Add fixtures and unit tests per
  > the §13 acceptance criteria. Update DECISIONS.md if any
  > divergence from the TS surface is discovered. Run `make ci`
  > until green AND `golangci-lint run ./...` until clean. Then
  > call `finalize_phase` with `phase=13`, the drafted handoff
  > markdown, the new root `HANDOFF.md` pointer, a
  > conventional-commit subject like `feat(phase-13): transform
  > pipeline`, and the kickoff prompt for Phase 14 (optional
  > SQLite runtime).
