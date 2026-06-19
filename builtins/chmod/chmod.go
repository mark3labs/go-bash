// Package chmod implements the `chmod` built-in.
//
// Modes: numeric "755" or symbolic "u+x,g-w,o=r". Flags: -R recursive,
// -v verbose, -c (changes-only verbose).
package chmod

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "chmod [-Rvc] MODE FILE..."
const helpText = `Usage: chmod [OPTION]... MODE[,MODE]... FILE...
Change the mode of each FILE to MODE.

  -R, --recursive    change files and directories recursively
  -v, --verbose      output a diagnostic for every file processed
  -c, --changes      like verbose but report only when a change is made`

// New returns the chmod command.
func New() command.Command { return command.Define("chmod", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	recursive := false
	verbose := false
	changesOnly := false
	var positional []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-R", a == "--recursive":
			recursive = true
		case a == "-v", a == "--verbose":
			verbose = true
		case a == "-c", a == "--changes":
			changesOnly = true
		case a == "--":
			i++
			positional = append(positional, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && !isModeArg(a):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			positional = append(positional, a)
		}
	}
run:
	if len(positional) < 2 {
		return builtinutil.Errorf(c.Stderr, "chmod", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "chmod", 1, "no filesystem")
	}
	modeSpec := positional[0]
	files := positional[1:]
	exit := 0
	for _, f := range files {
		abs := builtinutil.ResolvePath(c.Cwd, f)
		if err := apply(c, abs, modeSpec, recursive, verbose, changesOnly); err != nil {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "chmod", 1, "%s: %v", f, err)
		}
	}
	return command.Result{ExitCode: exit}
}

// isModeArg recognizes "-r..." style symbolic modes vs flags. Mode
// args can start with "-", e.g. "-w" to clear write bits, but chmod
// uses "+r" / "u-x" forms more commonly. We treat anything that has
// already passed our flag switch and that looks like a mode (contains
// + or =, or starts with digit) as a mode arg, not a flag.
func isModeArg(a string) bool {
	if a == "" {
		return false
	}
	// Pure octal like "0755" is rare to see here because the switch
	// above only fires on strings starting with "-".
	for _, c := range a[1:] {
		if c == '+' || c == '=' || c == 'r' || c == 'w' || c == 'x' || c == 'X' || c == 's' || c == 't' || c == 'u' || c == 'g' || c == 'o' || c == 'a' {
			return true
		}
	}
	return false
}

func apply(c *command.Context, p, spec string, recursive, verbose, changesOnly bool) error {
	fi, err := c.FS.Lstat(p)
	if err != nil {
		return err
	}
	newMode, err := builtinutil.ParseChmodMode(spec, fi.Mode().Perm(), fi.IsDir())
	if err != nil {
		return err
	}
	changed := newMode != fi.Mode().Perm()
	if err := c.FS.Chmod(p, newMode); err != nil {
		return err
	}
	if verbose || (changesOnly && changed) {
		if c.Stdout != nil {
			if changed {
				_, _ = fmt.Fprintf(c.Stdout, "mode of %q changed from %04o to %04o\n", p, fi.Mode().Perm(), newMode)
			} else {
				_, _ = fmt.Fprintf(c.Stdout, "mode of %q retained as %04o\n", p, newMode)
			}
		}
	}
	if recursive && fi.IsDir() {
		entries, err := c.FS.ReadDir(p)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := apply(c, path.Join(p, e.Name()), spec, recursive, verbose, changesOnly); err != nil {
				return err
			}
		}
	}
	return nil
}

func init() { command.RegisterBuiltin(New()) }
