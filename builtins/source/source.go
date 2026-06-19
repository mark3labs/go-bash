// Package source implements the `source` and `.` shell built-ins.
// Reads a script file from c.FS and runs it via c.Exec.
// Bumps c.SourceDepth + 1 into the SubExecOptions so MaxSourceDepth
// trips cleanly across nested invocations.
//
// mvdan/sh ships its own; reachable via /bin/source or /bin/. — but
// The spec specifically lists `source`/`.` as the canonical depth
// counters, so we route them through c.Exec.
package source

import (
	"context"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "source FILE [ARG...]"

// Run is the shared implementation used by both `source` and `.`.
func Run(ctx context.Context, args []string, c *command.Context) command.Result {
	if len(args) < 2 {
		return builtinutil.Errorf(c.Stderr, "source", 2, "filename argument required")
	}
	first := args[1]
	if first == "--help" {
		builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
		return command.Result{ExitCode: 0}
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "source", 1, "no filesystem")
	}
	resolved := builtinutil.ResolvePath(c.Cwd, first)
	data, err := c.FS.ReadFile(resolved)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "source", 1, "%v", err)
	}
	if c.Exec == nil {
		return builtinutil.Errorf(c.Stderr, "source", 1, "exec hook not available")
	}
	depth := c.SourceDepth
	if max := c.Limits.MaxSourceDepth; max > 0 && depth+1 > max {
		return builtinutil.Errorf(c.Stderr, "source", 1, "MaxSourceDepth (%d) exceeded", max)
	}
	res, err := c.Exec(ctx, string(data), command.SubExecOptions{
		Stdin:       c.Stdin,
		Stdout:      c.Stdout,
		Stderr:      c.Stderr,
		Env:         c.Env,
		ReplaceEnv:  true,
		Cwd:         c.Cwd,
		Args:        args[2:],
		SourceDepth: depth + 1,
	})
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "source", 1, "%v", err)
	}
	return command.Result{ExitCode: res.ExitCode}
}

// NewSource returns the `source` command.
func NewSource() command.Command { return command.Define("source", Run) }

// NewDot returns the `.` command.
func NewDot() command.Command { return command.Define(".", Run) }

func init() {
	command.RegisterBuiltin(NewSource())
	command.RegisterBuiltin(NewDot())
}
