// Package shopt implements the `shopt` shell built-in.
// Reads and writes per-Bash shell options via c.Shopt. mvdan/sh ships
// its own; this registration is reachable via /bin/shopt and is the
// canonical surface for toggling `expand_aliases` (which the
// alias-expansion pass in interp/runner consults at parse time).
package shopt

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "shopt [-s|-u|-p|-q] [OPTNAME...]"

// New returns the shopt command.
func New() command.Command { return command.Define("shopt", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	mode := 'p' // print
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
		case a == "-s":
			mode = 's'
		case a == "-u":
			mode = 'u'
		case a == "-p":
			mode = 'p'
		case a == "-q":
			mode = 'q'
		case strings.HasPrefix(a, "-"):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	names := args[i:]
	if c.Shopt == nil {
		if mode == 'q' {
			return command.Result{ExitCode: 1}
		}
		return command.Result{ExitCode: 0}
	}
	switch mode {
	case 's':
		for _, n := range names {
			c.Shopt.Set(n, true)
		}
	case 'u':
		for _, n := range names {
			c.Shopt.Set(n, false)
		}
	case 'q':
		for _, n := range names {
			if !c.Shopt.IsSet(n) {
				return command.Result{ExitCode: 1}
			}
		}
		return command.Result{ExitCode: 0}
	case 'p':
		if len(names) == 0 {
			names = c.Shopt.Names()
		}
		if c.Stdout != nil {
			for _, n := range names {
				state := "off"
				if c.Shopt.IsSet(n) {
					state = "on"
				}
				_, _ = fmt.Fprintf(c.Stdout, "%s\t%s\n", n, state)
			}
		}
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
