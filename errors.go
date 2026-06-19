package gobash

import (
	"fmt"

	"github.com/mark3labs/go-bash/parser"
)

// ExecutionLimitError is returned by Exec when a script trips an execution
// limit threshold. Use errors.As to extract the offending Limit name and
// the configured Value at the time of the failure.
//
// Enforcement of each limit lands across multiple phases (counters live in
// Phase 2, expansion-side caps in Phase 4); the error type itself is part
// of the Phase 1 public surface.
type ExecutionLimitError struct {
	Limit string
	Value int
}

func (e *ExecutionLimitError) Error() string {
	return fmt.Sprintf("execution limit exceeded: %s (value=%d)", e.Limit, e.Value)
}

// ParseError is the typed error returned by the parser front-end. It is
// defined in the parser subpackage and re-exported here via a type alias
// so existing call sites and tests that reference gobash.ParseError
// continue to compile unchanged.
//
// Consumers should match it with errors.As against either spelling.
type ParseError = parser.ParseError

// LexerError reports a lexer/tokenizer-level failure.
type LexerError struct {
	Msg  string
	Line int
	Col  int
}

func (e *LexerError) Error() string {
	if e.Line == 0 && e.Col == 0 {
		return "lexer error: " + e.Msg
	}
	return fmt.Sprintf("lexer error at %d:%d: %s", e.Line, e.Col, e.Msg)
}

// SecurityViolationError mirrors the just-bash class of the same name. The
// Go port enforces its threat model architecturally (no os/exec, virtual
// FS, allow-listed network) so this error type is rarely emitted; it is
// kept for direct API parity so host code that switches on it ports over.
type SecurityViolationError struct {
	Msg string
}

func (e *SecurityViolationError) Error() string {
	return "security violation: " + e.Msg
}

// PosixFatalError signals a POSIX-mode fatal shell error (e.g. `set -e`
// hitting a non-zero command in a POSIX-mandated context).
type PosixFatalError struct {
	Msg  string
	Code int
}

func (e *PosixFatalError) Error() string {
	return fmt.Sprintf("posix fatal: %s (code=%d)", e.Msg, e.Code)
}

// ArithmeticError reports an arithmetic-expansion failure such as
// division by zero or an unparseable expression inside $((…)).
type ArithmeticError struct {
	Msg string
}

func (e *ArithmeticError) Error() string {
	return "arithmetic error: " + e.Msg
}

// ExitError is the internal signal raised by the `exit N` builtin. The
// runtime translates it into BashExecResult.ExitCode at the Exec
// boundary; it is therefore rarely observed by callers but is exported
// for parity with the just-bash error class.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit %d", e.Code)
}

// AbortedError is returned when execution was aborted for a reason other
// than a limit or context cancellation (for example, a transform-plugin
// veto).
type AbortedError struct {
	Reason string
}

func (e *AbortedError) Error() string {
	return "aborted: " + e.Reason
}
