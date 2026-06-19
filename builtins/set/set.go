// Package set implements the `set` shell built-in (SPEC §11). The
// command tweaks shell-option flags (-e, -u, -x, -o pipefail, etc.)
// and positional parameters. mvdan/sh implements `set` natively; the
// registered version here is reachable via /bin/set as a diagnostic
// surface (option flags are recorded into c.Shopt under
// "set:<flag>" so tests can observe them; positional-parameter
// mutation is NOT propagated to the runner).
package set

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "set [-eEuxo OPT] [+eEuxo OPT] [--] [ARG...]"

// New returns the set command.
func New() command.Command { return command.Define("set", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	// No args: list every env var as "K=V" sorted.
	if len(args) == 1 {
		if c.Stdout != nil && c.Env != nil {
			for _, k := range sortedKeys(c.Env) {
				_, _ = fmt.Fprintf(c.Stdout, "%s=%s\n", k, c.Env[k])
			}
		}
		return command.Result{ExitCode: 0}
	}
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case a == "--":
			// positional-parameter clear/set; we don't propagate.
			return command.Result{ExitCode: 0}
		case a == "-o" || a == "+o":
			if i+1 < len(args) {
				name := args[i+1]
				on := a == "-o"
				if c.Shopt != nil {
					c.Shopt.Set("set:o:"+name, on)
				}
				i++
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for _, ch := range a[1:] {
				if c.Shopt != nil {
					c.Shopt.Set("set:"+string(ch), true)
				}
			}
		case strings.HasPrefix(a, "+") && len(a) > 1:
			for _, ch := range a[1:] {
				if c.Shopt != nil {
					c.Shopt.Set("set:"+string(ch), false)
				}
			}
		default:
			// positional-parameter set; we don't propagate.
			return command.Result{ExitCode: 0}
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
