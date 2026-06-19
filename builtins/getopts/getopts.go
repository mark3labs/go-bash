// Package getopts implements the `getopts` shell built-in.
// A faithful port requires per-loop persistent state (OPTIND, OPTARG),
// which the registered version cannot mutate in the runner's vars.
// We implement a single-shot stub that ALWAYS reports "no more options"
// (exit 1) and exits silently. mvdan/sh's native `getopts` is the
// real implementation; /bin/getopts here is for surface completeness.
package getopts

import (
	"context"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "getopts OPTSTRING NAME [ARG...]"

// New returns the getopts command.
func New() command.Command { return command.Define("getopts", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	if len(args) >= 2 && args[1] == "--help" {
		builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
		return command.Result{ExitCode: 0}
	}
	if len(args) < 3 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	// No persistent OPTIND across dispatches; signal "no more options".
	return command.Result{ExitCode: 1}
}

func init() { command.RegisterBuiltin(New()) }
