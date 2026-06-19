# BUILDING.md — Working Charter for go-bash Implementation

> **For end-users and AI-agent integrators:** see `AGENTS.md` for the
> user-facing guide. This file is the build-time coordination doc for
> contributors working on the go-bash internals.

You are implementing **go-bash**, a feature-for-feature Go port of the
**just-bash** TypeScript bash interpreter (`vercel-labs/just-bash`). The
complete build specification is in `SPEC.md` at the repository root.

**This repo already contains the complete build spec.** Your job is to turn
the spec into working, tested Go — phase by phase. Do not redesign;
implement.

## Read first (in order)

1. `HANDOFF.md` (repo root) — the START-HERE pointer. It always tells you what
   phase you're on and the exact kickoff prompt you were just handed.
2. The latest `handoffs/phase-<N>.md` — decisions, gotchas, and follow-ups
   from the previous session.
3. `SPEC.md` — the build spec. Find your phase, read its acceptance criteria,
   and read the resolved-decisions section at the bottom.
4. The `just-bash` source paths the spec cites for your phase. Treat those as
   ground truth for behavior; cite the file:line you ported from in
   `handoffs/phase-<N>.md`.

## Pinned versions

- Module path: `github.com/mark3labs/go-bash` (pinned in `go.mod` from
  Phase 1 onward)
- Go: **1.22+** (use the latest available toolchain; the spec lists 1.22 as
  the minimum)
- Bash engine: `mvdan.cc/sh/v3` (latest tagged)
- Other vendor choices are listed in `SPEC.md §0.2`. **Do not substitute
  silently** — record any swap in `DECISIONS.md`.

## How to work a phase (TDD, one phase per commit)

1. Read SPEC.md for the phase you're on. Re-read the resolved-decisions
   section at the bottom of the spec before starting.
2. Write the tests from the phase's acceptance criteria **first**. For phases
   that touch built-in commands, this means wiring the comparison-fixture
   loader before writing the command.
3. Implement until `go test -race ./<pkg>/...` is green.
4. Run the full gate locally:
   ```
   make ci
   ```
   (`make ci` = `tidy` + `build` + `vet` + `test-race` + `lint`. See the
   `Makefile`.)
5. A phase is done when its acceptance criteria are satisfied AND `make ci`
   passes.
6. **Close the phase by calling the `finalize_phase` tool.** The
   `.kit/extensions/handoff.go` extension (auto-loaded by kit) exposes it.
   Compose the full `handoffs/phase-<N>.md` body and the new root
   `HANDOFF.md` pointer, then invoke `finalize_phase` with `phase`,
   `handoff_markdown`, `pointer_markdown`, `commit_subject`, optional
   `commit_body`, and the `kickoff_prompt` for the next session. The tool
   runs `make ci`, writes both files, commits, and (via kit v0.79.0's
   `ctx.NewSession` extension API) automatically starts a fresh session with
   `kickoff_prompt` as the first user message. Keep the assistant's final
   message short — the new session does not see it. If `make ci` fails, the
   tool returns the failing output and writes nothing; fix and retry.

   Human users can still type `/handoff [N]` — that slash command (also
   from the same extension) injects a directive into the chat asking the
   agent to invoke `finalize_phase`. See `handoffs/README.md` for the
   canonical workflow.

## Non-negotiable rules

- **Feature-for-feature parity with just-bash.** When TS and Go semantics
  could diverge (e.g. encoding, regex, timezone), the TS source wins. Cite
  the just-bash file:line you ported from. Resolved decisions at the bottom
  of `SPEC.md` are law.
- **`context.Context` first.** Every public function and every command
  implementation takes `ctx` as its first parameter. `AbortSignal` → `ctx`.
- **`io.Reader` / `io.Writer` for stdin/stdout/stderr.** Convert to/from
  `string` only at the public `Exec` boundary when no writer is supplied.
- **Typed errors.** Port each TS error class as a Go error type; consumers
  use `errors.As`. See `SPEC.md §2.2`.
- **No `os/exec` in the runtime.** Forbidden by lint
  (`internal/lint/no-os-exec`). Only `cmd/record-fixtures` and the optional
  `pythonexec/exec` runtime may import it.
- **No real network in tests.** Set `GOBASH_TEST_NO_NETWORK=1`. The
  `make ci` target sets it automatically; the handoff tool also sets it.
- **No scope creep.** Implement only the current phase's symbols. Do not
  pre-build later phases. If you discover that a Phase 5 symbol is needed in
  Phase 4 because the spec ordering is wrong, FLAG IT in the handoff
  decisions section.
- **Cgo policy.** Main package and `jsexec` must build with
  `CGO_ENABLED=0`. The `Makefile` `build` target enforces this.

## Reference sources (read-only, do NOT import)

- The original just-bash TypeScript source. Read it via the GitHub repository
  `vercel-labs/just-bash` at the commit pinned by the spec. The spec cites
  exact file:line references for every behavior you must replicate. If you
  vendor it locally, put it under `reference/just-bash/` (gitignored).

## Build order (DAG — respect dependencies)

```
options/limits/errors  →  fs (memfs, overlayfs, rwfs, mountfs)  →  ast/parser
                                                                        ↓
        interp bridge  →  fs init  →  command registry  →  network
                                                                        ↓
                  built-in commands (waves A–H)  →  interpreter built-ins
                                                                        ↓
       transform API  →  optional runtimes (sqlite, js, python)  →  sandbox
                                                                        ↓
                          CLI  →  comparison harness  →  docs  →  hardening
```

Start with **Phase 1 (skeleton + public API surface + stub Exec)** — zero
external behavior, fully self-contained, the safest place to validate the
spec→code loop.

## Scaffold state

- The repo currently contains only this file, `SPEC.md`, the `Makefile`,
  the handoff extension, and `handoffs/README.md`. No Go code exists yet.
- Phase 1 will create `go.mod` (module path: pick your org), `doc.go`,
  `bash.go`, `options.go`, `result.go`, `errors.go`, `limits.go`, and a
  stub `Exec` backed by `mvdan.cc/sh/v3`.
- The first `make ci` you run will be at the END of Phase 1. Do not try to
  run it on the empty scaffold.
