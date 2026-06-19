// Package help implements the `help` built-in.
//
// Usage:
//
//	help                   list every registered command, one per line
//	help -d                list with placeholder descriptions
//	help NAME...           print each NAME's existence (exit 1 if any missing)
package help

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "help [-d] [NAME...]"

const helpText = `Usage: help [-d] [NAME...]
List or query the built-in commands registered on this Bash instance.

  -d          include a placeholder description after each name
  --help      show this help and exit
`

// New returns the help command.
func New() command.Command { return command.Define("help", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	withDesc := false
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
		case a == "-d":
			withDesc = true
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	names := args[i:]
	if c.Registry == nil {
		return builtinutil.Errorf(c.Stderr, "help", 1, "no registry available")
	}

	if len(names) == 0 {
		all := c.Registry.Names()
		if c.Stdout != nil {
			for _, n := range all {
				if withDesc {
					_, _ = fmt.Fprintf(c.Stdout, "%-20s  built-in command\n", string(n))
				} else {
					_, _ = io.WriteString(c.Stdout, string(n))
					_, _ = io.WriteString(c.Stdout, "\n")
				}
			}
		}
		return command.Result{ExitCode: 0}
	}

	exit := 0
	for _, n := range names {
		if !c.Registry.Has(n) {
			_ = builtinutil.Errorf(c.Stderr, "help", 1, "no help topics match `%s'", n)
			exit = 1
			continue
		}
		if c.Stdout != nil {
			if withDesc {
				_, _ = fmt.Fprintf(c.Stdout, "%-20s  built-in command\n", n)
			} else {
				_, _ = io.WriteString(c.Stdout, n)
				_, _ = io.WriteString(c.Stdout, "\n")
			}
		}
	}
	return command.Result{ExitCode: exit}
}

func init() { command.RegisterBuiltin(New()) }
