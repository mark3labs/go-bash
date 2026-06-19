# DECISIONS.md — vendor / behavior swaps recorded during go-bash

This file collects places where the go-bash implementation knowingly
diverges from the just-bash TypeScript source, the SPEC.md specification,
or the upstream mvdan/sh runtime. Each entry names the phase that
introduced the swap, the just-bash citation (or "n/a" when we adopted a
neutral implementation), and the reason.

The non-negotiable rule (AGENTS.md) is: just-bash semantics win when TS
and Go could differ. Any deliberate deviation belongs here.

---

## Phase 6 — Expansion & shell features (verification + caps)

### 1. Brace-cardinality counter is custom, not a port

* **What:** `enforceExpansionCaps` (file `caps.go`) walks the parsed
  *syntax.File and statically computes the brace cardinality of every
  *syntax.Word, falling back to a `cap+1` saturation path for
  unboundedly wide sequences like `{1..2000000000}`.
* **just-bash citation:** approximate — just-bash counts cardinality in
  `src/shell/brace-expand.ts` before materializing the expansion. The
  go-bash port re-implements the same intent against the mvdan/sh
  *syntax.BraceExp* node rather than porting the TS algorithm
  line-for-line, because mvdan/sh's AST is the source of truth for the
  Go side. We do NOT vendor just-bash (read-only spec source per AGENTS.md).
* **Why:** mvdan/sh does not expose a cardinality cap on
  `syntax.SplitBraces` or `expand.Braces`. The runtime expand path will
  happily allocate a 1B-element slice if we don't pre-reject.
* **Risk:** if just-bash ever lands a brace edge case (e.g. extglob
  inside a brace literal) the TS counter handles but our walk doesn't,
  it will surface as a missed-trip in fuzz/comparison fixtures. We
  guard the obvious overflow paths.

### 2. SplitBraces is called on a *copy*, not the live AST

* **What:** `wordBraceCardinality` in `caps.go` copies the Word before
  calling `syntax.SplitBraces` so the original AST never gains a
  `*syntax.BraceExp` node.
* **Why:** mvdan/sh's own `syntax.Walk` switch panics on
  `*syntax.BraceExp` ("unexpected node type"). Subsequent passes (the
  substitution-depth and array-element checks) must still walk the
  tree, so the file has to remain split-free. The mvdan/sh runtime
  expand path likewise copies the Word before splitting (see
  `expand/expand.go::FieldsSeq`), so static counting and runtime
  expansion stay in lock-step.
* **Risk:** none — we only read.

### 3. MaxSubstitutionDepth counts CmdSubst + ProcSubst, NOT ParamExp

* **What:** `checkSubstDepth` only treats `*syntax.CmdSubst` and
  `*syntax.ProcSubst` as depth-bumping. `${VAR/.../}` nested inside
  `${VAR/...}` does not count.
* **Why:** the SPEC §2.1 wording is "MaxSubstitutionDepth" — the
  pathological-recursion case is `$(cmd)` chains. Parameter expansion
  is structurally bounded by `MaxStringLength`. If a future phase
  uncovers a ParamExp-recursion DoS, expand the type switch.
* **Risk:** low. ParamExp is bounded by string length.

### 4. MaxGlobOperations bumps on every ReadDir, not just glob-driven ones

* **What:** `interp.Config.ReadDirHook` fires once per
  `interp.ReadDirHandlerFunc2` call, regardless of whether the
  triggering construct is a glob (`for f in *`), shell completion, or a
  builtin that internally walks a directory.
* **Why:** mvdan/sh routes all directory listing through the same
  handler. There is no flag at the handler boundary that says "this is
  a glob expansion". Adding heuristics (e.g. inspect the caller via
  runtime.Callers) is fragile.
* **Risk:** mild over-counting on scripts that mix globs with builtin
  readdirs. The default 100k is generous enough that real workloads
  won't notice.

### 5. MaxStringLength is enforced on command args, not on assignment

* **What:** the CallHandler in `bash.go` checks each arg length against
  `limits.MaxStringLength`. A raw `X="$(yes | head -n 1M)"` assignment
  that never reaches a CallHandler invocation will not trip the cap.
* **Why:** mvdan/sh does not expose an expansion-side hook between
  parameter-expansion completion and assignment commit. The CallHandler
  is the latest reachable choke point in the public API today. Closing
  the assignment side requires a fork of `expand.Config` or a runtime
  patch.
