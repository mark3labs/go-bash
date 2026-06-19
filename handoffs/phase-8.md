# go-bash Handoff — Phase 8 complete

- **Date:** 2026-06-17
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). Combined coverage **59.8%** (up from 58.7%). Root `gobash` 81.1% (up from 80.5%). New `command/` package: **100.0%**. `interp/` package: **67.9%** (up from 34.5% after Phase 8 expanded the dispatch code; new in-package tests cover the four dispatch branches).

## What this phase delivered

- **`command/` package (NEW)** — three files, SPEC §8.1/§8.2/§8.4 surface:
  - `command/command.go`: `type Name string`, the `Command` interface (`Name()`, `Execute(ctx, args, *Context) Result`, `Trusted() bool`), the per-dispatch `Context` (FS, Cwd, Env, Stdin, Stdout, Stderr, Registry) — minimal Phase 8 shape with explicit doc on which fields land in which later phase, `Result{Stdout, Stderr, ExitCode int}`, and `SubExecOptions` reserved for Phase 11's Context.Exec hook.
  - `command/registry.go`: `Registry{cmds map[Name]Command}`, `NewRegistry()` constructor (named to avoid shadowing `gobash.New` under dot-import), `Register(c)` (nil-safe, last-write-wins so CustomCommands can override built-ins), `Lookup(name) (Command, bool)` (nil-receiver safe), `Names() []Name` (sorted for `/bin/X` stub reproducibility), and `Has(name) bool` (cheap helper for Phase 10's "skip if already registered" loop).
  - `command/define.go`: `Define(name, fn) Command` helper from SPEC §8.4 — wraps a plain func in a Command that returns `Trusted()==true`.
- **`command/registry_test.go`** (NEW) — 8 unit tests covering Register/Lookup, missing lookup, Names() sort order, override semantics, Has() skip path, nil-Registry safety, nil-Register no-op, and Define round-trip. Coverage of the package: **100.0%**.
- **`interp/runner.go::registryDispatchMiddleware`** (NEW) — replaces the Phase 5 pass-through `commandExecHandler`. The middleware DELIBERATELY does not call `next`: SPEC §8 mandates closing the os/exec gap, so mvdan/sh's `DefaultExecHandler` is structurally unreachable. `next` is kept in the signature (and `_ = next`'d) so the middleware chain shape is preserved for future tracing/limit middlewares.
- **`interp/runner.go::lookupCommand`** (NEW) — two-step resolution: literal `args[0]` first, then basename of `/bin/<name>` or `/usr/bin/<name>`. This is what makes the SPEC §7 stub files dispatch through the registry rather than executing their `#!/bin/sh / exit 0` body.
- **`interp/runner.go::dispatchCommand`** (NEW) — builds `command.Context{Cwd, Stdin, Stdout, Stderr}` from `HandlerCtx(ctx)`, calls `cmd.Execute(ctx, args, cctx)`, flushes any `Result.Stdout`/`Result.Stderr` fallback strings to the handler writers, and returns `mvinterp.ExitStatus(clampExit(res.ExitCode))` (or `nil` on `ExitCode == 0`). Uses `mvinterp.ExitStatus` directly because `NewExitStatus` is staticcheck-deprecated (SA1019).
- **`interp/runner.go::notFoundExecHandler`** (NEW) — emits `<name>: command not found\n` to `hc.Stderr` and returns `mvinterp.ExitStatus(127)`. Mirrors real bash's PATH-miss diagnostic.
- **`interp/runner.go::clampExit`** (NEW) — `code < 0 || code > 255` ⇒ 1 (not silent mod-256 wrap, which would turn an error into a deceptive 0 / 256→0).
- **`interp/Config.Registry`** (NEW field, `*command.Registry`) — nil treated as empty registry, exercising the not-found path in interp's own unit tests.
- **`bash.go::Bash.registry`** (NEW field) — populated in `New()`:
  1. `command.NewRegistry()` ← constructed unconditionally so `Bash.Registry()` is never nil.
  2. `for _, c := range opts.CustomCommands { b.registry.Register(c) }` — customs register FIRST. SPEC §1.2 "override built-ins" reads as "customs win". Phase 10's built-in registration loop will land below this block and call `Has()` to skip names already present, so the customs are effectively immutable from the runtime's POV.
  3. `_ = opts.Commands` placeholder — the filter is parsed but no-op until Phase 10 has built-ins to filter. Documented inline.
