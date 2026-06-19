// Package unset implements the `unset` shell built-in.
// `unset [-v|-f] NAME...` removes variables or functions. mvdan/sh
// ships its own; the registered version reachable via /bin/unset
// mutates only c.Env / c.ExportedEnv (no propagation to runner).
package unset

import (
	"context"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "unset [-v|-f] NAME..."

// New returns the unset command.
func New() command.Command { return command.Define("unset", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	// -f is accepted but functions are mvdan-managed; we no-op.
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case a == "-v" || a == "-f":
			// recognized
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	for _, name := range args[i:] {
		if c.Env != nil {
			delete(c.Env, name)
		}
		if c.ExportedEnv != nil {
			delete(c.ExportedEnv, name)
		}
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
