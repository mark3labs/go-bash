// Package exit implements the `exit` shell built-in (SPEC §11).
// `exit [N]` sets the script's exit status to N (default 0). mvdan/sh
// implements exit as a built-in that unwinds the runner; the
// registered version here is reachable via /bin/exit and reports the
// requested code through command.Result without unwinding mvdan's
// runner state (the dispatcher translates ExitCode into ExitStatus).
package exit

import (
	"context"
	"strconv"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "exit [N]"

// New returns the exit command.
func New() command.Command { return command.Define("exit", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	if len(args) >= 2 {
		if args[1] == "--help" {
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "exit", 2, "%s: numeric argument required", args[1])
		}
		return command.Result{ExitCode: n & 0xff}
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
