# Phase Handoffs

One file per completed phase, written by the `finalize_phase` tool exposed by
the `.kit/extensions/handoff.go` extension. Each captures what was delivered,
the verified gate result, decisions/gotchas discovered, and the kickoff prompt
for the next phase.

## Workflow (automatic)

At the end of each phase, in the running kit session that did the work, the
agent calls the **`finalize_phase` tool** with:

| field              | purpose                                                                         |
|--------------------|---------------------------------------------------------------------------------|
| `phase`            | integer phase number just completed (e.g. `3`).                                 |
| `status`           | `"complete"` (default) or `"blocked"`.                                          |
| `handoff_markdown` | full markdown body of `handoffs/phase-<N>.md` (template below).                 |
| `pointer_markdown` | full markdown body of the root `HANDOFF.md` pointer (status + kickoff prompt).  |
| `commit_subject`   | conventional-commit subject, e.g. `feat(phase-3): in-memory filesystem`.        |
| `commit_body`      | optional extended description.                                                  |
| `kickoff_prompt`   | first user message for the new session; required when `status="complete"`.      |
| `skip_gate`        | optional; default `false`. Skip `make ci` (only with very good reason).         |
| `tag`              | optional; default `false`. Also create lightweight tag `phase-<N>`.             |

The tool:

1. Refuses unless invoked from the go-bash repo root (`SPEC.md`, `handoffs/`, `.git/`).
2. Runs `make ci` (unless `status="blocked"` or `skip_gate=true`). On failure it
   returns the tail of the gate output as the tool result and writes nothing —
   the agent fixes the issues and calls the tool again.
3. Writes `handoffs/phase-<N>.md` from `handoff_markdown`.
4. Overwrites the root `HANDOFF.md` pointer from `pointer_markdown`.
5. Runs `git add -A` and commits with `commit_subject` (and `commit_body`).
6. On success it stages the `kickoff_prompt`. When the agent's current turn
   ends, the extension's `OnAgentEnd` handler calls `ctx.NewSession(prompt)`
   (the kit v0.79.0 extension API), starting a fresh session pre-loaded with
   that first user message.

## Workflow (manual fallback)

The same extension also registers a `/handoff [N]` slash command. Typing it
injects a directive into the chat asking the agent to draft the handoff
markdown and call `finalize_phase` for the given phase. Use this when you want
to nudge the agent to close out a phase manually.

## Handoff markdown template

When composing the `handoff_markdown` argument, follow this structure exactly:

```markdown
# go-bash Handoff — Phase <N> complete

- **Date:** <ISO date>
- **Status:** COMPLETE | BLOCKED
- **Branch / commit:** <branch> @ <short-sha>
- **Gate:** make ci → PASS | FAIL (details below)

## What this phase delivered
- <bullet list of files created/changed and the public symbols implemented,
  each traced to its SPEC.md section, e.g. `limits.go` → ExecutionLimits, ResolveLimits (SPEC Phase 2.1)>

## Acceptance criteria (from SPEC.md Phase <N>)
- [x] <criterion from the phase's "Acceptance for Phase N" block> — <how it was satisfied>
- [x] ...

## Tests
- <packages touched, test count, coverage %>
- Comparison fixtures: <how many passed / N total in this phase's wave>

## Decisions & gotchas discovered
- <anything the spec was ambiguous about and how you resolved it; any
  divergence from just-bash behavior that you confirmed against the TS source
  before accepting>
- <if you deviated from a frozen name, dependency choice, or limit value:
  FLAG IT LOUDLY — it must be reconciled in SPEC.md before proceeding>

## Open follow-ups (non-blocking)
- <tech debt, TODOs, things intentionally deferred to a later phase>

## NEXT PHASE: Phase <N+1> — <name>
- **Goal:** <one line from SPEC.md>
- **Spec to read:** SPEC.md (Phase <N+1>), plus referenced just-bash source paths
- **Packages/files to create:** <list>
- **Public symbols to deliver:** <names from SPEC §<N+1>>
- **Prerequisites:** <met / the following must be built first: ...>
- **Kickoff prompt for the next session:**
  > <one-paragraph kickoff — same string also passed as kickoff_prompt>
```

The root `HANDOFF.md` (`pointer_markdown`) is much shorter: a status line, a
one-line summary of what's done, a link to `handoffs/phase-<N>.md`, and the
next-phase kickoff prompt. The next agent session reads it first.

## Phase map (from SPEC.md)

| Phase | Topic                              | File             |
|-------|------------------------------------|------------------|
| 0     | Ground rules / scaffold            | —                |
| 1     | Skeleton & public API surface      | `phase-1.md`     |
| 2     | Execution limits                   | `phase-2.md`     |
| 3     | Filesystem layer (4 FSes)          | `phase-3.md`     |
| 4     | Parser & AST                       | `phase-4.md`     |
| 5     | Interpreter bridge                 | `phase-5.md`     |
| 6     | Expansion & shell features         | `phase-6.md`     |
| 7     | Filesystem init & default layout   | `phase-7.md`     |
| 8     | Command registry                   | `phase-8.md`     |
| 9     | Network layer                      | `phase-9.md`     |
| 10    | Built-in commands (waves A–H)      | `phase-10a.md`…  |
| 11    | Interpreter built-ins              | `phase-11.md`    |
| 12    | Process info & defaults            | `phase-12.md`    |
| 13    | Transform API                      | `phase-13.md`    |
| 14    | SQLite (optional)                  | `phase-14.md`    |
| 15    | JavaScript (optional)              | `phase-15.md`    |
| 16    | Python hook (optional)             | `phase-16.md`    |
| 17    | Sandbox API                        | `phase-17.md`    |
| 18    | CLI                                | `phase-18.md`    |
| 19    | Comparison test harness            | `phase-19.md`    |
| 20    | Documentation                      | `phase-20.md`    |
| 21    | Hardening & polish                 | `phase-21.md`    |

Phase 10 is the largest and is expected to span multiple sessions (one per
wave). Use suffixed phase numbers for the handoff files
(`phase-10a.md`, `phase-10b.md`, …) but pass an integer `phase` to the tool
(e.g. `10`) — the tool simply writes `phase-<N>.md`, so the agent can override
the path manually when splitting a phase.

## Index

| Phase | File | Status |
|-------|------|--------|
| 0 (scaffold) | — | complete (see root HANDOFF.md) |
