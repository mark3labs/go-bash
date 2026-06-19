// Package readonly implements the `readonly` shell built-in.
// mvdan/sh handles this via *syntax.DeclClause. The registered version
// reachable via /bin/readonly is a best-effort surface that records
// assignments into c.Env (the "readonly" attribute is NOT tracked —
// later assignments via the same channel will overwrite silently).
package readonly

import (
	"context"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "readonly [-aAfp] [NAME[=VALUE]]..."

// New returns the readonly command.
func New() command.Command { return command.Define("readonly", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, op := range args[1:] {
		switch {
		case op == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case strings.HasPrefix(op, "-"):
			// recognized flags: -a -A -f -p — no-op
		default:
			eq := strings.IndexByte(op, '=')
			if eq >= 0 && c.Env != nil {
				c.Env[op[:eq]] = op[eq+1:]
			}
		}
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