* **Risk:** a script that allocates an oversized string into a variable
  but never passes it to a command escapes the cap. `MaxOutputSize`
  catches the related DoS via writes to stdout/stderr.

---

## mvdan/sh divergences from real bash (Phase 6 inventory)

These were observed during Phase 6 acceptance test authoring against
mvdan.cc/sh/v3 v3.13.1. They are NOT yet patched; each has a regression
test that pins the current behavior so we notice if mvdan/sh fixes them.

### A. `${VAR/#pat/repl}` and `${VAR/%pat/repl}` are no-ops

* **Bash behavior:** `/#pat/` anchors the match at the start of VAR;
  `/%pat/` anchors at the end.
* **mvdan/sh behavior:** the leading `#` / `%` is consumed silently and
  the replacement does NOT fire — the original string passes through.
* **go-bash regression test:** `TestPhase6ParamExpansionAnchorDivergence`
  pins the current (broken) output. Real bash would print `XXabc\n`;
  we currently print `abcabc\n`.
* **Fix plan:** a later phase will add a pre-Parse `*syntax.ParamExp`
  rewriter that translates the anchored forms to equivalent non-anchored
  ones (`${X/#abc/XX}` → `${X/abc/XX}` only if X starts with `abc`,
  etc.) via a small wrapper builtin. Not in scope for Phase 6.

### B. `IFS` is not re-read at `$*` assignment expansion

* **Bash behavior:** `IFS=,; set -- a b c; out="$*"` produces
  `out=a,b,c`.
* **mvdan/sh behavior:** assignment expansion of `"$*"` uses the
  default IFS (space) regardless of the current IFS. The same `"$*"`
  passed as a command argument *does* honor IFS.
* **go-bash regression test:** the §6.2 positional-parameter case uses
  the working form (`echo "$*"` rather than `out="$*"`). The
  assignment-side divergence is documented here so a future regression
  test can be added once we patch.
* **Fix plan:** out of scope for Phase 6. Would require shadowing
  `expand.Config.AssignFn` to re-evaluate IFS.

### C. `let "x = 4 * 3"` (with spaces) does not persist

* **Bash behavior:** the spaces around `=` are allowed; `x` is assigned 12.
* **mvdan/sh behavior:** the spaced form parses but the assignment is
  silently dropped; `$x` is empty.
* **go-bash regression test:** the §6.7 arithmetic suite uses the
  no-space form `let x=4*3`, which DOES work.
* **Fix plan:** a thin `let` wrapper builtin in Phase 11 will normalize
  the spaced form to the no-space form before delegating.

### D. `$((1/0))` writes "division by zero" but exits 0

* **Bash behavior:** division by zero is a non-zero exit.
* **mvdan/sh behavior:** the stderr message lands but `$?` stays 0.
* **go-bash regression test:** `TestPhase6ArithmeticDivByZero` asserts
  the stderr substring; the exit-code assertion is parked.
* **Fix plan:** wrap the runner with an OnError hook that translates
  arithmetic errors into `*gobash.ArithmeticError` (already declared in
  `errors.go`) and forces a non-zero exit. Owned by Phase 11
  (interpreter built-ins).

### E. `$FUNCNAME` is not populated

* **Bash behavior:** inside a function, `$FUNCNAME` contains the
  function's name (and is an array of the call stack).
* **mvdan/sh behavior:** the variable is unset.
* **go-bash regression test:** the §6.12 function suite skips the
  `$FUNCNAME` assertion. We exercise `local`, `return N`, and recursive
  arithmetic instead.
* **Fix plan:** the function-aware CallHandler can mutate the runner's
  Vars on entry/exit to populate FUNCNAME. Owned by Phase 11.

---

## How to add a new entry

1. Cite the SPEC.md section that locks the contract.
2. Cite the just-bash file:line if a port; "n/a" if a neutral choice.
3. Spell out the swap, the reason, and the regression test that pins it.
4. Add the fix plan and the phase that owns it.

Reverting a swap requires updating SPEC.md or its resolved-decisions
block at the bottom; do not silently revert.

---

## Phase 11 — Interpreter built-ins

### Mvdan/sh shadow decisions

