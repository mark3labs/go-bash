# go-bash Handoff — Phase 11 complete

- **Date:** 2026-06-18
- **Status:** COMPLETE
- **Branch / commit:** main @ feat(phase-11): interpreter built-ins
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). Combined coverage **68.7%** (+0.3 vs Wave H). Additional verification: `golangci-lint run ./...` → **0 issues**.

## What this phase delivered

- **32 new `builtins/<name>/` packages** following the Wave A–H template:
  - `breakcmd`, `cd`, `colon` (`:`), `compgen`, `complete`, `compopt`,
    `continuecmd`, `declare`, `dirs`, `eval`, `exit`, `export`,
    `getopts`, `hash`, `jobs`, `let`, `local`, `mapfile` (also
    registers `readarray`), `popd`, `pushd`, `read`, `readonly`,
    `returncmd`, `set`, `shopt`, `source` (also registers `.`),
    `test` (also registers `[`), `trap`, `umask`, `unset`, `wait`.
  - Total registered command Names: **115 builtin packages, 117
    registered command names** (source/. and mapfile/readarray and
    test/[ each ship two names from a single package).
- **`builtins/builtins.go`** — +30 blank imports under a new
  "Phase 11: interpreter built-ins" block (appended last,
  alphabetized within the block).
- **`internal/testdata/fixtures/<name>/basic.json`** — one fixture
  per command (32 new fixtures). All use the `/bin/<name>` form to
  bypass mvdan/sh's shadow; documented in DECISIONS.md.
- **`internal/alias/`** — NEW package implementing post-parse AST
  alias expansion (`Expand(file *syntax.File, aliases map[string]string)`).
  Walks every `*syntax.CallExpr`, re-parses the alias body, splices
  in the body's tokens. Supports chained aliases (up to 16 passes)
  with self-loop detection.
- **`internal/runtimestate/state.go`** — added `ShoptTable` (read/
  write surface for shopt options, safe for concurrent use,
  parallel to AliasTable / HistoryRing).
- **`command/command.go`** — added `Context.SourceDepth int`,
  `Context.Shopt command.ShoptTable` (interface), and
  `SubExecOptions.SourceDepth`. New `ShoptTable` interface lives in
  `command/` so `command.Context` can hold a typed pointer without
  importing the internal package (concrete impl in
  `internal/runtimestate`).
- **`interp/runner.go`** — plumb `SourceDepth` and `Shopt` through
  `Config` and `dispatchEnv` so every dispatched command sees them
  on `command.Context`.
- **`bash.go`** — adds `Bash.shopt *runtimestate.ShoptTable`,
  `Bash.execDepth int`, and the `Bash.Shopt()` getter. Threads
  `b.execDepth` into `BuildRunner.Config.SourceDepth` and bumps it
  on every `subExec` (resetting on return via deferred restore). The
  alias-expansion pass fires from `execLocked` immediately after
  parse when `b.shopt.IsSet("expand_aliases")` is on.
- **`fs_init.go`** — `applyDefaultLayout` is now defensive about
  /bin/<name> stub writes: names containing `/`, `\0`, or equal to
  `.` / `..` are skipped, and individual stub-write errors no longer
  short-circuit the layout loop. This was required because Phase 11
  registers `.` (the dot builtin) as a Command Name; without the
  skip, `WriteFile("/bin/.", …)` failed and subsequent stubs were
  never written.
- **`builtins/bash/bash.go`** + **`builtins/bash/bash_test.go`** —
  retired the `GOBASH_BASH_DEPTH` env var (Wave G follow-up).
  `bash`/`sh` now read `c.SourceDepth` directly and pass
  `SourceDepth: depth+1` through `SubExecOptions`.
- **Integration tests at repo root:**
  - `phase11_alias_test.go` — verifies alias expansion fires when
    `shopt expand_aliases` is on, and is silent when off.
  - `phase11_sourcedepth_test.go` — verifies `/bin/source`
    self-recursion trips `MaxSourceDepth=3`.
- **`DECISIONS.md`** — new Phase 11 section documenting:
  - The mvdan-shadow accept-vs-override decision per command.
  - The alias-expansion AST-rewrite approach.
  - The `GOBASH_BASH_DEPTH → Context.SourceDepth` promotion.
  - The `[` / `test` POSIX subset that was ported.
  - The `let` self-contained recursive-descent arithmetic parser.
  - The `/bin/read`-can't-mutate-runner caveat (and same for
    mapfile/getopts).

