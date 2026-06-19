# go-bash Handoff — Phase 7 complete

- **Date:** 2026-06-17
- **Status:** COMPLETE
- **Branch / commit:** main @ (this commit)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). Combined coverage 58.9% (up from 58.7%). Root `gobash` package coverage 80.5% (same — the new `fs_init.go` is fully covered, but it was a small addition).

## What this phase delivered

- **`fs_init.go`** (NEW, root package) — three package-private symbols plus a private constant pair:
  - `defaultLayoutDirs` — the canonical SPEC §7 directory list (`/`, `/home`, `/home/user`, `/bin`, `/usr`, `/usr/bin`, `/tmp`, `/etc`, `/dev`, `/proc`, `/proc/self`) in MkdirAll-safe order.
  - `defaultHostname = "localhost\n"` and `procSelfStatusTemplate` — the literal `/etc/hostname` content and the `Name:\tbash\nPid:\t…\nPPid:\t…\nUid:\t…\t…\t…\t…\nGid:\t…\t…\t…\t…\n` format from SPEC §11 (referenced by §7). Tab-separated, four-column Uid/Gid to mirror real Linux's `/proc/self/status`.
  - `defaultBinStubs() []string` — Phase 7 returns nil. Designed as the Phase 8 plug-in point: Phase 8 swaps the body to enumerate the command registry, and every name returned gets materialized as a 0o755 stub at `/bin/<name>` with a `#!/bin/sh / exit 0` body. No call-site restructuring required.
  - `applyDefaultLayout(target gbfs.FileSystem, info ProcessInfo) error` — does all the MkdirAll + WriteFile calls in one pass. Returns the first FS error (callers may ignore it; New does).
  - `renderProcSelfStatus(p ProcessInfo) string` — package-private template wrapper exposed so tests can pin the exact byte sequence.
- **`bash.go::New`** — added a `useDefaultLayout := opts.Cwd == "" && len(opts.Files) == 0` gate. When true, calls `applyDefaultLayout(b.fs, b.procInfo)` (errors discarded — matches the existing "best effort" treatment of `b.fs.MkdirAll(b.cwd, …)` immediately below) and seeds `$HOME=/home/user` plus `$PATH=/usr/bin:/bin` into `b.env` only if the caller didn't already supply each key. The gate is checked AFTER FS construction + `seedFiles`, so the layout writes onto whatever FS we ended up with.
- **`fs_init_test.go`** (NEW, root package) — seven Phase 7 acceptance tests:
  - `TestPhase7DefaultLayoutPresent` — every `defaultLayoutPaths` directory exists; `/etc/hostname` is exactly `"localhost\n"`; `/proc/self/status` contains the five expected lines with the default ProcessInfo values (PID=1, PPID=0, UID=1000, GID=1000 per SPEC §1.2). Uses a test-local `defaultLayoutPaths` slice that mirrors SPEC §7 — divergence between the helper and the spec lights up here as a test failure, not as silent agreement.
  - `TestPhase7DefaultEnvHomeAndPath` — `echo "$HOME"; echo "$PATH"` through `Bash.Exec` yields `/home/user\n/usr/bin:/bin\n`.
  - `TestPhase7UserHomeOverridesDefault` — caller-supplied `Env["HOME"]` and `Env["PATH"]` reach the script unchanged (`/custom/home` / `/sbin`).
  - `TestPhase7ProcSelfStatusInterpolatesProcessInfo` — custom `ProcessInfo{PID:42, PPID:7, UID:501, GID:502}` renders into the file with all four Uid/Gid columns matching.
  - `TestPhase7CwdSuppressesDefaultLayout` — `BashOptions{Cwd:"/workspace"}` produces only `/workspace`; `/etc/hostname`, `/proc/self/status`, `/home/user` are absent; `$PATH` is empty.
  - `TestPhase7FilesSuppressDefaultLayout` — `BashOptions{Files:{…}}` produces only the seeded file; `/etc/hostname`, `/proc/self/status`, `/home/user`, `/bin` are absent.
  - `TestPhase7LayoutVisibleToScripts` — `read host < /etc/hostname; echo "$host"` echoes `localhost`. End-to-end VFS-through-Exec sanity.

## Acceptance criteria (from SPEC §7)

