//go:build ignore

// go-bash phase-handoff extension.
//
// Drives the phase-by-phase implementation of go-bash (the feature-for-feature
// Go port of vercel-labs/just-bash described in SPEC.md). When the agent
// finishes a phase it invokes the `finalize_phase` tool, which deterministically:
//
//  1. Runs `make ci` (unless status="blocked" or skip_gate=true). If the gate
//     fails the tool returns the failing output as the tool result so the
//     agent can fix and retry — no handoff file or commit is produced.
//  2. Writes handoffs/phase-<N>.md from the markdown the agent supplied.
//  3. Overwrites the root HANDOFF.md pointer with the markdown the agent
//     supplied.
//  4. Stages everything and commits with the supplied subject/body, returning
//     the short SHA.
//  5. Stashes the kickoff prompt for the next phase. After the current agent
//     turn ends, OnAgentEnd consumes the stash and calls ctx.NewSession to
//     start a fresh session pre-loaded with that prompt — exercising the new
//     /new <prompt> path in kit v0.79.0.
//
// The /handoff [N] slash command is preserved as a manual trigger: it injects
// a directive into the chat asking the agent to call finalize_phase for the
// given phase.
//
// Usage:
//
//	kit                                  # auto-loads .kit/extensions/*.go
//	# inside the session, agent calls finalize_phase tool when phase done
//	# OR, manually: /handoff 7

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kit/ext"
)

// busyErrFragment is the substring kit returns from RequestNewSessionFromExtension
// when the agent hasn't yet flipped its busy flag to false. AgentEnd fires from
// TurnEndEvent before the app's drainQueue marks the agent idle, so the
// post-turn NewSession call can race against the busy state. We retry until
// the agent settles.
const (
	busyErrFragment    = "agent is busy"
	newSessionMaxTries = 60
	newSessionBackoff  = 100 * time.Millisecond
)

// ---------------------------------------------------------------------------
// Pending handoff state (tool → OnAgentEnd handoff)
// ---------------------------------------------------------------------------

// pending captures the kickoff prompt staged by a successful finalize_phase
// call. OnAgentEnd consumes it and triggers ctx.NewSession after the agent's
// final wrap-up message lands. Guarded by mu.
type pending struct {
	prompt string
	phase  int
	sha    string
}

var (
	mu             sync.Mutex
	pendingHandoff *pending
)

func stagePending(p pending) {
	mu.Lock()
	defer mu.Unlock()
	pendingHandoff = &p
}