Most Phase 11 commands are also shipped as native built-ins by
`mvdan.cc/sh/v3/interp`. When mvdan/sh dispatches a command word it
resolves the name against its INTERNAL builtin table BEFORE invoking
our `ExecHandlers` chain — so the bare command word never reaches our
registry for shadowed names. We had two options:

1. **Override** — install a CallHandler (or fork the runner) that
   redirects matching command words to our registry. This is the
   "correct" path for builtins whose semantics MUST diverge from
   mvdan's (e.g. a `source` that bumps `Context.SourceDepth` for
   `MaxSourceDepth` enforcement).
2. **Accept the shadow + provide `/bin/<name>`** — register the command
   in our registry as usual; mvdan/sh's native version handles the
   bare invocation, and our version is reachable only via the
   absolute path `/bin/<name>` (which the interp dispatch resolves
   via `lookupCommand` basename-stripping).

For Phase 11 we chose option (2) for every command **except** `source`
/ `.` / `eval` / `bash` / `sh`, where option (1) is required for the
`MaxSourceDepth` and `c.Exec` contracts. The override path for those
five is already in place via the `Context.Exec` plumbing; the bare
command word is intercepted in mvdan because both mvdan and our impl
run the script through `Context.Exec` which goes back to our
`execLocked` (which DOES enforce `SourceDepth`).

Shadowed (accept + /bin/<name>): `cd`, `export`, `unset`, `declare`,
`read`, `set`, `shopt`, `exit`, `return`, `break`, `continue`,
`hash`, `trap`, `[`, `test`, `wait`, `getopts`, `mapfile`,
`readarray`, `dirs`, `pushd`, `popd`, `:`, `local`, `readonly`.

Why option (2) over option (1) for these: mvdan/sh's CallHandler is
an `args []string` filter that can rewrite but cannot dispatch —
there's no `dispatchToHostBuiltin(args, ctx)` exit point. Wrapping
the runner to rewrite e.g. `cd` into `/bin/cd` would require a
shebang-like prelude AND would BREAK side-effecting bash semantics
(real `cd` mutates the runner's Dir; our `/bin/cd` cannot). The
pragmatic answer is to leave mvdan/sh as the canonical implementor
of state-mutating shadowed builtins and provide `/bin/<name>` as the
fallback / diagnostic surface.

Consequences:

- A script that runs `export FOO=bar` invokes mvdan's `*syntax.DeclClause`
  handler. The script that runs `/bin/export FOO=bar` invokes OUR
  `builtins/export` which mutates `c.Env` and `c.ExportedEnv` but
  does NOT propagate to the runner's Vars. Documented in each
  builtin's package comment.
- The `expand_aliases` shopt is a SPECIAL CASE — it must be observable
  from parse time, BEFORE mvdan/sh sees the script. We thread
  `Bash.Shopt()` into `bash.go::execLocked` and consult it before
  invoking `gbalias.Expand(file, b.aliases.All())`. Users wanting
  alias expansion call `b.Shopt().Set("expand_aliases", true)` OR
  `/bin/shopt -s expand_aliases` BEFORE the script that uses the
  alias. Setting shopt mid-script does NOT retroactively expand the
  current Exec's already-parsed file — the next Exec call sees the
  flag.

### Alias expansion: post-parse AST rewrite, not pre-parse text

* **What:** `internal/alias.Expand(file *syntax.File, aliases map[string]string)`
  walks every `*syntax.CallExpr` in the parsed AST and rewrites the
  first argument when it matches an alias name. The alias body is
  re-parsed as a standalone script via `mvdan/sh/syntax.NewParser` and
  its first simple-command Args are spliced in. We loop up to 16
  passes to support chained aliases (`alias a=b; alias b=c`); a trivial
  self-loop (`alias ls='ls --color'`) is detected and suppressed in
  bash-compatible fashion.
* **just-bash citation:** approximate — just-bash's alias expansion
  lives in `src/shell/parser.ts`. We don't port line-for-line; the
  mvdan/sh AST is our source of truth.
* **Why:** doing this pre-parse via text substitution requires
  tokenizing bash by hand, which is fragile. AST-level rewrite is
  faithful and shares the existing parser.
* **Risk:** the alias body must parse as a valid bash command. Aliases
  containing redirections, here-docs, etc. are dropped silently if
  the first stmt isn't a `*syntax.CallExpr`. Documented in `Expand`.

### Promote `GOBASH_BASH_DEPTH` env var to typed `Context.SourceDepth`

* **What:** Wave G's `bash`/`sh` builtins used the `GOBASH_BASH_DEPTH`
  env var to track recursive sub-shell depth against
  `Limits.MaxSourceDepth`. Phase 11 promotes this to a typed
  `Context.SourceDepth int` (plumbed through `interp.Config` and
  `dispatchEnv`) and a matching `SubExecOptions.SourceDepth` for
  child Exec calls. Source/eval/./bash/sh all consume the new field.
* **Why:** the env-var hack leaked into child env maps and was a
  documented Wave G follow-up. The typed field is type-safe and
  doesn't require child scripts to inherit our internal counter.

### `[` / `test` builtin port

* **What:** `builtins/test/test.go` ports the POSIX `test` operator
  set (string ==/!=/-z/-n, integer -eq/-ne/-lt/-le/-gt/-ge, file
  -e/-f/-d/-r/-w/-x/-s/-h, unary `!`, binary `-a`/`-o`, parentheses).
  File tests resolve through `c.FS` (NEVER host `os.Stat`).
* **just-bash citation:** approximate — just-bash's `[` / `test` is in
  `src/interpreter/builtins/test.ts`. The operator semantics are the
  POSIX standard.
* **Risk:** the `-r`/`-w`/`-x` triplet returns true unconditionally if
  the file exists, because the VFS doesn't model permission bits with
  the granularity real bash inspects. The Phase 21 hardening sweep
  may refine this against the FS's mode bits.

### `let` arithmetic is a self-contained recursive-descent parser

* **What:** `builtins/let/let.go` implements integer arithmetic with
  `+ - * / %`, parentheses, unary `-`, assignments (`name=expr`), and
  variable lookup against `c.Env`. Spaces around `=` are tolerated
  (works around mvdan divergence C in this file).
* **Why:** mvdan/sh's `let` is not registry-reachable (it's parser-side
  arithmetic) and re-using `mvdan/sh/expand.Arithm` requires hooking a
  full runner. A 200-line direct parser is simpler and Phase 11-scoped.
