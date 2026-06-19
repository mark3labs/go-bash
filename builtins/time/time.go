// Package time implements the `time` built-in (SPEC §10 Wave G).
//
// Usage:
//
//	time COMMAND [ARG]...
//
// Runs COMMAND through Context.Exec and prints
// `real\t<duration>` to stderr after it finishes. Returns COMMAND's
// exit status. Unlike GNU /usr/bin/time (which is far richer), the
// Wave G surface is intentionally minimal — `-p`, `-f`, `-o` may be
// added in Phase 19 if a fixture demands them.
package time

import (
	"context"
	"errors"
	"fmt"
	"strings"
	stdtime "time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "time COMMAND [ARG]..."

const helpText = `Usage: time COMMAND [ARG]...
Run COMMAND and print elapsed wall-clock time to stderr.

  --help    show this help and exit
`

// New returns the time command.
func New() command.Command { return command.Define("time", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
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
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	cmdArgs := args[i:]
	if len(cmdArgs) == 0 {
		return command.Result{ExitCode: 0}
	}
	if c.Exec == nil {
		return builtinutil.Errorf(c.Stderr, "time", 1, "sub-shell exec not available")
	}
	parts := make([]string, len(cmdArgs))
	for j, a := range cmdArgs {
		parts[j] = shellQuote(a)
	}
	script := strings.Join(parts, " ")
	start := stdtime.Now()
	res, err := c.Exec(ctx, script, command.SubExecOptions{
		Stdin:  c.Stdin,
		Stdout: c.Stdout,
		Stderr: c.Stderr,
		Env:    c.Env,
		Cwd:    c.Cwd,
	})
	elapsed := stdtime.Since(start)
	if c.Stderr != nil {
		_, _ = fmt.Fprintf(c.Stderr, "\nreal\t%s\n", formatElapsed(elapsed))
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return builtinutil.Errorf(c.Stderr, "time", 1, "%v", err)
	}
	return command.Result{ExitCode: res.ExitCode}
}

func formatElapsed(d stdtime.Duration) string {
	// "Xm Y.YYYs" — matches the bash time keyword format roughly.
	mins := int(d / stdtime.Minute)
	secs := d - stdtime.Duration(mins)*stdtime.Minute
	return fmt.Sprintf("%dm%.3fs", mins, secs.Seconds())
}

// shellQuote returns s wrapped in single quotes with embedded
// single-quotes escaped via the canonical bash idiom.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func init() { command.RegisterBuiltin(New()) }