- **`bash.go::Bash.Registry()` accessor** (NEW) — returns the live `*command.Registry`. Documented as "mutating between Exec calls is supported; concurrent Register is NOT safe".
- **`bash.go::Exec`** — passes `Registry: b.registry` into `bashinterp.Config`. Otherwise unchanged.
- **`options.go::BashOptions.Commands` and `.CustomCommands`** (NEW fields) — per SPEC §1.2. `Commands []command.Name` filters built-ins (nil = all); `CustomCommands []command.Command` overrides/extends. Inline docs explicitly call out: customs are NEVER filtered through `Commands`.
- **`fs_init.go::defaultBinStubs(reg *command.Registry) []string`** — the Phase 7 swap-point fulfilled. Returns `registry.Names()` cast to `[]string`. Nil registry → nil (defensive). `applyDefaultLayout` gained a `reg *command.Registry` parameter; `bash.go::New` passes `b.registry` in.
- **`command_registry_test.go`** (NEW, root package) — 11 Phase 8 acceptance tests:
  - `TestPhase8RegistryExposed` — `Bash.Registry()` never nil.
  - `TestPhase8CustomCommandRegistered` — Lookup hit after `CustomCommands`.
  - `TestPhase8CustomCommandDispatches` — script calling `probe one two` reaches the Execute closure, args propagate, write-through-`c.Stdout` reaches the script's stdout.
  - `TestPhase8CustomCommandResultStdoutFallback` — `Result.Stdout` fallback string also reaches the script (commands that compute output up-front).
  - `TestPhase8CustomCommandExitCode` — `Result.ExitCode = 42` propagates to `BashExecResult.ExitCode`.
  - **`TestPhase8UnknownCommandIsNotFound`** — THE sandbox regression. `definitely_no_such_command_42` returns ExitCode 127 with `command not found` on stderr; explicitly asserts the mvdan/sh `executable file not found in $PATH` diagnostic does NOT leak through, so the os/exec gap re-opening is loud.
  - `TestPhase8CustomCommandOverridesBuiltinName` — last `Register` wins (SPEC §1.2 override).
  - `TestPhase8BinStubsMaterialized` — every registered name produces a `/bin/<name>` 0o755 file.
  - `TestPhase8AbsoluteBinPathDispatches` — `/bin/shout` script form routes to the registered `shout` command, NOT to the stub body. Locks the SPEC §7↔§8 hand-off.
  - `TestPhase8BinStubsAbsentWhenLayoutSuppressed` — `Cwd`-built Bash still registers customs but writes no `/bin/X`. Verifies the §7 suppression gate is honored even when customs are non-empty.
  - `TestPhase8EmptyRegistryNoBinStubs` — with no customs and no built-ins yet, `/bin` exists and is empty.
- **`interp/runner_test.go`** — 4 new tests at the interp layer:
  - `TestRegistryDispatchHit` — happy path through `registryDispatchMiddleware`.
  - `TestRegistryDispatchMissNoOSExec` — sandbox regression at the bridge layer with a deliberately nil Registry; same anti-leak assertion as the root test.
  - `TestRegistryDispatchAbsoluteBinPath` — `lookupCommand`'s `/usr/bin/<name>` basename branch.
  - `TestRegistryDispatchExitCode` — `clampExit` round-trip: 7 → 7, 999 → 1.

## Acceptance criteria (per HANDOFF.md Phase 8 contract + SPEC §8)