func takePending() *pending {
	mu.Lock()
	defer mu.Unlock()
	p := pendingHandoff
	pendingHandoff = nil
	return p
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

func Init(api ext.API) {
	api.RegisterTool(ext.ToolDef{
		Name: "finalize_phase",
		Description: `Finalize a go-bash build phase. Runs the gate, writes the handoff file, refreshes the root HANDOFF.md pointer, commits, and (on success) schedules an automatic fresh session for the next phase.

Call this tool AT THE END of a phase, after you have:
  - implemented the phase per SPEC.md
  - verified the phase's acceptance criteria are satisfied
  - drafted the full markdown for handoffs/phase-<N>.md (per the template in handoffs/README.md)
  - drafted the full markdown for the root HANDOFF.md pointer
  - drafted the kickoff prompt for the NEXT phase's session

The tool will reject the handoff if 'make ci' fails (unless status="blocked" or skip_gate=true). On failure it returns the gate output verbatim — fix the failures and call the tool again with the same inputs.

On success, the tool stages a session switch: when your current turn ends, kit automatically starts a new session with kickoff_prompt as the first user message (using kit v0.79.0's /new <prompt> path). Keep your final assistant message short — the next session will not see it.`,
		Parameters: `{
  "type": "object",
  "properties": {
    "phase": {
      "type": "integer",
      "minimum": 0,
      "description": "Phase number just completed (e.g. 7)."
    },
    "status": {
      "type": "string",
      "enum": ["complete", "blocked"],
      "description": "Phase outcome. 'complete' runs make ci and switches session. 'blocked' writes the handoff + commits but does NOT run the gate or switch sessions.",
      "default": "complete"
    },
    "handoff_markdown": {
      "type": "string",
      "description": "Full content of handoffs/phase-<phase>.md. Must follow the template in handoffs/README.md (Date, Status, Branch, Gate, What this phase delivered, Done-when checklist, Tests, Decisions & gotchas, Open follow-ups, NEXT PHASE)."
    },
    "pointer_markdown": {
      "type": "string",
      "description": "Full content for the root HANDOFF.md — the short 'start here' pointer. Status line, one-line summary of what's done, link to handoffs/phase-<phase>.md, and the next-phase kickoff prompt."
    },
    "commit_subject": {
      "type": "string",
      "description": "Conventional-commit subject, e.g. 'feat(phase-7): context discovery'."
    },
    "commit_body": {
      "type": "string",
      "description": "Optional commit body (extended description). Empty = no body.",
      "default": ""
    },
    "kickoff_prompt": {
      "type": "string",
      "description": "First user message to inject into the new session. Required when status=complete. Should reference the new handoff file and AGENTS.md. Example: 'Implement Phase 3 of go-bash per SPEC.md. Follow AGENTS.md and read handoffs/phase-2.md first. ...'"
    },
    "skip_gate": {
      "type": "boolean",
      "description": "Skip the 'make ci' verification step. Only use when you have a very good reason (e.g. you already ran the gate manually this turn).",
      "default": false
    },
    "tag": {
      "type": "boolean",
      "description": "Also create a lightweight git tag phase-<phase>.",
      "default": false
    }
  },
  "required": ["phase", "handoff_markdown", "pointer_markdown", "commit_subject"]
}`,
		ExecuteWithContext: func(input string, tc ext.ToolContext) (string, error) {
			return runFinalizePhase(input, tc)
		},
	})

	// Auto-switch sessions once the agent finishes its wrap-up turn after a
	// successful finalize_phase call.
	api.OnAgentEnd(func(_ ext.AgentEndEvent, ctx ext.Context) {
		p := takePending()
		if p == nil || p.prompt == "" {
			return
		}
		go func(prompt string, phase int, sha string) {
			err := newSessionWithRetry(ctx, prompt)
			if err != nil {
				ctx.PrintError(fmt.Sprintf("handoff: could not start phase-%d session: %v", phase+1, err))
				return
			}
			ctx.PrintInfo(fmt.Sprintf("handoff: phase-%d committed (%s), launched fresh session for phase %d", phase, sha, phase+1))
		}(p.prompt, p.phase, p.sha)
	})

	// Manual fallback: /handoff [N] injects a directive into the chat so the
	// human user can ask the agent to finalize. The agent then composes the
	// markdowns and calls finalize_phase itself.
	api.RegisterCommand(ext.CommandDef{
		Name:        "handoff",
		Description: "Ask the agent to finalize the current Flyt phase via the finalize_phase tool. Optional arg: phase number.",
		Execute: func(args string, ctx ext.Context) (string, error) {
			phaseHint := strings.TrimSpace(args)
			msg := buildHandoffDirective(phaseHint)
			ctx.SendMultimodalMessage(msg, nil)
			return "", nil
		},
	})
}

func buildHandoffDirective(phaseHint string) string {
	var sb strings.Builder
	sb.WriteString("Close out the current go-bash build phase. Follow the procedure in handoffs/README.md and AGENTS.md.\n\n")
	if phaseHint != "" {
		fmt.Fprintf(&sb, "Phase just completed: %s.\n\n", phaseHint)
	} else {
		sb.WriteString("Infer the phase you just completed from the git diff and SPEC.md.\n\n")
	}
	sb.WriteString("Steps:\n")
	sb.WriteString("1. Tick off the phase's acceptance criteria in SPEC.md.\n")
	sb.WriteString("2. Draft the full markdown for handoffs/phase-<N>.md per the template in handoffs/README.md.\n")
	sb.WriteString("3. Draft the new root HANDOFF.md pointer with the next-phase kickoff prompt.\n")
	sb.WriteString("4. Call the `finalize_phase` tool with phase, handoff_markdown, pointer_markdown, commit_subject, commit_body, and kickoff_prompt.\n")
	sb.WriteString("   The tool runs `make ci`, writes the files, commits, and auto-launches the next session.\n")
	sb.WriteString("5. Keep your final assistant message short — the new session will not see it.\n")
	return sb.String()
}

// ---------------------------------------------------------------------------
// Tool: finalize_phase
// ---------------------------------------------------------------------------

type finalizeInput struct {
	Phase           int    `json:"phase"`
	Status          string `json:"status"`
	HandoffMarkdown string `json:"handoff_markdown"`
	PointerMarkdown string `json:"pointer_markdown"`
	CommitSubject   string `json:"commit_subject"`
	CommitBody      string `json:"commit_body"`
	KickoffPrompt   string `json:"kickoff_prompt"`
	SkipGate        bool   `json:"skip_gate"`
	Tag             bool   `json:"tag"`
}

// newSessionWithRetry calls ctx.NewSession but tolerates the brief window
// between AgentEnd firing and the app marking itself idle. It retries up to
// newSessionMaxTries times, sleeping newSessionBackoff between attempts.
// Any non-busy error is returned immediately.
func newSessionWithRetry(ctx ext.Context, prompt string) error {
	var lastErr error
	for i := 0; i < newSessionMaxTries; i++ {
		err := ctx.NewSession(prompt)
		if err == nil {
			return nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), busyErrFragment) {
			return err
		}
		time.Sleep(newSessionBackoff)
	}
	return fmt.Errorf("agent still busy after %s: %w",
		time.Duration(newSessionMaxTries)*newSessionBackoff, lastErr)
}

