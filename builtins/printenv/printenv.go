// Package printenv implements the `printenv` built-in.
//
// Usage:
//
//	printenv [-0] [NAME...]
//
// With no NAME, prints the exported environment as KEY=VALUE pairs
// sorted alphabetically. With one or more NAMEs, prints each named
// variable's value (one per line) and exits 1 if any name is unset.
package printenv

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "printenv [-0] [NAME...]"

const helpText = `Usage: printenv [OPTIONS] [NAME...]
Print the values of the named environment variables, or the entire
environment when no names are given.

Options:
  -0, --null      separate entries with NUL instead of newline
      --help      show this help and exit
`

// New returns the printenv command.
func New() command.Command { return command.Define("printenv", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	nullSep := false
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
		case a == "-0" || a == "--null":
			nullSep = true
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	names := args[i:]
	base := c.Env
	if len(base) == 0 {
		base = c.ExportedEnv
	}

	sep := "\n"
	if nullSep {
		sep = "\x00"
	}

	if len(names) == 0 {
		keys := make([]string, 0, len(base))
		for k := range base {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if c.Stdout != nil {
			for _, k := range keys {
				_, _ = fmt.Fprintf(c.Stdout, "%s=%s%s", k, base[k], sep)
			}
		}
		return command.Result{ExitCode: 0}
	}

	exit := 0
	for _, n := range names {
		v, ok := base[n]
		if !ok {
			exit = 1
			continue
		}
		if c.Stdout != nil {
			_, _ = io.WriteString(c.Stdout, v)
			_, _ = io.WriteString(c.Stdout, sep)
		}
	}
	return command.Result{ExitCode: exit}
}

func init() { command.RegisterBuiltin(New()) }
