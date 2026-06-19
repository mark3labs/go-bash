package command

import (
	"context"
	"time"
)

// SleepFunc is the sleep hook used by Phase 10's `sleep` builtin (and,
// later, `timeout`). When set on Context.Sleep, the builtin invokes it
// instead of time.Sleep so tests can elide wall-clock waits without
// mocking time globally. Returning a non-nil error aborts the sleep
// and is surfaced to the caller as the command's exit status (per
// The spec). Type alias keeps gobash.SleepFunc structurally
// compatible with command.SleepFunc.
type SleepFunc = func(ctx context.Context, d time.Duration) error

// TraceFunc receives instrumentation events emitted by the runtime
// and by trace-instrumented built-ins (Phase 10 lands the field on
// Context; consumers arrive in later waves). Nil-safe: built-ins
// guard the call site rather than relying on a no-op default.
type TraceFunc = func(TraceEvent)

// InvokeToolFunc is the host hook for js-exec tool calls (Phase 15).
// Declared here so the Context shape is frozen across phases.
type InvokeToolFunc = func(ctx context.Context, path, argsJSON string) (string, error)

// TraceEvent describes a single instrumentation point produced by the
// runtime or a built-in. The spec freezes this shape; the gobash
// root package re-exports it via type alias for backward compatibility
// with Phase 1's surface.
type TraceEvent struct {
	Category string
	Name     string
	Duration time.Duration
	Details  map[string]any
}

// Limits is the per-Exec resolved limit set commands may inspect at
// dispatch time. It mirrors the spec verbatim — the gobash root
// package's ResolvedLimits is a type alias to this struct so the
// runtime can pass through a single value.
//
// Wave A built-ins (Phase 10) consume nothing from Limits; the field
// is plumbed today so Wave D's awk/sed/jq (MaxAwkIterations,
// MaxSedIterations, MaxJqIterations) and the optional runtimes
// (MaxSqliteTimeout, MaxPythonTimeout, MaxJsTimeout) can consume them
// without a follow-up Context surface bump.
type Limits struct {
	MaxCallDepth             int
	MaxCommandCount          int
	MaxLoopIterations        int
	MaxAwkIterations         int
	MaxSedIterations         int
	MaxJqIterations          int
	MaxSqliteTimeout         time.Duration
	MaxPythonTimeout         time.Duration
	MaxJsTimeout             time.Duration
	MaxGlobOperations        int
	MaxStringLength          int
	MaxArrayElements         int
	MaxHeredocSize           int
	MaxSubstitutionDepth     int
	MaxBraceExpansionResults int
	MaxOutputSize            int
	MaxFileDescriptors       int
	MaxSourceDepth           int
}