func runFinalizePhase(input string, tc ext.ToolContext) (string, error) {
	var in finalizeInput
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return "", fmt.Errorf("invalid input JSON: %w", err)
	}
	if in.Status == "" {
		in.Status = "complete"
	}

	if err := validateFinalize(in); err != nil {
		return "", err
	}
	if err := assertRepoRoot(); err != nil {
		return "", err
	}

	// 1. Gate.
	if in.Status == "complete" && !in.SkipGate {
		if tc.OnProgress != nil {
			tc.OnProgress("running `make ci`…")
		}
		out, ok := runMakeCI(tc)
		if !ok {
			return formatGateFailure(out), nil
		}
	}

	// 2. Handoff file.
	handoffPath := filepath.Join("handoffs", fmt.Sprintf("phase-%d.md", in.Phase))
	if tc.OnProgress != nil {
		tc.OnProgress("writing " + handoffPath)
	}
	if err := writeFileEnsureNewline(handoffPath, in.HandoffMarkdown); err != nil {
		return "", fmt.Errorf("write %s: %w", handoffPath, err)
	}

	// 3. Root pointer.
	if tc.OnProgress != nil {
		tc.OnProgress("writing HANDOFF.md")
	}
	if err := writeFileEnsureNewline("HANDOFF.md", in.PointerMarkdown); err != nil {
		return "", fmt.Errorf("write HANDOFF.md: %w", err)
	}

	// 4. Commit.
	if tc.OnProgress != nil {
		tc.OnProgress("git add + commit")
	}
	if out, err := runGit("add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %w\n%s", err, out)
	}
	commitArgs := []string{"commit", "-m", in.CommitSubject}
	if strings.TrimSpace(in.CommitBody) != "" {
		commitArgs = append(commitArgs, "-m", in.CommitBody)
	}
	if out, err := runGit(commitArgs...); err != nil {
		return "", fmt.Errorf("git commit: %w\n%s", err, out)
	}
	sha, err := runGitTrimmed("rev-parse", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}

	// Optional tag.
	tagName := ""
	if in.Tag {
		tagName = fmt.Sprintf("phase-%d", in.Phase)
		if out, err := runGit("tag", tagName); err != nil {
			// Non-fatal: the file/commit work already landed.
			tagName = fmt.Sprintf("(tag %s failed: %v: %s)", tagName, err, out)
		}
	}

	// 5. Stage the session switch (only on complete status).
	switchScheduled := false
	if in.Status == "complete" && strings.TrimSpace(in.KickoffPrompt) != "" {
		stagePending(pending{
			prompt: in.KickoffPrompt,
			phase:  in.Phase,
			sha:    sha,
		})
		switchScheduled = true
	}

	return formatSuccess(in, sha, tagName, switchScheduled), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func validateFinalize(in finalizeInput) error {
	if in.Phase < 0 {
		return fmt.Errorf("phase must be >= 0 (got %d)", in.Phase)
	}
	switch in.Status {
	case "complete", "blocked":
	default:
		return fmt.Errorf(`status must be "complete" or "blocked" (got %q)`, in.Status)
	}
	if strings.TrimSpace(in.HandoffMarkdown) == "" {
		return fmt.Errorf("handoff_markdown is required and must be non-empty")
	}
	if strings.TrimSpace(in.PointerMarkdown) == "" {
		return fmt.Errorf("pointer_markdown is required and must be non-empty")
	}
	if strings.TrimSpace(in.CommitSubject) == "" {
		return fmt.Errorf("commit_subject is required and must be non-empty")
	}
	if in.Status == "complete" && strings.TrimSpace(in.KickoffPrompt) == "" {
		return fmt.Errorf("kickoff_prompt is required when status=complete (the next session needs a first prompt)")
	}
	return nil
}

