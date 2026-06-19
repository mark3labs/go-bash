// Package pwd implements the `pwd` built-in (SPEC §10 Wave A).
//
// Flags:
//   - -L (default): print the logical working directory verbatim.
//   - -P: resolve symlinks via the VFS's Realpath and print the
//     canonical path.
package pwd

import (
	"context"
	"fmt"
	"io"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "pwd [-LP] [--help]"
const helpText = `Usage: pwd [-LP] [--help]
Print the current working directory.

  -L  print the logical working directory (default)
  -P  print the physical directory, resolving symlinks via the VFS`

// New returns the pwd command.
func New() command.Command {
	return command.Define("pwd", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	physical := false
	for _, a := range args[1:] {
		switch a {
		case "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case "-L":
			physical = false
		case "-P":
			physical = true
		default:
			return builtinutil.UsageError(c.Stderr, usage)
		}
	}

	path := c.Cwd
	if physical && c.FS != nil {
		if rp, err := c.FS.Realpath(path); err == nil {
			path = rp
		}
		// Realpath errors fall through to printing the logical
		// path; matches bash's behavior of using the logical path
		// when Realpath can't resolve (e.g. directory doesn't
		// exist on disk but the shell still tracks its $PWD).
	}
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, path)
		_, _ = fmt.Fprintln(c.Stdout)
	}
	return command.Result{ExitCode: 0}
}

func init() {
	command.RegisterBuiltin(New())
}
