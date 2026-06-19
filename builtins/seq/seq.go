// Package seq implements the `seq` built-in (SPEC §10 Wave A).
//
// Forms:
//
//	seq LAST
//	seq FIRST LAST
//	seq FIRST INCREMENT LAST
//
// Flags:
//
//	-s SEPARATOR   use SEPARATOR instead of newline
//	-w             pad numbers with leading zeros to equal width
//	-f FORMAT      printf-style float format
package seq

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "seq [-s SEP] [-w] [-f FMT] [FIRST [INCREMENT]] LAST"
const helpText = `Usage: seq [OPTION]... [FIRST [INCREMENT]] LAST
Print numbers from FIRST to LAST, in steps of INCREMENT.

  -f FORMAT  use printf-style FORMAT (must consume one float)
  -s STRING  use STRING to separate numbers (default: newline)
  -w         equalize width by padding with leading zeros`

// New returns the seq command.
func New() command.Command {
	return command.Define("seq", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	sep := "\n"
	width := false
	format := ""
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-s":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			sep = args[i]
		case strings.HasPrefix(a, "-s"):
			sep = a[2:]
		case strings.HasPrefix(a, "--separator="):
			sep = strings.TrimPrefix(a, "--separator=")
		case a == "-w", a == "--equal-width":
			width = true
		case a == "-f":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			format = args[i]
		case strings.HasPrefix(a, "-f"):
			format = a[2:]
		case strings.HasPrefix(a, "--format="):
			format = strings.TrimPrefix(a, "--format=")
		case a == "--":
			i++
			goto done
		case len(a) >= 2 && a[0] == '-' && !isNumeric(a):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	operands := args[i:]
	var first, incr, last float64
	first, incr = 1, 1
	switch len(operands) {
	case 1:
		v, err := strconv.ParseFloat(operands[0], 64)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "seq", 1, "invalid floating point argument: %s", operands[0])
		}
		last = v
	case 2:
		a, err := strconv.ParseFloat(operands[0], 64)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "seq", 1, "invalid floating point argument: %s", operands[0])
		}
		b, err := strconv.ParseFloat(operands[1], 64)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "seq", 1, "invalid floating point argument: %s", operands[1])
		}
		first, last = a, b
	case 3:
		a, err := strconv.ParseFloat(operands[0], 64)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "seq", 1, "invalid floating point argument: %s", operands[0])
		}
		b, err := strconv.ParseFloat(operands[1], 64)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "seq", 1, "invalid floating point argument: %s", operands[1])
		}
		cc, err := strconv.ParseFloat(operands[2], 64)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "seq", 1, "invalid floating point argument: %s", operands[2])
		}
		first, incr, last = a, b, cc
	default:
		return builtinutil.UsageError(c.Stderr, usage)
	}
	if incr == 0 {
		return builtinutil.Errorf(c.Stderr, "seq", 1, "invalid Zero increment value: '%g'", incr)
	}
	// Build the value list.
	values := generate(first, incr, last)
	if len(values) == 0 || c.Stdout == nil {
		return command.Result{ExitCode: 0}
	}
	// Determine printed strings.
	strs := make([]string, len(values))
	if format != "" {
		for i, v := range values {
			strs[i] = fmt.Sprintf(format, v)
		}
	} else {
		// Match coreutils: if any operand was fractional, format
		// with %g; otherwise use %.0f.
		fractional := hasFractional(operands)
		for i, v := range values {
			if fractional {
				strs[i] = formatNumber(v)
			} else {
				strs[i] = strconv.FormatInt(int64(math.Round(v)), 10)
			}
		}
	}
	if width {
		maxLen := 0
		for _, s := range strs {
			// strip sign from width calc — coreutils pads after the sign.
			n := len(s)
			if strings.HasPrefix(s, "-") {
				n--
			}
			if n > maxLen {
				maxLen = n
			}
		}
		for i, s := range strs {
			sign := ""
			body := s
			if strings.HasPrefix(s, "-") {
				sign = "-"
				body = s[1:]
			}
			for len(body) < maxLen {
				body = "0" + body
			}
			strs[i] = sign + body
		}
	}
	out := strings.Join(strs, sep) + "\n"
	if _, err := io.WriteString(c.Stdout, out); err != nil {
		return builtinutil.Errorf(c.Stderr, "seq", 1, "write: %v", err)
	}
	return command.Result{ExitCode: 0}
}

func generate(first, incr, last float64) []float64 {
	var out []float64
	// Limit total entries to a reasonable safety cap so a degenerate
	// `seq 1 0.000000001 1e9` does not OOM. The cap is intentionally
	// loose; the Phase 21 hardening pass can revisit if needed.
	const safetyCap = 1 << 24
	if incr > 0 {
		for v := first; v <= last+1e-12 && len(out) < safetyCap; v += incr {
			out = append(out, v)
		}
	} else if incr < 0 {
		for v := first; v >= last-1e-12 && len(out) < safetyCap; v += incr {
			out = append(out, v)
		}
	}
	return out
}

func hasFractional(ops []string) bool {
	for _, o := range ops {
		if strings.ContainsAny(o, ".eE") {
			return true
		}
	}
	return false
}

func formatNumber(v float64) string {
	// strconv.FormatFloat with -1 prec drops trailing zeros but
	// keeps essential ones — closer to coreutils than %g.
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func isNumeric(a string) bool {
	if len(a) == 0 {
		return false
	}
	if a[0] == '-' && len(a) > 1 {
		_, err := strconv.ParseFloat(a, 64)
		return err == nil
	}
	return false
}

func init() {
	command.RegisterBuiltin(New())
}