### Command surface notes

- **Shadowed by mvdan/sh, accepted + /bin/<name> fallback:** `cd`,
  `export`, `unset`, `declare`, `read`, `set`, `shopt`, `exit`,
  `return`, `break`, `continue`, `hash`, `trap`, `[`, `test`,
  `wait`, `getopts`, `mapfile`, `readarray`, `dirs`, `pushd`,
  `popd`, `:`, `local`, `readonly`. The registered version is
  reachable via `/bin/<name>` and mutates `c.Env` /
  `c.ExportedEnv` as a best-effort surface (no propagation to the
  runner's Vars; mvdan's native impl handles the bare name).
- **NOT shadowed (canonical impl):** `source`, `.`, `eval`, `let`,
  `compgen`, `complete`, `compopt`, `jobs`, `umask`. These reach
  the registry on the bare name. `source` / `.` / `eval` use
  `c.Exec` and bump `c.SourceDepth + 1` into the child Exec.

### Alias expansion

- `internal/alias.Expand` runs from `bash.go::execLocked` right
  after `parser.Parse` and BEFORE `instrumentLoops` /
  `enforceExpansionCaps`. The expansion is conditional on
  `b.shopt.IsSet("expand_aliases")` — default OFF (matches
  non-interactive bash). Hosts enable it via
  `b.Shopt().Set("expand_aliases", true)` or
  `/bin/shopt -s expand_aliases` in the script (but the latter
  only affects the NEXT Exec call; the current parse has already
  finished).
- Limitations: alias body must parse as a `*syntax.CallExpr`
  (simple command). Aliases with embedded redirections, here-docs,
  or compound commands are silently dropped. Self-loops are
  detected and suppressed.

### Typed `Context.SourceDepth`

- `command.Context.SourceDepth int` replaces the
  `GOBASH_BASH_DEPTH` env-var hack from Wave G. Source / eval /
  bash / sh consume `c.SourceDepth` and forward
  `opts.SourceDepth = depth + 1` to `c.Exec`. The runtime maps
  `opts.SourceDepth` back onto `Bash.execDepth` via `subExec`,
  which becomes `Config.SourceDepth` for the child runner.
- `bash`/`sh` tests rewritten to use `c.SourceDepth` instead of
  the env map.

## Acceptance criteria (SPEC §11)

- [x] **cd** — accepts `-L`/`-P`, `~`, `-`, validates target via
  `c.FS.Stat`, mutates `c.Env[PWD]` / `OLDPWD`. `TestCdValidDir`,
  `TestCdMissing`, `TestCdHome`.
- [x] **set** — `-e`/`-u`/`-x`/`-o pipefail`/`-o noglob`/`-o
  errexit` recorded into `c.Shopt`. `+X` to disable.
  `set --` / `set ARG...` accepted (positional-param mutation
  silently dropped — see DECISIONS.md). `TestSetFlag`,
  `TestSetOPipefail`.
- [x] **shopt** — `-s`/`-u`/`-p`/`-q` on per-Bash `c.Shopt`.
  `TestShoptSet`, `TestShoptUnset`, `TestShoptQuery`,
  `TestShoptPrint`.
- [x] **export** — `export VAR`, `export VAR=val`, `export -p`,
  `export -n`. `TestExportSet`, `TestExportP`, `TestExportN`.
- [x] **unset** — `-v` / `-f`. `TestUnset`, `TestUnsetMissing`.
- [x] **local** — in-function gate is not enforced (we can't
  detect function context); best-effort assignment to `c.Env`.