- [x] **Layout present after `New(BashOptions{})`** — `TestPhase7DefaultLayoutPresent` (11 dirs + 2 sentinel files).
- [x] **Layout NOT created when `Files` is provided** — `TestPhase7FilesSuppressDefaultLayout` (no `/etc/hostname`, no `/proc/self/status`, no `/home/user`, no `/bin`).
- [x] **Layout NOT created when `Cwd` is provided** — `TestPhase7CwdSuppressesDefaultLayout` (no `/etc/hostname`, no `/proc/self/status`, no `/home/user`; only the supplied `/workspace`).
- [x] **`/proc/self/status` interpolates ProcessInfo** — `TestPhase7ProcSelfStatusInterpolatesProcessInfo` (custom 42/7/501/502 values appear in all five fields including the four Uid/Gid columns).
- [x] **User-supplied `Env["HOME"]` overrides the default** — `TestPhase7UserHomeOverridesDefault` covers both HOME and PATH.
- [x] **Phases 1–6 stay green** — `make ci` PASS.

## Tests

- `github.com/mark3labs/go-bash` — **67 tests** (60 from Phases 1–6 + 7 Phase 7). Coverage 80.5% (unchanged — the new ~25 statements in `fs_init.go` are all covered by the new tests).
- All other packages unchanged from Phase 6.
- Combined coverage: **58.9%** (up from 58.7%).
- Comparison fixtures: N/A this phase. Harness lands in Phase 19.

## Decisions & gotchas discovered

- **mvdan/sh's runner auto-fills `$HOME` from the host user when env is empty.** Discovered when `TestPhase7CwdSuppressesDefaultLayout` initially asserted `HOME=\n` for a Cwd-only construction and got back the actual host home (`/home/space_cowboy`). The runner's `Reset()` path calls `user.Current()` and sets `HOME` if it isn't already in the env we passed via `interp.Env(...)`. `$PATH` is NOT auto-filled, so the suppression test now uses PATH as the canary instead. This means:
  - When the default layout fires, we explicitly seed `HOME=/home/user` into `b.env`, which propagates to the runner via the existing env slice path — the seed wins because mvdan/sh only fills when the env lookup returns empty.
  - When the default layout is suppressed (Cwd or Files supplied), `HOME` will be the *host* user's home unless the caller supplied one. This is an acknowledged host-leak — out of scope for this phase but worth pinning in a future hardening pass. The Phase 21 hardening sweep (no-os-exec lint et al.) should also force-seed a sentinel `HOME` to prevent the leak; logging here as a follow-up.
- **The `useDefaultLayout` gate is `Cwd == "" && len(Files) == 0`** — `FS` supplied alone does NOT suppress the layout. Rationale: the existing default-cwd code already created `/home/user` on any FS (including caller-supplied), so the user expectation is "FS is the *storage*; layout is the *content*". A caller who wants a totally empty FS supplies `FS = memfs.New()` plus a single sentinel via `Files{}` (any non-empty map suppresses) or an explicit `Cwd`. Documented inline at the `useDefaultLayout` assignment.
- **`applyDefaultLayout` errors are discarded by `New`.** Mirrors the existing pattern (`_ = b.fs.MkdirAll(b.cwd, 0o755)`) — a caller-supplied read-only or restricted FS must be allowed to gracefully reject some of the writes without failing construction. The helper itself returns the first error so a Phase-8-or-later caller that *wants* the strict shape can still get it.
- **The `/bin/X` stub list is empty until Phase 8.** `defaultBinStubs() []string` returns nil. When Phase 8 lands the registry, the body becomes `return registry.Names()` (or similar) and every name produces a 0o755 file at `/bin/<name>` with the `binStubBody` sentinel content. No call-site restructuring required — that's the whole point of the helper.
- **`/proc/self/status` uses the SPEC §11 template literally.** Tab-separated, with the four-column Uid/Gid format mirroring real Linux's `/proc/self/status` (real, effective, saved-set, filesystem). gobash's virtualized identity model has no concept of set-uid transitions, so all four columns get the same value. just-bash does the same; if a future test exercises a script that parses set-uid changes out of `/proc/self/status`, this is the line to revisit.
- **Default-layout creation order is safe under the Phase 6 runtime caps.** The layout writes 11 directories + 2 files (13 ops total), well under `MaxBraceExpansionResults` / `MaxArrayElements` / `MaxGlobOperations` defaults (1000 / 100000 / 100000 respectively). The Phase 6 handoff explicitly flagged this envelope as a constraint; the layout doesn't even touch a cap.
- **No public-surface changes.** Phase 7 is internal plumbing. The behavior is observable via `Bash.FS()` and via `$HOME`/`$PATH`/`/etc/hostname` reads inside scripts, all exercised in the new test file.

## Open follow-ups (non-blocking)

