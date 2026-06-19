// Package dirs implements the `dirs` shell built-in (SPEC §11).
// Prints the current dir (we have no persistent dir stack across
// dispatches; the registered version is a single-entry approximation).
// mvdan/sh ships its own; reachable via /bin/dirs.
package dirs

import (
	"context"
	"fmt"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "dirs [-clpv] [+N | -N]"

// New returns the dirs command.
func New() command.Command { return command.Define("dirs", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, a := range args[1:] {
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		}
		// Other flags (-c -l -p -v, +N, -N) are accepted and ignored;
		// we have no persistent dir stack.
	}
	if c.Stdout != nil {
		_, _ = fmt.Fprintln(c.Stdout, c.Cwd)
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
