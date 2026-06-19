// Package readlink implements the `readlink` built-in (SPEC §10 Wave B).
//
// Flags: -f, -e, -m (canonicalize variants), -n (no trailing newline).
package readlink

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"io"
	"path"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "readlink [-fenm] FILE..."
const helpText = `Usage: readlink [OPTION]... FILE...
Print the value of a symbolic link or canonical file name.

  -f, --canonicalize             canonicalize; every comp must exist except last
  -e, --canonicalize-existing    canonicalize; every comp must exist
  -m, --canonicalize-missing     canonicalize; no comp need exist
  -n, --no-newline               do not output the trailing newline`

// New returns the readlink command.
func New() command.Command { return command.Define("readlink", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	mode := "" // "f", "e", "m", or "" for plain readlink
	noNewline := false
	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-f", a == "--canonicalize":
			mode = "f"
		case a == "-e", a == "--canonicalize-existing":
			mode = "e"
		case a == "-m", a == "--canonicalize-missing":
			mode = "m"
		case a == "-n", a == "--no-newline":
			noNewline = true
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
		return builtinutil.Errorf(c.Stderr, "readlink", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "readlink", 1, "no filesystem")
	}
	exit := 0
	for _, p := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, p)
		var out string
		var err error
		switch mode {
		case "":
			out, err = c.FS.Readlink(abs)
		case "e":
			out, err = c.FS.Realpath(abs)
		case "f":
			out, err = canonicalize(c, abs, true)
		case "m":
			out, err = canonicalize(c, abs, false)
		}
		if err != nil {
			exit = 1
			continue
		}
		if c.Stdout != nil {
			_, _ = io.WriteString(c.Stdout, out)
			if !noNewline {
				_, _ = fmt.Fprintln(c.Stdout)
			}
		}
	}
	return command.Result{ExitCode: exit}
}

// canonicalize resolves all components; if requireParent is true the
// parent dir must exist (matches `readlink -f`); if false the path
// may be entirely missing (matches `readlink -m`).
func canonicalize(c *command.Context, p string, requireParent bool) (string, error) {
	if r, err := c.FS.Realpath(p); err == nil {
		return r, nil
	}
	// Try realpath of parent + base.
	parent := path.Dir(p)
	if parent == "" || parent == "." {
		parent = "/"
	}
	pr, err := c.FS.Realpath(parent)
	if err != nil {
		if requireParent && !errors.Is(err, iofs.ErrNotExist) {
			return "", err
		}
		if !requireParent {
			return p, nil
		}
		return "", err
	}
	return path.Join(pr, path.Base(p)), nil
}

func init() { command.RegisterBuiltin(New()) }
