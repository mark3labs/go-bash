package gobash

import (
	"io"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/network"
	"github.com/mark3labs/go-bash/transform"
)

// BashOptions configures a Bash environment at construction time.
//
// Phase 1 wires only the fields whose types are defined in this package.
// Subsequent phases extend this struct in place — see SPEC.md §1.2:
//   - Phase 3 added Files (map[string]FileInit) and FS (fs.FileSystem)
//   - Phase 8 added Commands ([]command.Name) and CustomCommands
//   - Phase 9 adds Fetch (network.Doer) and Network (*network.Config)
//
// All adds are field-only; existing fields keep their names and meaning.
type BashOptions struct {
	Env             map[string]string
	Cwd             string
	ExecutionLimits *ExecutionLimits
	Python          *PythonConfig
	JavaScript      *JavaScriptConfig
	Sleep           SleepFunc
	Logger          Logger
	Trace           TraceFunc
	ProcessInfo     *ProcessInfo

	// Files seeds the initial in-memory filesystem with the given
	// path → FileInit entries. When FS is also supplied, Files is
	// applied to that FileSystem instead of constructing a new memfs.
	Files map[string]fs.FileInit

	// FS replaces the default in-memory filesystem. When nil, gobash
	// creates an empty memfs.FS at construction time. When non-nil,
	// any Files entries are applied to FS via the FileSystem methods.
	FS fs.FileSystem

	// Commands restricts which built-in commands are registered. A
	// nil slice registers every built-in (the default); a non-nil
	// slice filters to exactly the named subset. CustomCommands are
	// NOT filtered through this list — they always register.
	//
	// Phase 8 lands the filter wiring; Phase 10 lands the built-ins
	// the filter actually selects from. Until Phase 10 the only
	// observable effect is on Bash.Registry().Names() and the
	// derived SPEC §7 /bin/X stub set.
	Commands []command.Name

	// CustomCommands override or extend the built-in registry. Each
	// entry is registered last, so any name collision with a built-in
	// is resolved in favor of the CustomCommand. Tests use this hook
	// to inject deterministic sample commands without touching the
	// built-in registration order.
	CustomCommands []command.Command

	// Fetch is the network Doer used by Phase 10's `curl` and any
	// future network-touching built-in. When non-nil it overrides
	// the default SecureFetch built from Network; tests use this hook
	// to inject stub Doers. When nil, gobash falls back to
	// network.NewSecureFetch(Network). Combining Fetch != nil with
	// Network != nil is legal: Network is ignored.
	Fetch network.Doer

	// Network is the allow-list and policy used to build the default
	// SecureFetch Doer when Fetch is nil. When both Fetch and Network
	// are nil, command.Context.Fetch is nil at dispatch time and
	// network-touching built-ins must error out cleanly.
	Network *network.Config

	// TransformPlugins is the ordered list of transform-pipeline
	// plugins applied to every Exec call on this Bash. The plugins
	// are appended to the per-Bash slice at New() time; hosts can
	// add more later via Bash.RegisterTransformPlugin. Each plugin's
	// metadata payload surfaces in BashExecResult.Metadata under the
	// plugin's Name(). When the slice is empty, Exec runs the
	// fast-path that skips the parse → serialize → re-parse round
	// trip entirely (no observable effect from Phase 13).
	TransformPlugins []transform.Plugin

	// Deprecated convenience knobs — honored to match the just-bash TS
	// surface. Prefer ExecutionLimits.
	MaxCallDepth      int
	MaxCommandCount   int
	MaxLoopIterations int
}

// ExecOptions configures a single Exec call. Per-call settings override
// the per-Bash defaults.
type ExecOptions struct {
	Env        map[string]string
	ReplaceEnv bool
	Cwd        string
	RawScript  bool
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	// Args is appended to the first command bypassing parsing. Wiring
	// lands in Phase 5 alongside the interpreter bridge; declared here
	// to freeze the public surface.
	Args []string
}

// ProcessInfo carries virtualized process identifiers exposed to scripts
// via $$, $PPID, etc. Zero value yields the package defaults (see
// SPEC §1.2: PID=1, PPID=0, UID=1000, GID=1000).
type ProcessInfo struct {
	PID  int
	PPID int
	UID  int
	GID  int
}

// SleepFunc replaces the real time.Sleep used by `sleep`, `timeout`, and
// related builtins. Returning a non-nil error aborts the sleeping call.
// Aliased to command.SleepFunc so dispatch Contexts and BashOptions
// share a single underlying func signature (no conversion needed).
type SleepFunc = command.SleepFunc

// TraceFunc receives instrumentation events emitted by the runtime.
// Aliased to command.TraceFunc; see note on SleepFunc.
type TraceFunc = command.TraceFunc

// InvokeToolFunc is the host hook invoked from JavaScript-side tool calls
// (see SPEC Phase 15). It returns the tool's JSON-serialized result.
// Aliased to command.InvokeToolFunc.
type InvokeToolFunc = command.InvokeToolFunc

// TraceEvent describes a single instrumentation point.
// Aliased to command.TraceEvent so the dispatch Context's Trace hook
// and a host-supplied BashOptions.Trace share one type.
type TraceEvent = command.TraceEvent

// Logger receives structured log records emitted by the runtime.
type Logger interface {
	Info(msg string, fields map[string]any)
	Debug(msg string, fields map[string]any)
}

// JavaScriptConfig configures the opt-in JavaScript runtime (Phase 15).
type JavaScriptConfig struct {
	Bootstrap  string
	InvokeTool InvokeToolFunc
}

// PythonConfig configures the opt-in Python hook (Phase 16). The Runtime
// field is added in Phase 16 once the PythonRuntime interface is defined;
// the zero value here exists so callers can pass *PythonConfig today
// without breaking when the field is introduced.
type PythonConfig struct{}
