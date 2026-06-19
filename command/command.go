// Package command defines the Command interface every gobash builtin
// implements, the Context value those builtins receive at dispatch
// time, the Result value they return, and the Registry that maps
// names to implementations. The package is the contract layer between
// the runtime (gobash + gobash/interp) and the individual built-in
// packages introduced in Phase 10.
//
// Phase 8 lands the registry skeleton and the dispatch plumbing. The
// Context shape is intentionally minimal — only the fields whose
// types are stable in Phases 1–8. SPEC §8.1 lists additional fields
// (Fetch, Sleep, Trace, Limits, InvokeTool, FDs, JSBootstrap, etc.);
// each lands in the phase that owns its dependency:
//
//   - Phase 9 adds Fetch (network.Doer).
//   - Phase 10 wires Sleep / Trace / Limits / ExportedEnv through —
//     the built-in command bodies are the first consumers.
//   - Phase 11 adds Exec (sub-shell invocation) and Signal.
//   - Phase 15 adds JSBootstrap and InvokeTool.
//
// Adding fields is non-breaking; reserving them in the spec is enough.
//
// Cited surface: SPEC §8.1, §8.2, §8.4. Reference (read-only):
// vercel-labs/just-bash, src/commands/registry.ts.
package command

import (
	"context"
	"io"

	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/network"
)

// SubExecFunc is the signature of the sub-shell invocation hook
// reserved for Phase 11. Declared here so Context.Exec's type stays
// stable across phases; nil until the source/eval/. builtins land.
type SubExecFunc = func(ctx context.Context, script string, opts SubExecOptions) (Result, error)

// Name is the canonical lookup key for a Command. It mirrors the TS
// `CommandName` branded string type from src/commands/registry.ts.
// Stringification is straightforward; tests should construct Name
// values directly rather than going through fmt.
type Name string

// Command is the interface every gobash built-in implements. The
// runtime invokes Execute with the full argv (NOT including the
// command name's index 0 in args) and the dispatch Context. A
// non-zero Result.ExitCode is NOT an error — it is reported through
// the runner as the script's exit status.
//
// SPEC §8.1: the interface also includes a Trusted() bool used only
// by the sandbox subpackage (Phase 17+). Built-ins that opt out of
// sandbox trust will return false; until the sandbox lands, every
// implementation returns true and the runtime ignores the value.
type Command interface {
	// Name returns the canonical lookup key for the command. The
	// registry uses this value as the map key, so it must not change
	// across calls.
	Name() Name

	// Execute runs the command. ctx is the parent execution context
	// (cancellation propagates through it). args is the full argv —
	// args[0] is the command name as invoked by the script, args[1:]
	// are the positional arguments. c carries the dispatch state.
	Execute(ctx context.Context, args []string, c *Context) Result

	// Trusted reports whether the command may run inside a Phase 17+
	// sandbox. It is metadata only; no Phase 8–16 code paths inspect
	// it. Built-ins constructed via Define default to true.
	Trusted() bool
}

