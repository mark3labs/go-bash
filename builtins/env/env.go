// Package env implements the `env` built-in.
//
// Usage:
//
//	env [-i] [-u VAR]... [NAME=VALUE]... [COMMAND [ARG]...]
//
// Flags:
//
//	-i             start with an empty environment
//	-u VAR         remove VAR from the environment
//	--help         show this help and exit
//
// With no COMMAND, prints the current environment as KEY=VALUE pairs
// sorted alphabetically. With a COMMAND, runs it under the
// constructed environment via Context.Registry. NAME=VALUE operands
// preceding COMMAND are added to the environment for that
// invocation only.
package env

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "env [-i] [-u VAR]... [NAME=VALUE]... [COMMAND [ARG]...]"

const helpText = `Usage: env [OPTIONS] [NAME=VALUE]... [COMMAND [ARG]...]
Run COMMAND with a modified environment, or print the current
environment when no COMMAND is given.

Options:
  -i              start with an empty environment
  -u VAR          remove VAR from the environment
  --help          show this help and exit
`

// New returns the env command.
func New() command.Command { return command.Define("env", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	clean := false
	var drops []string

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case a == "-i" || a == "--ignore-environment":
			clean = true
		case a == "-u" || a == "--unset":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			drops = append(drops, args[i])
		case strings.HasPrefix(a, "--unset="):
			drops = append(drops, strings.TrimPrefix(a, "--unset="))
		case strings.HasPrefix(a, "-") && len(a) > 1 && !strings.Contains(a, "="):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	// Build the env from c.Env (per-call HandlerContext view —
	// includes any `KEY=VAL prog` prefix mvdan/sh propagated) or
	// fall back to ExportedEnv if Env is empty.
	base := c.Env
	if len(base) == 0 {
		base = c.ExportedEnv
	}
	envMap := make(map[string]string, len(base))
	if !clean {
		for k, v := range base {
			envMap[k] = v
		}
	}
	for _, d := range drops {
		delete(envMap, d)
	}

	// Walk remaining operands; KEY=VAL adds to env, first non-K=V is
	// the command name (and the rest are its argv).
	rest := args[i:]
	cmdStart := -1
	for j, a := range rest {
		if eq := strings.IndexByte(a, '='); eq > 0 && isIdent(a[:eq]) {
			envMap[a[:eq]] = a[eq+1:]
			continue
		}
		cmdStart = j
		break
	}

	if cmdStart < 0 {
		// No command — print the env, sorted alphabetically.
		keys := make([]string, 0, len(envMap))
		for k := range envMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if c.Stdout != nil {
			for _, k := range keys {
				_, _ = fmt.Fprintf(c.Stdout, "%s=%s\n", k, envMap[k])
			}
		}
		return command.Result{ExitCode: 0}
	}

	// Run the command. Look it up in c.Registry. We don't have a
	// generic "run a command line with this env" hook in Phase 10
	// (Context.Exec runs a script, not a single command), so we
	// dispatch directly via the registry with a synthesized Context.
	cmdArgs := rest[cmdStart:]
	if len(cmdArgs) == 0 {
		return command.Result{ExitCode: 0}
	}
	name := cmdArgs[0]
	if c.Registry == nil {
		return builtinutil.Errorf(c.Stderr, "env", 127, "%s: command not found", name)
	}
	cmd, ok := c.Registry.Lookup(name)
	if !ok {
		// Try basename for /bin/X dispatch.
		if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
			cmd, ok = c.Registry.Lookup(name[idx+1:])
		}
	}
	if !ok {
		return builtinutil.Errorf(c.Stderr, "env", 127, "%s: command not found", name)
	}
	child := &command.Context{
		FS:          c.FS,
		Cwd:         c.Cwd,
		Env:         envMap,
		Stdin:       c.Stdin,
		Stdout:      c.Stdout,
		Stderr:      c.Stderr,
		Registry:    c.Registry,
		Fetch:       c.Fetch,
		Sleep:       c.Sleep,
		Trace:       c.Trace,
		Limits:      c.Limits,
		ExportedEnv: envMap,
		Aliases:     c.Aliases,
		History:     c.History,
		Exec:        c.Exec,
	}
	res := cmd.Execute(ctx, cmdArgs, child)
	if res.Stdout != "" && c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, res.Stdout)
	}
	if res.Stderr != "" && c.Stderr != nil {
		_, _ = io.WriteString(c.Stderr, res.Stderr)
	}
	return command.Result{ExitCode: res.ExitCode}
}

// isIdent reports whether s is a valid POSIX env variable name:
// starts with letter or underscore, rest are letters/digits/underscore.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func init() { command.RegisterBuiltin(New()) }
