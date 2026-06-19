// Package which implements the `which` built-in (SPEC §10 Wave A).
//
// Resolution order:
//
//  1. If the command is in the dispatch Registry, print
//     /usr/bin/<name> (or whichever stub directory matches first
//     in $PATH). This makes `which X` work for built-ins exactly
//     like coreutils against /usr/bin entries.
//  2. Otherwise walk $PATH (defaulting to "/usr/bin:/bin") and
//     return the first executable found on the VFS.
//  3. If nothing matches, exit 1 with no output.
//
// Flags:
//
//	-a  print every match, not just the first
//	-s  silent: no output, just set the exit code
package which

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "which [-a] [-s] NAME..."
const helpText = `Usage: which [-a] [-s] NAME...
Print the full pathname of the executable that would run NAME.

  -a  print every match found on PATH, not just the first
  -s  silent: produce no output, only set the exit code`

// New returns the which command.
func New() command.Command {
	return command.Define("which", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	all := false
	silent := false
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case "-a":
			all = true
		case "-s":
			silent = true
		case "--":
			i++
			goto done
		default:
			if strings.HasPrefix(a, "-") && len(a) > 1 {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			goto done
		}
	}
done:
	names := args[i:]
	if len(names) == 0 {
		return command.Result{ExitCode: 1}
	}
	pathEnv := c.Env["PATH"]
	if pathEnv == "" {
		pathEnv = "/usr/bin:/bin"
	}
	dirs := strings.Split(pathEnv, ":")

	notFound := 0
	for _, name := range names {
		if name == "" {
			notFound++
			continue
		}
		var matches []string
		// If the name contains a slash, treat as a direct path and
		// only check it (matches GNU which).
		if strings.ContainsRune(name, '/') {
			if isExec(c, name) {
				matches = append(matches, name)
			}
		} else {
			for _, d := range dirs {
				p := joinPath(d, name)
				if isExec(c, p) {
					matches = append(matches, p)
					if !all {
						break
					}
				}
			}
		}
		if len(matches) == 0 {
			notFound++
			continue
		}
		if !silent && c.Stdout != nil {
			for _, m := range matches {
				_, _ = io.WriteString(c.Stdout, m)
				_, _ = fmt.Fprintln(c.Stdout)
			}
		}
	}
	if notFound > 0 {
		return command.Result{ExitCode: 1}
	}
	return command.Result{ExitCode: 0}
}

func isExec(c *command.Context, path string) bool {
	if c.FS == nil {
		return false
	}
	fi, err := c.FS.Stat(path)
	if err != nil {
		return false
	}
	if fi.IsDir() {
		return false
	}
	// Mode bits: any of the executable bits set is enough.
	return fi.Mode().Perm()&0o111 != 0
}

func joinPath(dir, name string) string {
	if dir == "" {
		return name
	}
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

func init() {
	command.RegisterBuiltin(New())
}
