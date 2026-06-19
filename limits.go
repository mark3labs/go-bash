package gobash

import (
	"time"

	"github.com/mark3labs/go-bash/command"
)

// ExecutionLimits caps script behavior. A nil field falls through to the
// documented default; a non-nil pointer field overrides per-construction.
// The full default table is frozen in SPEC.md §2.1.
type ExecutionLimits struct {
	MaxCallDepth             *int
	MaxCommandCount          *int
	MaxLoopIterations        *int
	MaxAwkIterations         *int
	MaxSedIterations         *int
	MaxJqIterations          *int
	MaxSqliteTimeout         *time.Duration
	MaxPythonTimeout         *time.Duration
	MaxJsTimeout             *time.Duration
	MaxGlobOperations        *int
	MaxStringLength          *int
	MaxArrayElements         *int
	MaxHeredocSize           *int
	MaxSubstitutionDepth     *int
	MaxBraceExpansionResults *int
	MaxOutputSize            *int
	MaxFileDescriptors       *int
	MaxSourceDepth           *int
}

// ResolvedLimits is ExecutionLimits with all defaults applied. The
// runtime threads a value of this type rather than ExecutionLimits so
// internal callsites never have to nil-check.
//
// As of Phase 10 ResolvedLimits is a type alias for command.Limits so
// the dispatch Context can carry the same struct without conversion;
// the alias keeps the gobash.ResolvedLimits public surface stable.
type ResolvedLimits = command.Limits

// DefaultLimits returns the fully-resolved default execution limits. The
// values are law — they must match SPEC.md §2.1 exactly. Tests assert
// this; do not adjust either side without updating the spec.
func DefaultLimits() ResolvedLimits {
	const mib10 = 10 * 1024 * 1024
	return ResolvedLimits{
		MaxCallDepth:             100,
		MaxCommandCount:          10000,
		MaxLoopIterations:        10000,
		MaxAwkIterations:         10000,
		MaxSedIterations:         10000,
		MaxJqIterations:          10000,
		MaxSqliteTimeout:         5 * time.Second,
		MaxPythonTimeout:         10 * time.Second,
		MaxJsTimeout:             10 * time.Second,
		MaxGlobOperations:        100000,
		MaxStringLength:          mib10,
		MaxArrayElements:         100000,
		MaxHeredocSize:           mib10,
		MaxSubstitutionDepth:     50,
		MaxBraceExpansionResults: 10000,
		MaxOutputSize:            mib10,
		MaxFileDescriptors:       1024,
		MaxSourceDepth:           100,
	}
}

// ResolveLimits returns the effective limits, applying defaults to any
// nil-valued field of in. A nil in yields the defaults verbatim.
func ResolveLimits(in *ExecutionLimits) ResolvedLimits {
	out := DefaultLimits()
	if in == nil {
		return out
	}
	if in.MaxCallDepth != nil {
		out.MaxCallDepth = *in.MaxCallDepth
	}
	if in.MaxCommandCount != nil {
		out.MaxCommandCount = *in.MaxCommandCount
	}
	if in.MaxLoopIterations != nil {
		out.MaxLoopIterations = *in.MaxLoopIterations
	}
	if in.MaxAwkIterations != nil {
		out.MaxAwkIterations = *in.MaxAwkIterations
	}
	if in.MaxSedIterations != nil {
		out.MaxSedIterations = *in.MaxSedIterations
	}
	if in.MaxJqIterations != nil {
		out.MaxJqIterations = *in.MaxJqIterations
	}
	if in.MaxSqliteTimeout != nil {
		out.MaxSqliteTimeout = *in.MaxSqliteTimeout
	}
	if in.MaxPythonTimeout != nil {
		out.MaxPythonTimeout = *in.MaxPythonTimeout
	}
	if in.MaxJsTimeout != nil {
		out.MaxJsTimeout = *in.MaxJsTimeout
	}
	if in.MaxGlobOperations != nil {
		out.MaxGlobOperations = *in.MaxGlobOperations
	}
	if in.MaxStringLength != nil {
		out.MaxStringLength = *in.MaxStringLength
	}
	if in.MaxArrayElements != nil {
		out.MaxArrayElements = *in.MaxArrayElements
	}
	if in.MaxHeredocSize != nil {
		out.MaxHeredocSize = *in.MaxHeredocSize
	}
	if in.MaxSubstitutionDepth != nil {
		out.MaxSubstitutionDepth = *in.MaxSubstitutionDepth
	}
	if in.MaxBraceExpansionResults != nil {
		out.MaxBraceExpansionResults = *in.MaxBraceExpansionResults
	}
	if in.MaxOutputSize != nil {
		out.MaxOutputSize = *in.MaxOutputSize
	}
	if in.MaxFileDescriptors != nil {
		out.MaxFileDescriptors = *in.MaxFileDescriptors
	}
	if in.MaxSourceDepth != nil {
		out.MaxSourceDepth = *in.MaxSourceDepth
	}
	return out
}
