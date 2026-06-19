// Package pushd implements the `pushd` shell built-in (SPEC §11).
// Stub: validates the target exists via c.FS.Stat, prints "TARGET\n",
// exits 0. mvdan/sh ships its own; reachable via /bin/pushd.
package pushd

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "pushd [+N | -N | DIR]"

// New returns the pushd command.
func New() command.Command { return command.Define("pushd", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	if len(args) < 2 {
		return builtinutil.Errorf(c.Stderr, "pushd", 1, "no other directory")
	}
	a := args[1]
	if a == "--help" {
		builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
		return command.Result{ExitCode: 0}
	}
	if strings.HasPrefix(a, "+") || strings.HasPrefix(a, "-") {
		// stack rotation flag — we have no stack, just exit 0.
		return command.Result{ExitCode: 0}
	}
	target := a
	if !strings.HasPrefix(target, "/") {
		target = path.Join(c.Cwd, target)
	}
	if c.FS != nil {
		if _, err := c.FS.Stat(target); err != nil {
			return builtinutil.Errorf(c.Stderr, "pushd", 1, "%s: %v", a, err)
		}
	}
	if c.Stdout != nil {
		_, _ = fmt.Fprintln(c.Stdout, target)
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
