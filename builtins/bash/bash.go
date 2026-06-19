// Package bash implements the `bash` built-in.
//
// Usage:
//
//	bash [OPTIONS] [SCRIPT [ARG...]]
//
// Options:
//
//	-c SCRIPT       run SCRIPT (passed inline)
//	-e              exit on error (set -e in the sub-shell)
//	-x              trace commands (set -x in the sub-shell)
//	-n              parse only — read and check syntax, do not run
//	-s              read script from stdin
//	--              end of options
//	--help          show this help and exit
//
// Recursive invocations are tracked via the GOBASH_BASH_DEPTH env
// variable and bounded by Context.Limits.MaxSourceDepth. The
// child runs through Context.Exec, which routes back into the same
// *Bash runtime (sharing FS, registry, network, etc.).
package bash

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
	"github.com/mark3labs/go-bash/parser"
)

const usage = "bash [-c SCRIPT] [-e] [-x] [-n] [-s] [--] [SCRIPT [ARG...]]"

// Mode selects which command name (`bash` or `sh`) we are
// implementing. The flag surface is identical; only the registered
// name and helper text differ.
type Mode int

const (
	ModeBash Mode = iota
	ModeSh
)

// Run is the shared implementation used by both the `bash` and `sh`
// built-ins.
func Run(ctx context.Context, args []string, c *command.Context, mode Mode) command.Result {
	name := "bash"
	if mode == ModeSh {
		name = "sh"
	}
	help := helpText(name)

	var inlineScript string
	inlineSet := false
	errexit := false
	xtrace := false
	noexec := false
	readStdin := false

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, help)
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case a == "-c":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			inlineScript = args[i]
			inlineSet = true
		case a == "-e":
			errexit = true
		case a == "-x":
			xtrace = true
		case a == "-n":
			noexec = true
		case a == "-s":
			readStdin = true
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// Parse bundled short flags like "-ex".
			ok := true
			for _, ch := range a[1:] {
				switch ch {
				case 'e':
					errexit = true
				case 'x':
					xtrace = true
				case 'n':
					noexec = true
				case 's':
					readStdin = true
				default:
					ok = false
				}
			}
			if !ok {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			goto done
		}
	}
done:
	rest := args[i:]

	// Resolve the script source.
	var script string
	switch {
	case inlineSet:
		script = inlineScript
	case readStdin || len(rest) == 0:
		if c.Stdin != nil {
			b, err := io.ReadAll(c.Stdin)
			if err != nil {
				return builtinutil.Errorf(c.Stderr, name, 1, "read stdin: %v", err)
			}
			script = string(b)
		}
	default:
		// First positional is a script file path.
		if c.FS == nil {
			return builtinutil.Errorf(c.Stderr, name, 1, "no filesystem")
		}
		path := builtinutil.ResolvePath(c.Cwd, rest[0])
		b, err := c.FS.ReadFile(path)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, name, 1, "%v", err)
		}
		script = string(b)
	}

	// -n: parse only, never run.
	if noexec {
		if _, err := parser.Parse(script); err != nil {
			return builtinutil.Errorf(c.Stderr, name, 2, "%v", err)
		}
		return command.Result{ExitCode: 0}
	}

	// Depth check against MaxSourceDepth.
	depth := c.SourceDepth
	maxDepth := c.Limits.MaxSourceDepth
	if maxDepth > 0 && depth+1 > maxDepth {
		return builtinutil.Errorf(c.Stderr, name, 1, "MaxSourceDepth (%d) exceeded", maxDepth)
	}

	if c.Exec == nil {
		return builtinutil.Errorf(c.Stderr, name, 1, "sub-shell exec not available")
	}

	// Prefix opt-in shell modes.
	var prefix []string
	if errexit {
		prefix = append(prefix, "set -e")
	}
	if xtrace {
		prefix = append(prefix, "set -x")
	}
	if len(prefix) > 0 {
		script = strings.Join(prefix, "\n") + "\n" + script
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
		return builtinutil.Errorf(c.Stderr, name, 1, "%v", err)
	}
	return command.Result{ExitCode: res.ExitCode}
}

func helpText(name string) string {
	return fmt.Sprintf(`Usage: %s [OPTIONS] [SCRIPT [ARG...]]
Invoke a sub-shell to run SCRIPT (or stdin).

Options:
  -c SCRIPT       run SCRIPT (passed inline as the next argument)
  -e              exit on error (set -e)
  -x              trace commands (set -x)
  -n              parse only — read and check syntax, do not run
  -s              read script from standard input
  --              end of options
  --help          show this help and exit
`, name)
}

// New returns the bash command (Mode = ModeBash).
func New() command.Command {
	return command.Define("bash", func(ctx context.Context, args []string, c *command.Context) command.Result {
		return Run(ctx, args, c, ModeBash)
	})
}

func init() { command.RegisterBuiltin(New()) }
