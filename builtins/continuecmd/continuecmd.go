// Package continuecmd implements the `continue` shell built-in.
// Stub: accepts any arguments and exits 0. mvdan/sh may
// shadow the bare name; the registered version is reachable via
// /bin/continue.
package continuecmd

import (
	"context"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "continue [ARG...]"

// New returns the continue command.
func New() command.Command { return command.Define("continue", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, a := range args[1:] {
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		}
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
