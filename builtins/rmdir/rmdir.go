// Package rmdir implements the `rmdir` built-in (SPEC §10 Wave B).
//
// Flags: -p (also remove parent dirs if empty).
package rmdir

import (
	"context"
	"strings"

	gbfs "github.com/mark3labs/go-bash/fs"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "rmdir [-p] DIRECTORY..."
const helpText = `Usage: rmdir [OPTION]... DIRECTORY...
Remove the DIRECTORY(ies), if they are empty.

  -p, --parents   remove DIRECTORY and its ancestors, if they become empty`

// New returns the rmdir command.
func New() command.Command { return command.Define("rmdir", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	parents := false
	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-p", a == "--parents":
			parents = true
		case a == "--":
			i++
			paths = append(paths, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			paths = append(paths, a)
		}
	}
run:
	if len(paths) == 0 {
		return builtinutil.Errorf(c.Stderr, "rmdir", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "rmdir", 1, "no filesystem")
	}
	exit := 0
	for _, p := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, p)
		if err := c.FS.Remove(abs); err != nil {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "rmdir", 1, "failed to remove %q: %v", p, err)
			continue
		}
		if parents {
			for {
				abs = gbfs.Dirname(abs)
				if abs == "/" || abs == "." {
					break
				}
				if err := c.FS.Remove(abs); err != nil {
					break
				}
			}
		}
	}
	return command.Result{ExitCode: exit}
}

func init() { command.RegisterBuiltin(New()) }
