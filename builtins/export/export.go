// Package export implements the `export` shell built-in (SPEC §11).
// mvdan/sh handles `export` as a *syntax.DeclClause at the parser
// level, so the bare command word is intercepted before reaching our
// registry. The registered version here is reachable via /bin/export
// and mutates c.Env / c.ExportedEnv as a best-effort diagnostic
// surface (mutations do NOT propagate to the runner). DECISIONS.md
// documents the shadow.
package export

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "export [-p] [-n] [NAME[=VALUE]]..."

// New returns the export command.
func New() command.Command { return command.Define("export", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	printAll := false
	unsetExport := false
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
		case a == "-p":
			printAll = true
		case a == "-n":
			unsetExport = true
		case strings.HasPrefix(a, "-") && len(a) > 1 && !strings.Contains(a, "="):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	operands := args[i:]
	if printAll || (len(operands) == 0 && !unsetExport) {
		if c.Stdout != nil && c.ExportedEnv != nil {
			for _, k := range sortedKeys(c.ExportedEnv) {
				_, _ = fmt.Fprintf(c.Stdout, "declare -x %s=\"%s\"\n", k, c.ExportedEnv[k])
			}
		}
		return command.Result{ExitCode: 0}
	}
	for _, op := range operands {
		name, val, hasVal := splitNameValue(op)
		if name == "" {
			_ = builtinutil.Errorf(c.Stderr, "export", 1, "`%s': not a valid identifier", op)
			continue
		}
		if unsetExport {
			if c.ExportedEnv != nil {
				delete(c.ExportedEnv, name)
			}
			continue
		}
		if hasVal {
			if c.Env != nil {
				c.Env[name] = val
			}
			if c.ExportedEnv != nil {
				c.ExportedEnv[name] = val
			}
			continue
		}
		// promote existing var
		if c.Env != nil && c.ExportedEnv != nil {
			if v, ok := c.Env[name]; ok {
				c.ExportedEnv[name] = v
			}
		}
	}
	return command.Result{ExitCode: 0}
}

func splitNameValue(s string) (name, val string, hasVal bool) {
	eq := strings.IndexByte(s, '=')
	if eq < 0 {
		return s, "", false
	}
	return s[:eq], s[eq+1:], true
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// quick insertion sort (small N typical)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func init() { command.RegisterBuiltin(New()) }