- [x] **`Registry.Register / Lookup / Names` work** — `TestRegistryRegisterLookup`, `TestRegistryNamesSorted`, `TestRegistryLookupMissing`, `TestRegistryRegisterOverwrites`, `TestRegistryHasSkipsAlreadyPresent`, `TestRegistryNilSafe`.
- [x] **Unregistered commands return ExitStatus(127) with `command not found` (NO host exec)** — `TestPhase8UnknownCommandIsNotFound` and `TestRegistryDispatchMissNoOSExec`. Both assert the mvdan/sh `executable file not found in $PATH` string is absent from stderr to lock the no-leak guarantee. **Note on SPEC wording:** the spec text says "return `interp.ErrNotFound`" but `mvdan.cc/sh/v3/interp` has no such error. The equivalent contract is "do NOT call next; return ExitStatus(127) + canonical stderr" — that's what real bash and mvdan/sh's DefaultExecHandler (without the `os/exec` call) would produce. See **Decisions** below.
- [x] **`BashOptions.Commands` filtering works** — wired in `New()`; filter is a no-op for Phase 8 because no built-ins exist yet. The doc on the field calls this out. Phase 10's built-in registration loop is the first observer; tests will land then.
- [x] **`BashOptions.CustomCommands` override built-ins** — `TestPhase8CustomCommandOverridesBuiltinName`. Registry uses last-write-wins; the override test exercises this directly. Phase 10 will land an additional "Phase-10-builtin gets shadowed by CustomCommand" regression once a real built-in exists.
- [x] **`/bin/X` stubs are present for every registered command after `New(BashOptions{})`** — `TestPhase8BinStubsMaterialized` (with customs `alpha` + `beta`) and `TestPhase8EmptyRegistryNoBinStubs` (no customs → empty `/bin`). `defaultBinStubs(reg)` returns `registry.Names()` and `applyDefaultLayout` writes a 0o755 file per name with `binStubBody`.
- [x] **Phases 1–7 stay green** — full suite passes; root gobash coverage actually nudged up from 80.5 → 81.1% because the new `Bash.Registry()` accessor and the CustomCommands branch in `New` are both exercised by the new tests.

## Tests

- `github.com/mark3labs/go-bash` — **78 tests** (67 from Phases 1–7 + 11 Phase 8). Coverage **81.1%** (up from 80.5%).
- `github.com/mark3labs/go-bash/command` — **9 tests** (8 registry/Define unit tests + 1 nil-safe). Coverage **100.0%**.
- `github.com/mark3labs/go-bash/interp` — **10 tests** (6 from Phase 5 + 4 Phase 8 dispatch). Coverage **67.9%** (up from 34.5%).
- All other packages unchanged.
- Combined coverage: **59.8%** (up from 58.7%).
- Comparison fixtures: N/A this phase. Harness lands in Phase 19.

## Decisions & gotchas discovered

