// Package popd implements the `popd` shell built-in.
// Stub: we have no dir stack, so popd always reports "directory stack
// empty" and exits 1. mvdan/sh ships its own; reachable via /bin/popd.
package popd

import (
	"context"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "popd [+N | -N]"

// New returns the popd command.
func New() command.Command { return command.Define("popd", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, a := range args[1:] {
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		}
	}
	return builtinutil.Errorf(c.Stderr, "popd", 1, "directory stack empty")
}

func init() { command.RegisterBuiltin(New()) }
