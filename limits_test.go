package gobash_test

import (
	"testing"
	"time"

	gobash "github.com/mark3labs/go-bash"
)

// TestDefaultResolvedLimits asserts that the default values produced by
// ResolveLimits(nil) match the table frozen in the spec exactly. The
// values are law; any divergence here must be reconciled in the spec before
// the build can proceed.
func TestDefaultResolvedLimits(t *testing.T) {
	got := gobash.ResolveLimits(nil)
	want := gobash.ResolvedLimits{
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
		MaxStringLength:          10 * 1024 * 1024,
		MaxArrayElements:         100000,
		MaxHeredocSize:           10 * 1024 * 1024,
		MaxSubstitutionDepth:     50,
		MaxBraceExpansionResults: 10000,
		MaxOutputSize:            10 * 1024 * 1024,
		MaxFileDescriptors:       1024,
		MaxSourceDepth:           100,
	}
	if got != want {
		t.Errorf("ResolveLimits(nil) mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// TestResolveLimitsOverrides asserts that explicitly-set pointer fields
// take effect and that nil pointers fall through to defaults.
func TestResolveLimitsOverrides(t *testing.T) {
	cmd := 42
	pyto := 99 * time.Second
	in := &gobash.ExecutionLimits{
		MaxCommandCount:  &cmd,
		MaxPythonTimeout: &pyto,
	}
	got := gobash.ResolveLimits(in)
	if got.MaxCommandCount != 42 {
		t.Errorf("MaxCommandCount = %d; want 42", got.MaxCommandCount)
	}
	if got.MaxPythonTimeout != 99*time.Second {
		t.Errorf("MaxPythonTimeout = %v; want 99s", got.MaxPythonTimeout)
	}
	// Fall-through default for an un-overridden field.
	if got.MaxCallDepth != 100 {
		t.Errorf("MaxCallDepth = %d; want default 100", got.MaxCallDepth)
	}
}