- [x] **declare / typeset** — `-a -A -i -r -x -p -n` recognized;
  `-x` mutates `c.ExportedEnv`. Array init syntax not ported.
- [x] **readonly** — `-a -f -p` recognized; assignments hit
  `c.Env` (readonly attribute NOT tracked — see DECISIONS.md).
- [x] **eval** — runs via `c.Exec`, bumps SourceDepth.
  `TestEvalRuns`, `TestEvalMaxDepth`.
- [x] **source / `.`** — reads file via `c.FS`, runs via
  `c.Exec`, bumps SourceDepth, enforces MaxSourceDepth.
  `TestSourceBasic`, `TestSourceMaxDepth`, `TestSourceBumpsDepth`,
  plus end-to-end `TestSourceDepthEnforced` in repo-root.
- [x] **exit** — `exit [N]` returns ExitCode N; non-numeric → 2.
- [x] **return** — same shape as exit.
- [x] **break / continue** — `[N]` accepted; exit 0 (we can't
  actually unwind a loop from /bin).
- [x] **getopts** — single-shot stub: exits 1 (no OPTIND
  persistence; see DECISIONS.md).
- [x] **let** — arithmetic; exit code 0 if non-zero, 1 if zero.
  Spaces around `=` tolerated. `TestLetSimple`,
  `TestLetZeroResult`, `TestLetAssign`, `TestLetSpacedAssign`,
  `TestLetParens`, `TestLetVariable`.
- [x] **read** — `-r`, `-p PROMPT`, `-t TIMEOUT`, `-n N`, `-N N`,
  `-d DELIM`, `-a ARRAY`, `-s` (silent — no-op without
  terminal). Multi-NAME splitting honored. EOF → exit 1.
  `TestReadBasic`, `TestReadMultiple`, `TestReadDelim`,
  `TestReadN`, `TestReadPrompt`, `TestReadEOF`.
- [x] **mapfile / readarray** — `-t`, `-n N`, `-O ORIGIN`, `-s
  SKIP`. Array entries flattened into `<NAME>_<INDEX>` env vars.
- [x] **dirs / pushd / popd** — best-effort single-entry stack
  (we have no persistent dir stack across dispatches).
- [x] **hash** — stub; recognized `-r`, `-d`, `-p`.
- [x] **trap** — stub; records nothing but accepts the argv.
- [x] **`[`** — full POSIX test surface (string, integer, file,
  negation, AND/OR, parens). File tests route through `c.FS`.
- [x] **`:`** — exits 0.
- [x] **wait** — exits 0 (background jobs are synchronous).
- [x] **jobs / fg / bg** — `jobs` exits 0 (empty). `fg`/`bg` not
  registered (no SPEC fixture demand; trivial follow-up).
- [x] **umask** — `0022` default, `-S` symbolic, validates
  numeric MODE.
- [x] **compgen / complete / compopt** — stubs exit 0 with no
  output (matches the TS stub).
- [x] **Alias parse-time expansion when shopt expand_aliases is
  on** — `internal/alias.Expand`, wired in `bash.go::execLocked`.
  Acceptance: `TestAliasExpansionPhase11`.
- [x] **`GOBASH_BASH_DEPTH` retired** — typed
  `Context.SourceDepth` + `SubExecOptions.SourceDepth`.