// Context is the per-dispatch state passed to a Command's Execute
// method. The runtime constructs a fresh Context for every command
// invocation; mutating it is permitted but ephemeral — changes do
// NOT propagate back to the parent Bash unless the runtime itself
// writes them back (see Env mutation semantics in SPEC §5.6).
//
// Phase 8 wires the seven fields below; the rest land in their
// owning phases (see the package doc comment).
type Context struct {
	// FS is the virtual filesystem backing every script-side file
	// operation. Commands MUST route I/O through this field — never
	// through host os.* helpers — to preserve the sandbox contract.
	FS gbfs.FileSystem

	// Cwd is the working directory at dispatch time, resolved through
	// the VFS (not the host disk). Commands that perform relative
	// path resolution should join against this value.
	Cwd string

	// Env is the effective environment at dispatch time. Commands may
	// mutate this map; the runtime decides whether to persist the
	// mutation per SPEC §5.6.
	Env map[string]string

	// Stdin, Stdout, Stderr are the streams plumbed through from the
	// runner's current redirection state. Commands write structured
	// output here; they should NOT populate Result.Stdout/Stderr in
	// the same call (the runtime treats those as a fallback when the
	// writers are not consulted, e.g. for early returns).
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Registry is a back-reference to the dispatch registry so
	// commands that delegate to peers (e.g. a future `which`) can
	// look them up without a global state lookup.
	Registry *Registry

	// Fetch is the network Doer made available to commands like
	// `curl` (Phase 10). It is nil when the host did not configure
	// BashOptions.Network or BashOptions.Fetch — commands that need
	// network MUST check for nil and produce a `network disabled`
	// diagnostic rather than panicking. Phase 9 plumbs this field;
	// Phase 10 wires it to its first real consumer.
	Fetch network.Doer

	// Sleep is the host-supplied sleep hook used by Phase 10's
	// `sleep` builtin. When nil, the builtin uses a real time.Sleep
	// guarded by ctx. Tests override this to elide wall-clock waits.
	Sleep SleepFunc

	// Trace is the host-supplied trace hook. Wave A built-ins do not
	// emit events; the field is plumbed today so later waves can
	// instrument hot paths without a follow-up Context surface bump.
	Trace TraceFunc

	// Limits is the resolved execution-limit set for the current
	// Exec. Wave A built-ins ignore it; Wave D's awk/sed/jq and the
	// optional runtimes (sqlite/python/js) consume the iteration and
	// timeout caps.
	Limits Limits

	// ExportedEnv is the per-dispatch exported-environment view used
	// by env/printenv/export (Phase 10 Wave G). Wave A does not
	// consume it; the field is plumbed today as a landing zone so
	// later waves do not require a Context surface bump.
	ExportedEnv map[string]string

	// Aliases is the per-Bash alias table consulted and mutated by
	// the `alias` / `unalias` built-ins (Phase 10 Wave G). Aliases
	// only expand at parse time when `shopt expand_aliases` is on;
	// the parse-side wiring lands in Phase 11. nil is treated as an
	// empty table.
	Aliases AliasTable

	// History is the per-Bash command history ring (Phase 10 Wave G).
	// The `history` built-in reads and mutates this ring. The runtime
	// itself does NOT currently push commands into the ring — Phase 19
	// will wire automatic recording at parse time. Hosts can populate
	// it manually via Bash.History() until then.
	History HistoryRing

	// Exec is the sub-shell invocation hook used by the Phase 10
	// Wave G `bash` / `sh` / `timeout` built-ins (which would
	// otherwise need to recurse into the runtime). SPEC §8.1 reserves
	// this field for Phase 11 (source / eval / .); Wave G needs it
	// earlier — see handoffs/phase-10.md Decisions for the ordering
	// note. nil means sub-shell features are unavailable; consumers
	// MUST nil-check and produce a clean diagnostic.
	Exec SubExecFunc

	// SourceDepth tracks recursive sub-shell / source / . / eval
	// invocations against Limits.MaxSourceDepth (Phase 11). The
	// runtime supplies the current depth at dispatch time; built-ins
	// that delegate to c.Exec must bump it by 1 in the
	// SubExecOptions they forward (the runtime then reads it back
	// when constructing the next dispatch Context). A value at or
	// above Limits.MaxSourceDepth means the call must be rejected
	// with a clean diagnostic.
	SourceDepth int

	// Shopt is the per-Bash shell-option table (`shopt`). The
	// `shopt` builtin reads and mutates it; the alias-expansion path
	// (Phase 11) consults `expand_aliases` at parse time. Nil-safe:
	// a nil table reports every option as off and silently drops
	// writes.
	Shopt ShoptTable
}

// ShoptTable is the read/write surface for the `shopt` builtin. The
// concrete implementation lives in internal/runtimestate; this
// interface lets command.Context refer to it without an import
// cycle.
type ShoptTable interface {
	IsSet(name string) bool
	Set(name string, on bool)
	Names() []string
}

// AliasTable is the read/write surface for the `alias` / `unalias`
// builtins. The concrete implementation lives in internal/runtimestate;
// this interface lets command.Context (and the public Bash.Aliases
// getter) refer to it without leaking the internal type or creating an
// import cycle. A nil value is NOT safe to call through — the runtime
// always supplies a live table.
type AliasTable interface {
	Get(name string) (string, bool)
	Set(name, value string)
	Unset(name string) bool
	Clear()
	Names() []string
	All() map[string]string
}

// HistoryRing is the read/write surface for the `history` builtin. The
// concrete implementation lives in internal/runtimestate; this
// interface lets command.Context (and the public Bash.History getter)
// refer to it without leaking the internal type. A nil value is NOT
// safe to call through — the runtime always supplies a live ring.
type HistoryRing interface {
	Add(cmd string)
	Clear()
	List() (seqs []int, cmds []string)
	Len() int
}

// Result is the outcome of a Command.Execute call. A non-zero
// ExitCode is propagated to the runner as the command's exit status
// but is NOT treated as a harness error.
//
// Stdout / Stderr are FALLBACK string buffers for commands that did
// not consume Context.Stdout / Context.Stderr directly. The runtime
// flushes any non-empty value here to the corresponding writer
// before translating ExitCode into an mvdan/sh ExitStatus. Commands
// that already wrote through the writers MUST leave these empty to
// avoid double-emission.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// SubExecOptions mirrors the top-level ExecOptions for the
// Context.Exec hook introduced in Phase 11 (when source/eval/.
// land). It is declared here so the public surface freezes from
// Phase 8 onward; the field set will grow alongside ExecOptions.
type SubExecOptions struct {
	Env        map[string]string
	ReplaceEnv bool
	Cwd        string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Args       []string

	// SourceDepth, when non-zero, is the depth the sub-shell should
	// start at. The Phase 11 source / eval / . / bash / sh / timeout
	// builtins set this to parent.SourceDepth + 1 so MaxSourceDepth
	// trips cleanly across nested invocations.
	SourceDepth int
}
