// Package ln implements the `ln` built-in (SPEC §10 Wave B).
//
// Flags: -s (symbolic), -f (force remove existing target).
package ln

import (
	"context"
	"path"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "ln [-sf] TARGET [LINK_NAME]"
const helpText = `Usage: ln [OPTION]... TARGET LINK_NAME
Create a link to TARGET with the name LINK_NAME.

  -s, --symbolic   make symbolic links instead of hard links
  -f, --force      remove existing destination files`

// New returns the ln command.
func New() command.Command { return command.Define("ln", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	symbolic := false
	force := false
	var positional []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-s", a == "--symbolic":
			symbolic = true
		case a == "-f", a == "--force":
			force = true
		case a == "-sf", a == "-fs":
			symbolic, force = true, true
		case a == "--":
			i++
			positional = append(positional, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			positional = append(positional, a)
		}
	}
run:
	if len(positional) < 1 || len(positional) > 2 {
		return builtinutil.Errorf(c.Stderr, "ln", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "ln", 1, "no filesystem")
	}
	target := positional[0]
	var link string
	if len(positional) == 2 {
		link = builtinutil.ResolvePath(c.Cwd, positional[1])
	} else {
		link = builtinutil.ResolvePath(c.Cwd, path.Base(target))
	}
	if force {
		_ = c.FS.Remove(link)
	}
	if symbolic {
		if err := c.FS.Symlink(target, link); err != nil {
			return builtinutil.Errorf(c.Stderr, "ln", 1, "failed to create symbolic link: %v", err)
		}
		return command.Result{}
	}
	// Hard link.
	srcAbs := builtinutil.ResolvePath(c.Cwd, target)
	if err := c.FS.Link(srcAbs, link); err != nil {
		return builtinutil.Errorf(c.Stderr, "ln", 1, "failed to create hard link: %v", err)
	}
	return command.Result{}
}

func init() { command.RegisterBuiltin(New()) }
