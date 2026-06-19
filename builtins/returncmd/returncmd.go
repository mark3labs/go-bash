// Package returncmd implements the `return` shell built-in (SPEC §11).
// `return [N]` sets the exit code; mvdan/sh ships its own builtin so
// this is normally reachable via /bin/return.
package returncmd

import (
	"context"
	"strconv"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "return [N]"

// New returns the return command.
func New() command.Command { return command.Define("return", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	if len(args) >= 2 {
		if args[1] == "--help" {
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "return", 2, "%s: numeric argument required", args[1])
		}
		return command.Result{ExitCode: n & 0xff}
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
