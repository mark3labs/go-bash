// Package cd implements the `cd` shell built-in. Real bash
// semantics: change the working directory; mvdan/sh ships its own `cd`
// that mutates the runner's Dir. Our registration is reachable via
// /bin/cd and CANNOT mutate the runner's Dir (it has no back-channel),
// so it functions as a path-validation + diagnostic stub: it resolves
// the target via c.FS.Stat and emits a clean error if missing.
//
// Documented shadow in DECISIONS.md (Phase 11).
package cd

import (
	"context"
	"path"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "cd [-L|-P] [DIR]"

// New returns the cd command.
func New() command.Command { return command.Define("cd", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	dir := ""
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case a == "-L" || a == "-P":
			// accepted, no-op
		case a == "--":
			i++
			goto done
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	if i < len(args) {
		dir = args[i]
	}
	if dir == "" {
		dir = c.Env["HOME"]
		if dir == "" {
			return builtinutil.Errorf(c.Stderr, "cd", 1, "HOME not set")
		}
	}
	if dir == "-" {
		if old := c.Env["OLDPWD"]; old != "" {
			dir = old
		} else {
			return builtinutil.Errorf(c.Stderr, "cd", 1, "OLDPWD not set")
		}
	}
	if !strings.HasPrefix(dir, "/") {
		cwd := c.Cwd
		if cwd == "" {
			cwd = "/"
		}
		dir = path.Join(cwd, dir)
	}
	if c.FS != nil {
		info, err := c.FS.Stat(dir)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "cd", 1, "%s: %v", args[len(args)-1], err)
		}
		if !info.IsDir() {
			return builtinutil.Errorf(c.Stderr, "cd", 1, "%s: not a directory", args[len(args)-1])
		}
	}
	// Best-effort mutation: c.Env update is in-place but not
	// propagated to mvdan/sh's runner.
	if c.Env != nil {
		c.Env["OLDPWD"] = c.Cwd
		c.Env["PWD"] = dir
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
