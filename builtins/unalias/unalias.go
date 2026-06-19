// Package unalias implements the `unalias` built-in (SPEC §10 Wave G).
//
// Usage:
//
//	unalias NAME...        remove each named alias
//	unalias -a             remove every alias
//
// Exits 1 if any NAME was not set.
package unalias

import (
	"context"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "unalias [-a] NAME..."

const helpText = `Usage: unalias [-a] NAME...
Remove each NAME from the alias list.

Options:
  -a          remove every alias
  --help      show this help and exit
`

// New returns the unalias command.
func New() command.Command { return command.Define("unalias", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	all := false
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case a == "-a":
			all = true
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	tbl := c.Aliases
	if all {
		tbl.Clear()
		return command.Result{ExitCode: 0}
	}
	names := args[i:]
	if len(names) == 0 {
		return builtinutil.Errorf(c.Stderr, "unalias", 2, "usage: %s", usage)
	}
	exit := 0
	for _, n := range names {
		if !tbl.Unset(n) {
			_ = builtinutil.Errorf(c.Stderr, "unalias", 1, "%s: not found", n)
			exit = 1
		}
	}
	return command.Result{ExitCode: exit}
}

func init() { command.RegisterBuiltin(New()) }
