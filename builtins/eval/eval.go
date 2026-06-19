// Package eval implements the `eval` shell built-in.
// Concatenates its arguments with spaces and runs the result via
// c.Exec. Bumps c.SourceDepth + 1 into the sub-exec options so
// MaxSourceDepth caps nested invocations.
package eval

import (
	"context"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "eval [ARG...]"

// New returns the eval command.
func New() command.Command { return command.Define("eval", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	if len(args) >= 2 && args[1] == "--help" {
		builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
		return command.Result{ExitCode: 0}
	}
	script := strings.Join(args[1:], " ")
	if script == "" {
		return command.Result{ExitCode: 0}
	}
	if c.Exec == nil {
		return builtinutil.Errorf(c.Stderr, "eval", 1, "exec hook not available")
	}
	depth := c.SourceDepth
	if max := c.Limits.MaxSourceDepth; max > 0 && depth+1 > max {
		return builtinutil.Errorf(c.Stderr, "eval", 1, "MaxSourceDepth (%d) exceeded", max)
	}
	res, err := c.Exec(ctx, script, command.SubExecOptions{
		Stdin:       c.Stdin,
		Stdout:      c.Stdout,
		Stderr:      c.Stderr,
		Env:         c.Env,
		ReplaceEnv:  true,
		Cwd:         c.Cwd,
		SourceDepth: depth + 1,
	})
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "eval", 1, "%v", err)
	}
	return command.Result{ExitCode: res.ExitCode}
}

func init() { command.RegisterBuiltin(New()) }
