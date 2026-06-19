// Package mkdir implements the `mkdir` built-in.
//
// Flags: -p (parents, no-error-if-exists), -m MODE.
package mkdir

import (
	"context"
	"os"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "mkdir [-p] [-m MODE] DIRECTORY..."
const helpText = `Usage: mkdir [OPTION]... DIRECTORY...
Create the DIRECTORY(ies), if they do not already exist.

  -p, --parents     no error if existing, make parent directories as needed
  -m, --mode=MODE   set file mode (as in chmod), not a=rwx - umask`

// New returns the mkdir command.
func New() command.Command {
	return command.Define("mkdir", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	parents := false
	mode := os.FileMode(0o755)
	modeSet := false
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
		case a == "-m", a == "--mode":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			m, err := builtinutil.ParseChmodMode(args[i], 0o755, true)
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "mkdir", 1, "invalid mode: %s", args[i])
			}
			mode = m
			modeSet = true
		case strings.HasPrefix(a, "--mode="):
			m, err := builtinutil.ParseChmodMode(strings.TrimPrefix(a, "--mode="), 0o755, true)
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "mkdir", 1, "invalid mode: %s", a[7:])
			}
			mode = m
			modeSet = true
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
		return builtinutil.Errorf(c.Stderr, "mkdir", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "mkdir", 1, "no filesystem")
	}
	exit := 0
	for _, p := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, p)
		var err error
		if parents {
			err = c.FS.MkdirAll(abs, mode)
		} else {
			err = c.FS.Mkdir(abs, mode)
		}
		if err != nil {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "mkdir", 1, "cannot create directory %q: %v", p, err)
			continue
		}
		if modeSet {
			_ = c.FS.Chmod(abs, mode)
		}
	}
	return command.Result{ExitCode: exit}
}

func init() { command.RegisterBuiltin(New()) }