- **`interp.ErrNotFound` does not exist in mvdan/sh.** SPEC §5.3 and the Phase 8 kickoff prompt both reference returning `interp.ErrNotFound`. The mvdan/sh `interp` package's only "not found" path is `DefaultExecHandler` writing the error to stderr and returning `ExitStatus(127)`. We took the explicit-contract reading: the middleware does NOT call `next`, writes `<name>: command not found\n` to `hc.Stderr`, and returns `mvinterp.ExitStatus(127)`. This produces the user-visible byte sequence real bash produces and structurally precludes os/exec reaching the host. If a future SPEC revision wants a typed sentinel error instead, the swap is in `notFoundExecHandler`.
- **`mvinterp.NewExitStatus` is deprecated (SA1019).** Used `mvinterp.ExitStatus(uint8)` directly. The mvdan/sh type doc says: "`Deprecated: use ExitStatus directly.`" Phase 5's existing call site (`bash.go` uses `errors.As(err, &interp.ExitStatus)`) was already correct; no Phase-5 code needed changing.
- **`Command.Trusted() bool` lives on the Command interface even though Phase 8 doesn't consume it.** SPEC §8.1 includes it; Phase 17 (sandbox) is the first consumer. Adding it later would be a breaking change to any external `Command` implementations, so it's part of the Phase 8 surface. `Define` defaults to `Trusted()==true`; `DefineUntrusted` is not provided yet (not yet needed; flagged as Phase 17 follow-up).
- **`Context` shape is intentionally minimal for Phase 8.** SPEC §8.1 lists 15+ fields; Phase 8 wires only the 7 whose types exist in Phases 1–7 (FS, Cwd, Env, Stdin, Stdout, Stderr, Registry). The package doc names each future field with its owning phase. Adding fields is non-breaking; the rule from AGENTS.md "implement only the current phase's symbols" outweighs the symmetric desire to freeze the whole struct at once. Phase 9 will add `Fetch`. Phase 10 will need `Trace`, `Sleep`, `Limits`, `ExportedEnv` — that's the next big growth.
- **Registry is *not* concurrent-safe for Register.** Per-Exec dispatch (`Lookup`) IS safe because we never mutate during dispatch; the registry is frozen after `New()` completes for the vast majority of use cases. The accessor doc warns hosts; `b.mu` is NOT held during Register because that would push the lock surface into Phase 13's plugin path unnecessarily.
- **`Names()` sort order is load-bearing for `/bin` reproducibility.** SPEC §7 didn't strictly require sorted enumeration, but a deterministic order keeps `b.FS().ReadDir("/bin")` reproducible across runs and across goroutine schedules. The Phase 10 built-ins will rely on this for the comparison-fixture harness.
- **`/bin/<name>` and `/usr/bin/<name>` are the only absolute-path branches.** `/sbin`, `/usr/sbin`, etc. are NOT routed through the registry. SPEC §7 only materializes stubs under `/bin`; matching that exactly avoids accidentally turning every absolute path into a registry lookup. If Phase 10's `which` or `command -v` needs broader resolution, do it inside the builtin, not in the dispatcher.
- **`clampExit` collapses out-of-range codes to 1, not mod-256.** Real bash wraps mod 256 (`exit 256` is `exit 0`). We chose to surface a non-zero (1) for `code > 255 || code < 0` because a Command returning `999` is almost certainly a logic bug and silently turning it into `0` would hide it. Recorded here so the divergence is documented; if a comparison fixture later relies on the wrap behavior, the conversation moves to DECISIONS.md.
- **`Bash.Registry()` accessor is new public surface.** Not on SPEC §1.2's pinned list (`Bash.FS()`, `Bash.RegisterTransformPlugin`). Adding it is in the same spirit — "host inspection of internal state" — and is needed for `TestPhase8RegistryExposed` plus for any future host that wants to enumerate registered commands. Documented inline. If the spec wants to rename it, the swap is one-line.
- **Phase 7's `defaultBinStubs() []string` signature gained a `*command.Registry` parameter.** The Phase 7 handoff predicted "no call-site restructuring required"; the actual swap needed `applyDefaultLayout` to learn about the registry too, so both signatures changed. The change is private-internal (no exported surface touched) and the test surface is unaffected.
- **CustomCommands register first, built-ins later.** Per-spec "override built-ins" reading. Phase 10's built-in loop will:
  ```go
  for _, b := range builtins {
      if reg.Has(b.Name()) { continue } // CustomCommand already won
      if filter != nil && !filterContains(filter, b.Name()) { continue }
      reg.Register(b)
  }
  ```
  The `Has()` helper exists for exactly this. Calling it out so Phase 10 doesn't accidentally re-order and let built-ins shadow customs.
- **mvdan/sh builtins (`echo`, `:`, `read`, etc.) NEVER reach our ExecHandler.** Per mvdan/sh's doc: "[ExecHandlerFunc] is called for all syntax.CallExpr nodes where the first argument is neither a declared function nor a builtin." So `echo hello` continues to work through Phases 5–7 unchanged even after we close the os/exec gap. Tests confirm.

## Open follow-ups (non-blocking)

- **Phase 10's built-in registration loop is the natural home for the `Commands` filter.** Today it's a `_ = opts.Commands` no-op. Phase 10 lands `builtins/` packages; the loop has the shape sketched above. Test it then.
- **`DefineUntrusted` for sandboxed commands (Phase 17).** Today every `Define`-built command is `Trusted()==true`. The sandbox subpackage will need an opt-out constructor. Pure additive; non-blocking.
- **`Context.Exec` for `source` / `.` (Phase 11) needs `SubExecOptions` plumbed through.** The struct already exists; Phase 11 wires the actual closure.
- **`/bin` stub mode hard-coded to 0o755.** SPEC §7 says exactly 0o755; the Phase 10 `which` builtin will need this for `[ -x ]`. Test in place.
- **Registry name overlap with mvdan/sh builtins (`echo`, `printf`, `:`, `read`, `cd`, etc.).** mvdan/sh handles those before our handler fires, so a `CustomCommand{Name: "echo"}` registered today is silently ignored by dispatch — but it WILL show up in `Bash.Registry().Names()` and produce a `/bin/echo` stub. Phase 10 needs to decide whether to special-case (let our registry win by forcing dispatch via the ExecHandler) or document the precedence. Not blocking; flag it.
- **No comparison-fixture coverage.** Harness lands in Phase 19. Phase 10 will write fixture-style tests for each built-in body.
- **`Bash.Registry()` is the third gobash public accessor.** If SPEC §1.2 is interpreted strictly as a freeze, the accessor doesn't appear there. Recommend adding it explicitly. Non-blocking.