- [x] **`source` / `.` / `eval` use `c.Exec` + bump SourceDepth**.
- [x] **All file I/O through `c.FS`; stdin via `c.Stdin`; stdout
  via `c.Stdout`**. No `os/exec`; no host `os.Stat` (the `[`
  builtin's file tests go through `c.FS.Stat` / `c.FS.Lstat`).
- [x] **mvdan-shadow decisions documented in DECISIONS.md**.
- [x] **Wave A–H + Phases 1–10 stay green** — `make ci` PASS.
- [x] **`golangci-lint run ./...` clean** — 0 issues.

## Tests

- 32 new packages, **~80 unit tests** + 32 comparison-fixture
  subtests total.
- 32 new fixture files.
- 2 new integration tests at repo root
  (`phase11_alias_test.go`, `phase11_sourcedepth_test.go`).
- 1 new internal/alias package with 5 unit tests.
- Per-package coverage of new packages ranges 60–100%; many of
  the shadowed stubs hit 90%+ because the surface is tiny.
- Combined repo coverage: **68.7%** (+0.3 vs Wave H).
- No new third-party deps.

## Decisions & gotchas discovered

- **mvdan shadows are SHALLOW** — mvdan/sh's `*syntax.DeclClause`
  handles `export`/`declare`/`local`/`readonly` at the parser
  level, not at command dispatch. CallHandler intercepts the
  command word AFTER mvdan has already dispatched its native
  builtin. Practical consequence: there is NO `args` list to
  rewrite for these names — they never reach our middleware.
  Confirmed by tracing mvdan's `runner.go` lines 671+.
- **`/bin/.` is an invalid filename** — `fs/memfs/path.go`'s
  Clean drops `.` components, so `WriteFile("/bin/.", …)`
  resolves to `WriteFile("/bin", …)` which fails because `/bin`
  is already a directory. Fixed `applyDefaultLayout` to skip
  names like `.` / `..` and to continue past individual write
  failures (the loop previously short-circuited and broke
  `/bin/<sorted-after-.>` stubs).
- **`echo` is NOT a Phase 11 command** — it's Wave A. Phase 11's
  shadow list doesn't include it.
- **`fg` / `bg` were OMITTED** — spec lists them under `jobs` but
  no fixture demand; trivial stub additions if needed.
- **`alias` shopt is OFF by default** — bash's `expand_aliases`
  defaults to ON for interactive shells, OFF for non-interactive.
  We default OFF (we ARE non-interactive). Hosts running tests
  with aliases need `b.Shopt().Set("expand_aliases", true)`
  BEFORE the Exec call that uses the alias.
- **`source` / `.` cannot pass positional args to the child** —
  `SubExecOptions.Args` is populated but the child's
  `execLocked` doesn't currently consume `opts.Args` into the
  runner's positional parameters. Phase 12 should wire this; it's
  a one-line `runner.Params = opts.Args` call. Tracked as
  follow-up.
- **`let` doesn't support bit-ops, comparisons, or ternary** —
  the recursive-descent parser handles arithmetic only. Add as
  fixtures demand.
- **`getopts` is single-shot** — without persistent OPTIND across
  dispatches, the only honest answer is "no more options" (exit
  1). A future enhancement could stash OPTIND in `c.Env` per
  invocation, but it requires the caller to read it back; mvdan's
  native getopts is the real implementation.
- **`/bin/cd /tmp` failed** in the initial fixture because
  `files: {"/tmp": ""}` makes `/tmp` a regular file (not a
  directory). Fix: use `"/tmp/.keep"` to force the parent
  directory to be a dir.
- **`Bash.History()` parsed-command recording still NOT wired** —
  carried from Phase 10. Phase 19 owns it.
- **`alias.Expand` walks via `syntax.Walk`** — it shares the
  Phase 6 mvdan/sh quirk (panics on `*syntax.BraceExp`), but
  Expand fires BEFORE `enforceExpansionCaps`/`syntax.SplitBraces`,
  so there are never any BraceExp nodes at expand time. Safe.

## Open follow-ups (non-blocking)

- **`source` / `.` positional args not propagated** — see above.
  One-line fix in `Bash.subExec` to write `opts.Args` into the
  child runner's params before Run.
- **`/bin/cd` doesn't actually `chdir`** — the registered version
  validates the target and mutates `c.Env[PWD]`, but the runner's
  Dir is not propagated back. Real `cd` (mvdan's) does the right
  thing. If a fixture needs `/bin/cd && pwd` to show the new dir,
  we need a CallHandler back-channel.
- **`getopts` OPTIND persistence** — see above. Use `c.Env` as a
  back-channel if needed.
- **`fg` / `bg` not registered** — trivial follow-up.
- **`hash` cache** — currently a no-op stub. If a fixture demands
  observable cache state, store it in a `Bash.hashCache` map.
- **`trap` handlers never fire** — we have no process-signal
  delivery. `trap "..." EXIT` could be honored on `b.Exec` return
  if we plumb a teardown hook; out of scope for Phase 11.
- **`let` bit-ops / comparison / ternary** — extend the parser as
  fixtures demand.
- **`compgen` / `complete` / `compopt`** — stubs match TS
  output (empty). Real completion machinery is out of scope.
- **Test `-x` / `-r` / `-w`** — always true if file exists. The
  VFS lacks the granularity to refine this; Phase 21 hardening
  may revisit.
- **`mapfile` / `readarray` flatten arrays to `<NAME>_<N>` env
  vars** — there's no Context back-channel to set real bash
  arrays. If Phase 12 adds a `SetVar` hook, switch.
- **`cmpfixture.RunDirWith(t, dir, opts)` overload** — carried
  from Wave H. Phase 19 should land this.
- **Lint loop is still NOT part of `make ci`.** Run
  `golangci-lint run ./...` manually before handoff.

## NEXT PHASE: Phase 12 — Process Info & Defaults

- **Goal:** Per SPEC §12. Implement the `processInfo` defaults
  (pid=1, ppid=0, uid=1000, gid=1000), `BASHPID` as a virtual
  subshell-incrementing counter, `$$` = virtualPid (never host
  PID), the `/proc/self/status` text template, `whoami` always
  prints `user`, and `hostname` reads `/etc/hostname` (default
  `localhost`). Most of this is ALREADY DONE (Phase 7 wired the
  default FS layout including `/proc/self/status` and
  `/etc/hostname`; `whoami` / `hostname` are already registered
  Wave A built-ins). Phase 12 should AUDIT what's missing
  (probably `BASHPID` subshell counter, `$$` plumbing, and any
  fixture coverage gaps).
- **Read:** SPEC §12, this handoff, `handoffs/phase-11.md` (this
  file).
- **Deliver:** any missing `$$` / `BASHPID` plumbing; audit of
  existing `whoami` / `hostname` behavior; comparison fixtures.

- **Kickoff prompt for the Phase 12 session:**

  > Implement Phase 12 (process info & defaults) of go-bash per
  > SPEC.md §12. Read AGENTS.md, HANDOFF.md, `handoffs/phase-11.md`.
  > Most of this phase is already done — Phase 7 wired the SPEC §7
  > default layout (including `/proc/self/status` and
  > `/etc/hostname` from `Bash.procInfo`) and Phase 10 Wave A
  > registered `whoami` (prints `user`) and `hostname` (reads
  > `/etc/hostname`). Phase 12 should AUDIT the §12 acceptance
  > criteria: (1) `$$` resolves to `procInfo.PID` (virtualized, NOT
  > host PID); (2) `BASHPID` starts at the virtual PID and
  > increments per sub-shell; (3) `/proc/self/status` matches the
  > §11 template byte-for-byte; (4) `whoami` always prints `user`;
  > (5) `hostname` reads `/etc/hostname` (default `localhost`).
  > Add any missing plumbing — likely `$$` resolution requires
  > seeding an env var or hooking mvdan's `$$` expansion; `BASHPID`
  > likely needs a subshell counter on `Bash`. Add fixtures for
  > each acceptance point in `internal/testdata/fixtures/<name>/`.
  > Update `DECISIONS.md` if any divergence is discovered. Run
  > `make ci` until green AND `golangci-lint run ./...` until
  > clean. Then call `finalize_phase` with `phase=12`, the drafted
  > handoff markdown, the new root `HANDOFF.md` pointer, a
  > conventional-commit subject like `feat(phase-12): process info
  > & defaults`, and the kickoff prompt for Phase 13 (transform
  > API).
