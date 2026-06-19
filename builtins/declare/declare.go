// Package declare implements the `declare` shell built-in (SPEC §11).
// `declare` doubles as `typeset`. mvdan/sh ships its own
// `declare`/`local`/`readonly`/`export` via *syntax.DeclClause; the
// registered version reachable via /bin/declare mutates c.Env as a
// best-effort surface only.
package declare

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "declare [-aAirxnp] [NAME[=VALUE]]..."

// New returns the declare command.
func New() command.Command { return command.Define("declare", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	print := false
	export := false
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for _, ch := range a[1:] {
				switch ch {
				case 'p':
					print = true
				case 'x':
					export = true
				case 'a', 'A', 'i', 'r', 'n':
					// recognized, no-op
				default:
					return builtinutil.UsageError(c.Stderr, usage)
				}
			}
		default:
			goto done
		}
	}
done:
	operands := args[i:]
	if print || len(operands) == 0 {
		if c.Stdout != nil && c.Env != nil {
			for _, k := range sortedKeys(c.Env) {
				_, _ = fmt.Fprintf(c.Stdout, "declare -- %s=\"%s\"\n", k, c.Env[k])
			}
		}
		return command.Result{ExitCode: 0}
	}
	for _, op := range operands {
		eq := strings.IndexByte(op, '=')
		var name, val string
		hasVal := eq >= 0
		if hasVal {
			name, val = op[:eq], op[eq+1:]
		} else {
			name = op
		}
		if c.Env != nil {
			if hasVal {
				c.Env[name] = val
			} else if _, ok := c.Env[name]; !ok {
				c.Env[name] = ""
			}
		}
		if export && c.ExportedEnv != nil {
			if v, ok := c.Env[name]; ok {
				c.ExportedEnv[name] = v
			}
		}
	}
	return command.Result{ExitCode: 0}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func init() { command.RegisterBuiltin(New()) }
