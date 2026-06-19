// Package truecmd implements the `true` built-in (SPEC §10 Wave A).
// `true` always exits 0; the only flag it honors is --help.
package truecmd

import (
	"context"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const helpText = `Usage: true [--help]
Exit with a status code indicating success.`

// New returns the true command.
func New() command.Command {
	return command.Define("true", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	// SPEC §10: only --help is honored; other args are ignored to
	// match real bash (`true --unknown` exits 0 silently).
	for _, a := range args[1:] {
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		}
	}
	return command.Result{ExitCode: 0}
}

func init() {
	command.RegisterBuiltin(New())
}
