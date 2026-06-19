// Package timeout implements the `timeout` built-in (SPEC §10 Wave G).
//
// Usage:
//
//	timeout [OPTIONS] DURATION COMMAND [ARG]...
//
// Options:
//
//	-s, --signal=SIG       signal to send on timeout (recorded but
//	                       not delivered — no real signals)
//	-k, --kill-after=DUR   send KILL after DUR if process is still
//	                       running (also a no-op)
//	--preserve-status      return COMMAND's status even on timeout
//	--help                 show this help and exit
//
// On timeout, exits 124 (matching GNU timeout). The COMMAND is
// dispatched via Context.Exec so it runs through the same runtime
// pipeline (limits, registry, stdio) as the caller.
package timeout

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "timeout [-s SIG] [-k DUR] DURATION COMMAND [ARG]..."

const helpText = `Usage: timeout [OPTIONS] DURATION COMMAND [ARG]...
Run COMMAND under a wall-clock timeout. If COMMAND has not finished
after DURATION, terminate it and exit with status 124.

Options:
  -s, --signal=SIG       signal to send on timeout (recorded, not delivered)
  -k, --kill-after=DUR   send KILL after DUR more time (no-op here)
      --preserve-status  return COMMAND's status even on timeout
      --help             show this help and exit
`

// New returns the timeout command.
func New() command.Command { return command.Define("timeout", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	preserve := false

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
		case a == "--preserve-status":
			preserve = true
		case a == "-s" || a == "--signal":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			// recorded, ignored
		case strings.HasPrefix(a, "--signal="):
			// recorded, ignored
		case a == "-k" || a == "--kill-after":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			// recorded, ignored
		case strings.HasPrefix(a, "--kill-after="):
			// recorded, ignored
		case strings.HasPrefix(a, "-") && len(a) > 1 && !isDurationLike(a):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	rest := args[i:]
	if len(rest) < 2 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	dur, err := parseDuration(rest[0])
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "timeout", 125, "invalid time interval %q", rest[0])
	}
	cmdArgs := rest[1:]

	if c.Exec == nil {
		return builtinutil.Errorf(c.Stderr, "timeout", 125, "sub-shell exec not available")
	}

	// Build a script. Shell-quote each arg to preserve word boundaries.
	parts := make([]string, len(cmdArgs))
	for j, a := range cmdArgs {
		parts[j] = shellQuote(a)
	}
	script := strings.Join(parts, " ")

	tctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()
	res, err := c.Exec(tctx, script, command.SubExecOptions{
		Stdin:  c.Stdin,
		Stdout: c.Stdout,
		Stderr: c.Stderr,
		Env:    c.Env,
		Cwd:    c.Cwd,
	})
	timedOut := errors.Is(err, context.DeadlineExceeded) ||
		(tctx.Err() == context.DeadlineExceeded && (errors.Is(err, context.Canceled) || err == nil))
	if timedOut {
		if preserve {
			return command.Result{ExitCode: res.ExitCode}
		}
		return command.Result{ExitCode: 124}
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return builtinutil.Errorf(c.Stderr, "timeout", 125, "%v", err)
	}
	return command.Result{ExitCode: res.ExitCode}
}

// parseDuration accepts the same suffix set as `sleep`: bare seconds
// (with optional fraction), s/m/h/d.
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	mult := time.Second
	switch s[len(s)-1] {
	case 's':
		s = s[:len(s)-1]
	case 'm':
		mult = time.Minute
		s = s[:len(s)-1]
	case 'h':
		mult = time.Hour
		s = s[:len(s)-1]
	case 'd':
		mult = 24 * time.Hour
		s = s[:len(s)-1]
	}
	if s == "" {
		return 0, fmt.Errorf("no number")
	}
	v, err := parseFloat(s)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("negative")
	}
	return time.Duration(v * float64(mult)), nil
}

func parseFloat(s string) (float64, error) {
	// Local helper to avoid pulling strconv-only imports into the
	// hot section above; reuses the stdlib parser.
	var v float64
	_, err := fmt.Sscanf(s, "%g", &v)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func isDurationLike(a string) bool {
	if len(a) < 2 {
		return false
	}
	// Starts with digit after the dash? e.g. "-1" (not a duration here)
	// — but timeout's DURATION is the first positional, not a flag.
	// The check here exists to keep negative-number-looking args out
	// of the flag parser when the user puts them in flag position.
	if a[1] >= '0' && a[1] <= '9' {
		return true
	}
	return false
}

// shellQuote returns s wrapped in single quotes with embedded
// single-quotes escaped via the canonical bash idiom.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// Conservative: always single-quote.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func init() { command.RegisterBuiltin(New()) }