- **Phase 8 plugs the `/bin/X` stub list in via `defaultBinStubs`.** Replace `return nil` with the registry-derived name list. The helper signature, call site, and stub content (`binStubBody`) are all settled.
- **HOME host-leak when the default layout is suppressed.** A `Cwd`-or-`Files`-constructed Bash inherits the host user's HOME because mvdan/sh's runner auto-fills it from `user.Current()`. Phase 21 hardening should force-seed a sentinel HOME (e.g. `/`) when the caller doesn't supply one and the default layout is suppressed. Cheap to fix; deferring because it's outside the §7 acceptance bullets.
- **`/proc/self/status` is a snapshot at construction time.** A future phase that lets ProcessInfo mutate (e.g. background-job pid increments per SPEC §6.11 / handoff line 1561) will need to either re-render on read or accept the snapshot semantics. Phase 11's `proc/self/status`-reading builtins (`whoami`, `hostname`) just read the file, so the snapshot is fine through Phase 11; revisit if Phase 12+ needs live values.
- **The `/bin` stubs vs. lookup semantics.** Phase 8's dispatch must NOT exec the `binStubBody` contents on `/bin/cat`-style invocation — it must route through the registry. The stub file existence is purely for `which`, `command -v`, `[ -x ]`, and absolute-path-arg-matching. Document this loudly in the Phase 8 handoff.
- **Comparison fixtures (Phase 19) will retroactively exercise §7 too.** Many just-bash plain-shell fixtures read `/etc/hostname` or check `$HOME` paths; failures here will surface as Phase 7 regressions and should be patched in `fs_init.go`.

## NEXT PHASE: Phase 8 — Command Registry

- **Goal:** port `src/commands/registry.ts`. Land the `command` package with `command.Name`, `command.Command` interface, `command.Context`, `command.Result`, and the `Registry{Register, Lookup, Names}` type. Wire the `commandExecHandler` middleware to actually look up `args[0]` in the registry (custom > built-in), build a `CommandContext` from `interp.HandlerCtx(ctx)`, call `cmd.Execute`, translate the result into `interp.NewExitStatus`, and return `interp.ErrNotFound` for unregistered commands so the os/exec fall-through is closed. Plug `defaultBinStubs()` into `registry.Names()` so the §7 `/bin/X` stubs materialize.
- **Read:** SPEC.md §8 in full, this handoff for the `defaultBinStubs` swap point, `handoffs/phase-5.md` for the `commandExecHandler` middleware shape and the existing os/exec fall-through caveat, and `bash.go` + `interp/runner.go` for the call-site wiring.
- **Deliver:** new `command/` package, registry plumbing, the swap of `commandExecHandler` from pass-through to real dispatch, plug `defaultBinStubs` into the registry, and the `BashOptions.Commands` / `BashOptions.CustomCommands` fields from SPEC §1.2. Tests: at least one registry-Lookup test, one ErrNotFound regression confirming os/exec is no longer the chain terminator, and one `/bin/X` stub-creation test asserting the Phase 7 helper now produces files.
- **Acceptance (SPEC §8):** `Registry.Register/Lookup/Names` work; unregistered commands return `interp.ErrNotFound` (not host exec); `BashOptions.Commands` filtering works; `BashOptions.CustomCommands` override built-ins; `/bin/X` stubs are present for every registered command after `New(BashOptions{})`.
- **Don't:** implement any built-in command bodies (Phase 10) — Phase 8 is just the registry skeleton plus one minimal sample command (e.g. `echo` re-implemented to verify the dispatch chain) if needed for the ErrNotFound test.

- **Kickoff prompt for the Phase 8 session:**

  > Implement Phase 8 of go-bash per SPEC.md §8. Read AGENTS.md and HANDOFF.md first, then `handoffs/phase-7.md` for the `defaultBinStubs` swap point in `fs_init.go` (the helper signature, call site, and `binStubBody` are all settled — Phase 8 only needs to swap `return nil` for the registry-derived name list), `handoffs/phase-5.md` for the `commandExecHandler` middleware shape and the still-open os/exec fall-through caveat, and `bash.go` + `interp/runner.go` for the call-site wiring. Goal: land the `command` package with `Name`, `Command` interface, `Context`, `Result`, and `Registry{Register, Lookup, Names}`; wire the `commandExecHandler` middleware to dispatch through the registry (custom > built-in) and return `interp.ErrNotFound` for unregistered commands so the os/exec fall-through is closed; add `BashOptions.Commands` (filter) and `BashOptions.CustomCommands` (override) from SPEC §1.2; plug `defaultBinStubs()` into `registry.Names()` so the §7 `/bin/X` stubs materialize at construction time. Do NOT implement any built-in command bodies — Phase 10 owns those. The Phase 8 deliverable is the dispatch chain plus the registry skeleton, with at least one minimal sample command (e.g. an `echo` shim) if needed to write the ErrNotFound and dispatch-success regression tests. Run `make ci` until green, then call `finalize_phase` with `phase=8`, the drafted handoff markdown, the new root `HANDOFF.md` pointer, a conventional-commit subject like `feat(phase-8): command registry`, and the kickoff prompt for Phase 9 (network layer).
