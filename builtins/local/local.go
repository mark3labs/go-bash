// Package local implements the `local` shell built-in.
// `local NAME[=VALUE]...` declares a function-local variable. mvdan/sh
// handles this via *syntax.DeclClause when used inside a function. Our
// registration is reachable via /bin/local and rejects use outside a
// function (we cannot detect function context here, so we just allow
// it as a best-effort assignment).
package local

import (
	"context"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "local [NAME[=VALUE]]..."

// New returns the local command.
func New() command.Command { return command.Define("local", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, op := range args[1:] {
		if op == "--help" {
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		}
		eq := strings.IndexByte(op, '=')
		if eq >= 0 && c.Env != nil {
			c.Env[op[:eq]] = op[eq+1:]
		}
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