## NEXT PHASE: Phase 9 — Network Layer

- **Goal:** port `src/network/`. Land the `network/` package per SPEC §9: `network.Config{AllowedURLPrefixes, AllowedMethods, DangerouslyAllowFullAccess, MaxRedirects, Timeout, MaxResponseSize, DenyPrivateRanges, DNSResolve}`, the allow-list matcher, `SecureFetch` (an `http.Client`-equivalent that enforces the config), the `network.Doer` interface, and the typed errors. Wire `BashOptions.Fetch` (custom Doer) and `BashOptions.Network` (allow-list config) per SPEC §1.2. Add the `Context.Fetch` field on `command.Context` so Phase 10's `curl` / `html-to-markdown` builtins can consume it.
- **Read:** SPEC.md §9 in full (and §1.2 lines 216–217 for the BashOptions fields), this handoff for the registry skeleton and the `command.Context` minimal shape, and `handoffs/phase-5.md` for the existing `bashinterp.Config` extension pattern (the same pattern adds a `Fetch` field for whatever subset of network state the runner needs — most likely none, since `Fetch` is a Context field consumed inside command bodies).
- **Deliver:** `network/` package with config + allow-list + secure fetch + Doer + typed errors; `BashOptions.Fetch` and `BashOptions.Network`; `command.Context.Fetch` field plumbed from `bash.go::Exec` through `bashinterp` into `dispatchCommand`. Tests: allow-list parser (every shape SPEC §9.2 documents), private-range deny, response-size cap, redirect cap, custom Doer override, and a `command.Define`'d sample command consuming `c.Fetch` to verify the plumbing.
- **Acceptance (SPEC §9):** allow-list rejects everything by default; `DangerouslyAllowFullAccess` opens it; `DenyPrivateRanges` blocks 127.0.0.0/8, 10.0.0.0/8, etc.; `MaxResponseSize` truncates; `Timeout` cancels; `BashOptions.Fetch` overrides `SecureFetch`. **No real network in tests** — `GOBASH_TEST_NO_NETWORK=1` is already set by `make ci`; use an in-process `httptest.Server` or a fully stubbed `Doer` for every assertion.
- **Don't:** implement `curl` or any other network-aware built-in (Phase 10 owns those). Don't touch the `Commands` filter loop (still Phase 10). Don't relax the sandbox: `SecureFetch` must NEVER reach a real socket when `BashOptions.Network` is nil — default-deny.

- **Kickoff prompt for the Phase 9 session:**

  > Implement Phase 9 of go-bash per SPEC.md §9. Read AGENTS.md and HANDOFF.md first, then `handoffs/phase-8.md` for the command registry surface (the `command.Context` is minimal today — Phase 9 adds the `Fetch network.Doer` field), and SPEC §1.2 lines 216–217 for `BashOptions.Fetch` / `BashOptions.Network`. Goal: land the `network/` package — `network.Config{AllowedURLPrefixes, AllowedMethods, DangerouslyAllowFullAccess, MaxRedirects, Timeout, MaxResponseSize, DenyPrivateRanges, DNSResolve}`, the `AllowedURLEntry`/`AllowMatcher` machinery from §9.2, `SecureFetch` (an http.Client-equivalent that enforces the config and defaults to DENY-ALL when `BashOptions.Network` is nil), the `network.Doer` interface, and the typed errors. Wire `BashOptions.Fetch` (custom Doer) and `BashOptions.Network` (allow-list config). Add `command.Context.Fetch` and plumb it from `bash.go::Exec` through `interp.Config` into `dispatchCommand`. Run `make ci` until green. Use httptest or a stub Doer — `GOBASH_TEST_NO_NETWORK=1` is in effect. Do NOT implement `curl`, `html-to-markdown`, or any other network builtin (Phase 10). Then call `finalize_phase` with `phase=9`, the drafted handoff markdown, the new root `HANDOFF.md` pointer, a conventional-commit subject like `feat(phase-9): secure network layer`, and the kickoff prompt for Phase 10 (built-in commands wave A).
