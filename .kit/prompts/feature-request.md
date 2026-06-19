---
description: Create a feature request using the GitHub template
---

Create a feature request for the go-bash repository. The user wants to request: $@

## Feature Request Template

This prompt uses the `feature_request` GitHub template which requires:

| Field | Required | Purpose |
|-------|----------|---------|
| **Feature Description** | Yes | What should be added or changed |
| **Motivation / Use Case** | Yes | Why is this needed? What problem does it solve? |
| **Proposed Implementation** | No | How do you think this should work? |

## Steps

1. **Understand the request** from the user input: $@
   - What capability is missing?
   - What would the ideal behavior look like?

2. **Ask clarifying questions** if needed:
   - "What problem does this solve for you?"
   - "How would you expect this to work?"
   - "Are there similar features in other tools you use?"

3. **Craft the title** using conventional format:
   - `feat: <short description>`
   - Lowercase, imperative mood, ≤72 chars
   - Good examples:
     - `feat: add MaxOutputBytesPerCommand execution limit`
     - `feat: support OverlayRoot in sandbox.Options`
     - `feat: bcrypt builtin for password hashing in scripts`
   - Bad examples:
     - `Feature request: can we have...` (too vague)
     - `It would be nice if...` (not imperative)

4. **Build the body** with the template fields:

   **Feature Description:**
   - Clear statement of what to add/change
   - Be specific about the behavior
   - Include UI/UX details if relevant

   **Motivation / Use Case:**
   - What problem does this solve?
   - Current workaround (if any) and why it's insufficient
   - Who benefits from this feature?

   **Proposed Implementation** (optional but helpful):
   - High-level approach
   - API changes if applicable
   - Example usage code

5. **Create the issue**:
   ```bash
   gh issue create --template feature_request --title "feat: ..." --body "..."
   ```

6. **Confirm success**:
   - Show the issue URL and number
   - Mention it was created with the feature_request template

## Guidelines

- Focus on the *problem* first, then the solution
- Include concrete examples of how the feature would be used
- Consider edge cases and mention them
- If proposing API changes, show before/after code
- Check if similar features exist in related tools (mention them for reference)
- Align with go-bash's scope: a sandboxed in-process bash interpreter for AI agents (see SPEC.md for what's in/out of scope)

## Example

User: `/feature-request I want a per-command output cap so one runaway builtin can't burn the whole MaxOutputSize budget`

You:
1. Title: `feat: add MaxOutputBytesPerCommand execution limit`
2. Body:
   - **Feature Description**: Add a `MaxOutputBytesPerCommand` field to `ExecutionLimits` that caps stdout+stderr per command, distinct from the existing combined-Exec `MaxOutputSize`. Tripping returns `*ExecutionLimitError{Limit: "MaxOutputBytesPerCommand"}`.
   - **Motivation**: In an agent loop, a single misbehaving `find /` saturates the 1 MiB combined budget and starves later commands in the same pipeline. A per-command cap lets the host reject the offender without losing the rest.
   - **Proposed Implementation**: Wrap the per-command writer in a fresh `ringbuf.Tracker` keyed off the new limit value; reuse the existing trip-and-cancel pattern in `bash.go`.

3. Execute: `gh issue create --template feature_request --title "feat: ..." --body "..."`
4. Confirm: Created issue #N using feature_request template
