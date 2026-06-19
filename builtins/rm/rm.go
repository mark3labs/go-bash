// Package rm implements the `rm` built-in (SPEC §10 Wave B).
//
// Flags: -r/-R (recursive), -f (force), -i (interactive — REJECTED in
// sandbox), -d (remove empty directories).
package rm

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "rm [-rRfdi] FILE..."
const helpText = `Usage: rm [OPTION]... FILE...
Remove (unlink) the FILE(s).

  -f, --force           ignore nonexistent files and arguments, never prompt
  -i                    interactive (REJECTED in sandbox)
  -r, -R, --recursive   remove directories and their contents recursively
  -d, --dir             remove empty directories`

// New returns the rm command.
func New() command.Command { return command.Define("rm", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	recursive := false
	force := false
	dirEmpty := false

	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-r", a == "-R", a == "--recursive":
			recursive = true
		case a == "-f", a == "--force":
			force = true
		case a == "-i":
			if c.Stderr != nil {
				_, _ = fmt.Fprintln(c.Stderr, "rm: interactive mode not supported in sandbox")
			}
			return command.Result{ExitCode: 1}
		case a == "-d", a == "--dir":
			dirEmpty = true
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			// Allow combined short options like -rf, -fr, -rd.
			if combinedShortOptsRm(a, &recursive, &force, &dirEmpty) {
				continue
			}
			return builtinutil.UsageError(c.Stderr, usage)
		case a == "--":
			i++
			paths = append(paths, args[i:]...)
			goto run
		default:
			paths = append(paths, a)
		}
	}
run:
	if len(paths) == 0 {
		if force {
			return command.Result{ExitCode: 0}
		}
		return builtinutil.Errorf(c.Stderr, "rm", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "rm", 1, "no filesystem")
	}
	exit := 0
	for _, p := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, p)
		fi, err := c.FS.Lstat(abs)
		if err != nil {
			if force && errors.Is(err, iofs.ErrNotExist) {
				continue
			}
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "rm", 1, "cannot remove %q: %v", p, err)
			continue
		}
		if fi.IsDir() {
			if recursive {
				if err := c.FS.RemoveAll(abs); err != nil {
					exit = 1
					_ = builtinutil.Errorf(c.Stderr, "rm", 1, "cannot remove %q: %v", p, err)
				}
				continue
			}
			if dirEmpty {
				if err := c.FS.Remove(abs); err != nil {
					exit = 1
					_ = builtinutil.Errorf(c.Stderr, "rm", 1, "cannot remove %q: %v", p, err)
				}
				continue
			}
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "rm", 1, "cannot remove %q: is a directory", p)
			continue
		}
		if err := c.FS.Remove(abs); err != nil {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "rm", 1, "cannot remove %q: %v", p, err)
		}
	}
	return command.Result{ExitCode: exit}
}

func combinedShortOptsRm(a string, recursive, force, dirEmpty *bool) bool {
	// Bundle of short flags: must start with - and contain only r/R/f/d.
	if !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
		return false
	}
	for _, c := range a[1:] {
		switch c {
		case 'r', 'R':
			*recursive = true
		case 'f':
			*force = true
		case 'd':
			*dirEmpty = true
		default:
			return false
		}
	}
	return true
}

func init() { command.RegisterBuiltin(New()) }
