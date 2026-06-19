// Package falsecmd implements the `false` built-in.
// `false` always exits 1; the only flag it honors is --help.
package falsecmd

import (
	"context"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const helpText = `Usage: false [--help]
Exit with a status code indicating failure.`

// New returns the false command.
func New() command.Command {
	return command.Define("false", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, a := range args[1:] {
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		}
	}
	return command.Result{ExitCode: 1}
}

func init() {
	command.RegisterBuiltin(New())
}
