// Package sleep implements the `sleep` built-in.
//
// Accepted durations:
//
//	123      -> 123 seconds
//	123s     -> 123 seconds
//	5m       -> 5 minutes
//	2h       -> 2 hours
//	1d       -> 1 day
//	0.5      -> 500 milliseconds (fractional values allowed)
//
// Multiple operands are summed (matching coreutils). The actual wait
// goes through Context.Sleep when non-nil so tests can elide the
// wall-clock delay; otherwise we use time.Sleep guarded by ctx.
package sleep

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "sleep DURATION..."
const helpText = `Usage: sleep NUMBER[SUFFIX]...
Pause for the sum of all durations.

SUFFIX may be 's' (seconds, default), 'm' (minutes), 'h' (hours), or
'd' (days). NUMBER may be a fractional value.`

// New returns the sleep command.
func New() command.Command {
	return command.Define("sleep", run)
}

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
		case strings.HasPrefix(a, "-") && len(a) > 1 && !isNumericOperand(a):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	operands := args[i:]
	if len(operands) == 0 {
		return builtinutil.Errorf(c.Stderr, "sleep", 1, "missing operand")
	}
	var total time.Duration
	for _, op := range operands {
		d, err := parseDuration(op)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "sleep", 1, "invalid time interval %q", op)
		}
		total += d
	}
	if total <= 0 {
		return command.Result{ExitCode: 0}
	}
	var err error
	if c.Sleep != nil {
		err = c.Sleep(ctx, total)
	} else {
		err = realSleep(ctx, total)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return command.Result{ExitCode: 130}
		}
		return builtinutil.Errorf(c.Stderr, "sleep", 1, "%v", err)
	}
	return command.Result{ExitCode: 0}
}

func realSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// isNumericOperand returns true if a starts with a digit (possibly
// negative, but bash's sleep rejects negatives — we accept "-" only
// to surface the canonical "invalid time interval" error). Used so
// "-0.5" doesn't get swallowed by the option parser.
func isNumericOperand(a string) bool {
	if len(a) == 0 {
		return false
	}
	if a[0] >= '0' && a[0] <= '9' {
		return true
	}
	return false
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	// Strip a trailing single-char suffix.
	mult := time.Second
	last := s[len(s)-1]
	switch last {
	case 's':
		mult = time.Second
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
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("negative")
	}
	return time.Duration(v * float64(mult)), nil
}

func init() {
	command.RegisterBuiltin(New())
}
