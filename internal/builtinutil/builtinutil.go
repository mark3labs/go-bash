// Package builtinutil contains small helpers shared by Phase 10's
// Wave A built-in command packages. The helpers are deliberately
// minimal — each built-in still owns its own argv parsing — but
// the common "print usage on --help, error on unknown option"
// pattern lives here to keep parity stable.
package builtinutil

import (
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
)

// PrintHelp writes usage + a trailing newline to w. SPEC §10 contract:
// --help exits 0 with usage written to stdout. The trailing newline
// matches real GNU coreutils.
func PrintHelp(w io.Writer, usage string) {
	if !strings.HasSuffix(usage, "\n") {
		usage += "\n"
	}
	_, _ = io.WriteString(w, usage)
}

// UsageError writes "usage: <usage>\n" to stderr and returns the
// canonical exit-2 result. SPEC §10 contract: unknown options exit 2
// with "usage: ..." to stderr, except where real bash silently
// ignores them (callers that need the bash-silent behavior should
// not call this helper).
func UsageError(stderr io.Writer, usage string) command.Result {
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, "usage: %s\n", usage)
	}
	return command.Result{ExitCode: 2}
}

// Errorf writes "<cmd>: <msg>\n" to stderr and returns a Result with
// the given exit code. The "<cmd>: " prefix mirrors coreutils
// diagnostics (e.g. "basename: missing operand"). Pass an empty cmd
// to suppress the prefix.
func Errorf(stderr io.Writer, cmd string, code int, format string, args ...any) command.Result {
	if stderr != nil {
		if cmd != "" {
			_, _ = fmt.Fprintf(stderr, "%s: ", cmd)
		}
		_, _ = fmt.Fprintf(stderr, format, args...)
		_, _ = io.WriteString(stderr, "\n")
	}
	return command.Result{ExitCode: code}
}