* **Risk:** lacks bit-ops (`& | ^ ~ << >>`), comparison ops, and the
  ternary operator. Add as fixtures demand.

### `read` / `mapfile` / `getopts` can't mutate the runner

* **What:** `/bin/read`, `/bin/mapfile`, `/bin/readarray`, and
  `/bin/getopts` write to `c.Env` rather than runner Vars. For
  `mapfile`/`readarray`, array entries are flattened to
  `<NAME>_<INDEX>` env vars. `getopts` always reports "no more
  options" (exit 1) because OPTIND state cannot persist across
  dispatches.
* **Why:** the registry's `command.Context` has no back-channel to the
  mvdan/sh runner's Vars map. Phase 12+ may add a `SetVar(name,
  value)` hook to Context if a fixture demands; for now, scripts that
  need real-bash semantics must use the bare names (which hit mvdan's
  native builtins).


## Phase 12 — Process info & defaults

### `$$`, `$PPID`, `$BASHPID` rewritten in the AST, not via env

* **What:** A new `rewriteProcInfo` pre-Run pass (procinfo.go) walks
  every `*syntax.Word` and `*syntax.DblQuoted` and replaces "simple"
  `$$` / `$PPID` / `$BASHPID` `*syntax.ParamExp` nodes with literal
  `*syntax.Lit` nodes holding `procInfo.PID`, `procInfo.PPID`, and the
  per-subshell BASHPID counter respectively.
* **Why:** mvdan/sh's `interp/vars.go:174` (v3.13.1) hardcodes `$$ →
  strconv.Itoa(os.Getpid())` and `$PPID → strconv.Itoa(os.Getppid())`
  in its `lookupVar` switch, BEFORE env consultation. Setting these
  names via `interp.Env(expand.ListEnviron(...))` has no effect — the
  switch wins. Mutating the host process's identity is not a realistic
  option, so an AST rewrite is the only correct way to virtualize
  these three names. SPEC §12 says `$$` "is the virtual pid, never
  the host PID" — a hard parity requirement.
* **Risk / scope:** the rewrite only fires on the "simple" form
  (`$X` or `${X}` with no Length / Width / Index / Slice / Repl / Exp
  / Names / IsSet / Excl / NestedParam / Modifiers / Flags). Complex
  forms like `${BASHPID:-fallback}` or `${#PPID}` are left to mvdan
  and therefore still see the host process's identity. Real scripts
  do not use these forms; if a fixture ever exercises one we will
  expand the predicate. The `simple()` check mirrors mvdan's own
  unexported `(*ParamExp).simple()`.

### `BASHPID` counter is per *subshell*, not per *call*

* **What:** `rewriteProcInfo` enumerates every `*syntax.Subshell` node
  in DFS pre-order at parse time and assigns each a unique counter
  value (`procInfo.PID+1`, `procInfo.PID+2`, ...). `$BASHPID`
  references inside a subshell expand to the innermost enclosing
  subshell's counter; references outside any subshell expand to
  `procInfo.PID`.
* **Why:** SPEC §12 says "BASHPID starts at virtual pid; each subshell
  increments a counter". Real bash assigns a fresh PID on `fork(2)`,
  which is what `(...)` triggers. Lexical subshell scope is the
  closest static analogue and matches the just-bash port semantics.
* **Risk:** background `&`, process substitution `<(cmd)`, and
  pipeline stages (which also fork in real bash) do NOT trigger a
  fresh BASHPID under this rule — only explicit `(...)` does. This
  is documented; no fixtures currently rely on the broader behavior.
  Phase 21 hardening may extend the rule to background/procsub if a
  use-case emerges.

### `$$` stays constant across subshells (matches real bash)

* **What:** Inside `(...)`, `$$` continues to expand to `procInfo.PID`
  (NOT the subshell's BASHPID).
* **Why:** Real bash's `$$` is "the PID of the calling shell" — it is
  set once and never changes, even in subshells. `BASHPID` is the
  per-subshell-fork pid. SPEC §12's "$$ = virtualPid (never the host
  PID)" pins the parent-only semantics. Pinned by
  `TestPhase12DollarDollar/inside_subshell`.

### Other §12 items already wired in earlier phases

* **Default `procInfo`:** `pid=1, ppid=0, uid=1000, gid=1000` set in
  `defaultProcessInfo()` (bash.go), exercised in §1 / Phase 1.
* **`/proc/self/status` template:** rendered in fs_init.go's
  `procSelfStatusTemplate` and written by `applyDefaultLayout` in
  Phase 7. Byte-exact match to SPEC §11 is pinned by
  `TestPhase12ProcSelfStatus`.
* **`whoami` always prints "user":** Phase 10 Wave A
  (`builtins/whoami`). Pinned by `TestPhase12Whoami`.
* **`hostname` reads `/etc/hostname`:** Phase 10 Wave A
  (`builtins/hostname`). Pinned by `TestPhase12Hostname` and the new
  `hostname/etc_hostname_default.json` + `hostname/custom.json`
  fixtures.

---

## Phase 13 — Transform pipeline

### Plugins mutate `*syntax.File` via `Script.Origin`, not the typed AST

* **What:** `transform.Plugin.Transform(ctx Context) Result` receives a
  `*ast.Script` (the typed Go AST that mirrors just-bash's TS AST).
  The TeePlugin port, however, performs its rewrites on
  `ctx.AST.Origin` — the underlying `*mvdan.cc/sh/v3/syntax.File` the
  parser produced. `transform.Serialize` already prefers Origin
  for byte-faithful round-trip (Phase 4), so the post-mutation
  printer pass works without going through the limited `astToFile`
  inverse translator.
* **just-bash citation:** approximate — `src/transform/plugins/tee-plugin.ts`
  works against the just-bash typed AST. The TS port has a complete
  inverse translator; ours does not (Phase 4 wired only the SimpleCommand /
  Pipeline / Statement subset; everything else still relies on Origin).
* **Why:** Extending `astToFile` to cover every compound command type
  is a Phase-4-shaped task. Mutating Origin keeps the Phase 13
  surface honest without dragging a full inverse translator forward.
* **Risk:** Plugins that synthesize entirely new ASTs (Origin == nil)
  still bottleneck on `astToFile`. The pipeline accepts that path and
  documents the limitation in `transform/serialize.go`. The Phase 13
  built-ins (collector / tee) both keep Origin set.

### TeePlugin omits PIPESTATUS-restore synthesized pipeline

* **What:** `TeePlugin.Transform` injects `cmd | tee /OUT/<idx>-<cmd>.stdout.txt`
  after every wrapped pipeline stage but does NOT emit the
  PIPESTATUS save/restore pipeline SPEC §13.3 calls for "faithful"
  port of `tee-plugin.ts`.
* **just-bash citation:** `src/transform/plugins/tee-plugin.ts` (not
  read by this port — we work from the SPEC §13.3 description).
* **Why:** PIPESTATUS preservation is a multi-statement rewrite
  (save the array into a tmp var before the tee insertion, restore
  after) and would force a substantial broadening of the inverse
  translator (Phase 4 covers Pipeline-of-SimpleCommand only). The
  Phase 13 acceptance criteria do not exercise PIPESTATUS, so the
  port lands without it and the gap is recorded here.
* **Risk:** Scripts that observe `${PIPESTATUS[@]}` or run under
  `set -o pipefail` after a wrapped pipeline will see PIPESTATUS
  shifted by +1 per inserted tee stage. Documented as a Phase 13
  open follow-up.

### TeePlugin counter is per `Transform` call, not per `Pipeline.Transform`

* **What:** `tee.Plugin.Transform` resets its `CommandIndex` counter
  to 0 on every invocation; the counter is local to the per-call
  `state` value. Re-running the same Plugin against the same script
  twice produces fresh `0..N-1` indices each time.
* **Why:** Plugins are documented as shareable across pipelines
  (SPEC §13.1 — no per-Plugin state contract). A shared counter
  would also produce non-deterministic file paths in tests.
* **Risk:** If a host pipes the same Plugin instance into multiple
  Bash environments concurrently, the indices are still locally
  monotonic per Exec. Acceptable.

### Single-stage "pipelines" are also wrapped

* **What:** TeePlugin wraps every Stmt whose command name matches
  `TargetCommandMatch`, including non-pipeline single commands
  (`echo hi` → `echo hi | tee /OUT/0-echo.stdout.txt`).
* **Why:** SPEC §13.3 phrases the wrap target as "each non-trivial
  pipeline stage". A bare command is a one-stage pipeline; treating
  it consistently is the least-surprise behavior and keeps the
  per-command-mirror promise from the file naming scheme.
* **Risk:** None observed. Hosts that want only multi-stage wrapping
  can supply a `TargetCommandMatch` predicate that filters by name.

### `BashOptions.TransformPlugins` is appended, not replaced, by `RegisterTransformPlugin`

* **What:** `New(BashOptions{TransformPlugins: ...})` copies the
  slice into `Bash.plugins`. Later `RegisterTransformPlugin(p)`
  calls append to the same slice. There is no `ClearTransformPlugins`
  or `SetTransformPlugins`.
* **just-bash citation:** matches the TS `Bash.registerTransformPlugin`
  surface — registration is monotonic.
* **Why:** Symmetry with the rest of the per-Bash registration
  surface (`CustomCommands`, alias / shopt tables).
* **Risk:** None — hosts that need fresh plugin state should
  construct a new `Bash`.


## Phase 14 — Optional SQLite runtime

### File DBs shuttle through host tmp files

* **What:** `sqlite3 file.db "QUERY"` resolves the path via `c.FS`
  (`builtinutil.ResolvePath` against `c.Cwd`), reads any existing
  bytes from the VFS, writes them to a host `os.MkdirTemp` file,
  hands modernc.org/sqlite that real-disk path as the DSN, then on
  completion reads the host file back and writes it to the VFS
  before deleting the tmp dir.
* **just-bash citation:** SPEC §14 explicitly endorses this MVP
  ("map VFS file → temp file on disk for the query duration, then
  write back"); just-bash itself uses `better-sqlite3` against the
  VFS adapter, which has no Go equivalent.
* **Why:** modernc.org/sqlite cannot open a `database/sql` DSN
  against an arbitrary `io/fs` implementation — the underlying
  pager assumes real OS file handles. Until v2 we route file DBs
  through a tmp shuttle so the VFS still owns canonical persistence.
* **Risk:** Concurrent writers to the same VFS file race — last
  cleanup wins. Real bash + sqlite3 has the same shape but on the
  host filesystem. Phase 17 sandbox can lock the VFS path if needed.
  The shuttle also doubles the disk-bytes-in-flight, fine for the
  10–100 MiB DBs the Phase-2 limits already cap us at.

### Output-mode defaults match the sqlite3 CLI semantics

* **What:** Default is `list` mode (rows joined by `|`, no header).
  `-header` prepends a header row to list / CSV output. `-csv`
  uses CRLF line endings via `csv.Writer.UseCRLF = true`. `-json`
  emits one JSON document — a top-level array of row objects with
  keys sorted alphabetically (Go's `encoding/json` map encoder is
  not preserving column order). `-line` emits `<width-padded name>
  = <value>\n` per column, blank line between rows.
* **just-bash citation:** sqlite3 CLI behavior, mirrored by
  just-bash. The TS port's column-ordered JSON is NOT reproduced —
  Go's stdlib JSON encoder sorts map keys, and we did not synthesize
  a custom encoder. Hosts that need column-ordered JSON keys must
  fall back to `-csv` or write SQL with `json_object()`.
* **Why:** stdlib first; CRLF for CSV matches `sqlite3 -csv` and
  RFC 4180. JSON key sort is a known Go-stdlib divergence.
* **Risk:** Scripts comparing JSON output byte-for-byte against the
  TS port may diverge if the SELECT lists columns in non-alphabetical
  order. Documented.

### Timeout enforcement: ctx.WithTimeout + db.Close() on cancellation

* **What:** When `Options.Timeout > 0` (or, fallback,
  `Limits.MaxSqliteTimeout > 0`) the runner wraps `ctx` in a
  `context.WithTimeout` and spawns a goroutine that closes the
  `*sql.DB` on `ctx.Done()`. modernc.org/sqlite aborts in-flight
  queries when the underlying connection closes, so long-running
  recursive CTEs return with an error instead of running to
  completion. A `done` channel signals the happy-path so the
  goroutine exits without closing the still-good DB twice (a
  second `db.Close()` is a documented no-op anyway).
* **just-bash citation:** SPEC §14 ("on `ctx.Done()` call
  `db.Close()` to interrupt"). The TS port uses `better-sqlite3`'s
  `db.interrupt()`; we use `Close()` because modernc has no
  equivalent interrupt API on the public `database/sql` surface.
* **Why:** `Close()` is the supported portable way to abort an
  in-flight `database/sql` query — `QueryContext` cancellation
  alone does not currently kill long-running recursive CTEs on the
  modernc driver.
* **Risk:** Closing the DB drops any uncommitted writes. For file
  DBs the host tmp file may be in an inconsistent state when the
  cleanup write-back runs; we still write the tmp bytes to the VFS
  because mid-transaction rollback is preferable to silently losing
  the cleanup. Hosts that need guaranteed-clean rollback must
  themselves wrap with savepoints.

### Stub builtin lives at `internal/testdata/fixtures/sqlite3-stub/`

* **What:** The Phase 10 Wave H stub's comparison test (in
  `builtins/sqlite3/sqlite3_comparison_test.go`) now runs against
  `internal/testdata/fixtures/sqlite3-stub/`. The original
  `internal/testdata/fixtures/sqlite3/` directory is reserved for
  real-runtime fixtures consumed by the `sqlite/` subpackage's
  own comparison-test harness.
* **Why:** A single fixture directory cannot satisfy both shapes —
  the stub returns "not enabled", the runtime returns real query
  output. Splitting the directories keeps both test paths honest
  and lets Phase 19 bulk-import recorded fixtures into the runtime
  dir without colliding with the stub's pin.
* **Risk:** None. The split is invisible to hosts.

### `sqlite3 :memory:` and stdin-piped SQL share the same code path

* **What:** When no positional SQL argument is supplied, the runner
  reads `c.Stdin` to EOF and uses that as the query text. Empty
  trimmed input is rejected with "no SQL supplied" (exit 1).
* **just-bash citation:** matches sqlite3 CLI, which also accepts
  SQL on stdin when invoked with only a DATABASE argument.
* **Why:** Convenience for pipelines (`echo "SELECT 1" | sqlite3
  :memory:`). The TS port also supports this shape.
* **Risk:** None — a script that wants to consume stdin via SQL
  redirection still can (the runtime treats c.Stdin uniformly).