// assertRepoRoot makes sure the tool is running from a go-bash repo checkout —
// presence of SPEC.md, handoffs/, and a .git/ dir is a sufficient signal.
func assertRepoRoot() error {
	required := []string{
		"SPEC.md",
		"handoffs",
		".git",
	}
	for _, p := range required {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("not in a go-bash repo root (missing %s): %w", p, err)
		}
	}
	return nil
}

func writeFileEnsureNewline(path, content string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func runMakeCI(tc ext.ToolContext) (string, bool) {
	cmd := exec.Command("make", "ci")
	// go-bash tests must not perform real network calls. The Network
	// subpackage tests gate on this env var; the suite respects it.
	cmd.Env = append(os.Environ(), "GOBASH_TEST_NO_NETWORK=1")
	out, err := cmd.CombinedOutput()
	if tc.IsCancelled != nil && tc.IsCancelled() {
		return "make ci cancelled", false
	}
	return string(out), err == nil
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runGitTrimmed(args ...string) (string, error) {
	out, err := runGit(args...)
	return strings.TrimSpace(out), err
}

// ---------------------------------------------------------------------------
// Result formatting
// ---------------------------------------------------------------------------

func formatGateFailure(out string) string {
	trimmed := tailLines(out, 200)
	var sb strings.Builder
	sb.WriteString("GATE FAILED — `make ci` did not pass.\n")
	sb.WriteString("No handoff file was written. No commit was made. No session switch was staged.\n\n")
	sb.WriteString("Fix the failures shown below, then call finalize_phase again with the same inputs.\n\n")
	sb.WriteString("---- last lines of `make ci` output ----\n")
	sb.WriteString(trimmed)
	if !strings.HasSuffix(trimmed, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("---- end ----\n")
	return sb.String()
}

func formatSuccess(in finalizeInput, sha, tagInfo string, switchScheduled bool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Phase %d finalized (%s).\n", in.Phase, in.Status)
	fmt.Fprintf(&sb, "  - handoffs/phase-%d.md written\n", in.Phase)
	sb.WriteString("  - HANDOFF.md pointer refreshed\n")
	fmt.Fprintf(&sb, "  - committed: %s — %s\n", sha, in.CommitSubject)
	if tagInfo != "" {
		fmt.Fprintf(&sb, "  - tag: %s\n", tagInfo)
	}
	if switchScheduled {
		fmt.Fprintf(&sb, "\nA fresh session for phase %d will start automatically when this turn ends.\n", in.Phase+1)
		sb.WriteString("Keep your final assistant message brief — the new session will not see it.\n")
	} else if in.Status == "blocked" {
		sb.WriteString("\nStatus=blocked: no session switch scheduled. Investigate the failures listed in the handoff.\n")
	}
	return sb.String()
}

func tailLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
