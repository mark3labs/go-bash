// Package strings implements the `strings` built-in (SPEC §10 Wave C).
//
// Flags: -n MIN (default 4), -a (all sections — no-op since we have
// no sections), -t {d,o,x} print offset prefix.
package strings

import (
	"context"
	"fmt"
	"io"
	stdstrings "strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "strings [-a] [-n MIN] [-t d|o|x] [FILE...]"
const helpText = `Usage: strings [OPTION]... [FILE]...
Print printable character sequences of at least MIN length in each FILE.

  -a, --all          scan whole file (default)
  -n, --bytes=MIN    minimum sequence length (default 4)
  -t, --radix=FMT    write each string preceded by its byte offset (d, o, x)`

// New returns the strings command.
func New() command.Command { return command.Define("strings", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	minLen := 4
	radix := ""
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-a", a == "--all":
			// no-op
		case a == "-n", a == "--bytes":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n := 0
			if _, err := fmt.Sscanf(args[i], "%d", &n); err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "strings", 1, "invalid -n")
			}
			minLen = n
		case stdstrings.HasPrefix(a, "--bytes="):
			n := 0
			if _, err := fmt.Sscanf(stdstrings.TrimPrefix(a, "--bytes="), "%d", &n); err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "strings", 1, "invalid -n")
			}
			minLen = n
		case stdstrings.HasPrefix(a, "-n") && len(a) > 2:
			n := 0
			if _, err := fmt.Sscanf(a[2:], "%d", &n); err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "strings", 1, "invalid -n")
			}
			minLen = n
		case a == "-t", a == "--radix":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			radix = args[i]
		case stdstrings.HasPrefix(a, "--radix="):
			radix = stdstrings.TrimPrefix(a, "--radix=")
		case stdstrings.HasPrefix(a, "-t") && len(a) > 2:
			radix = a[2:]
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			files = append(files, a)
		}
	}
run:
	if len(files) == 0 {
		files = []string{"-"}
	}
	exit := 0
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "strings: %s: %v\n", f, err)
			exit = 1
			continue
		}
		data, err := io.ReadAll(r)
		if closer != nil {
			_ = closer.Close()
		}
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "strings: %s: %v\n", f, err)
			exit = 1
			continue
		}
		emit(c.Stdout, data, minLen, radix)
	}
	return command.Result{ExitCode: exit}
}

func emit(w io.Writer, data []byte, minLen int, radix string) {
	start := -1
	for i := 0; i <= len(data); i++ {
		var b byte
		if i < len(data) {
			b = data[i]
		}
		printable := i < len(data) && isPrintable(b)
		if printable {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			if i-start >= minLen {
				writeRun(w, data[start:i], start, radix)
			}
			start = -1
		}
	}
}

func writeRun(w io.Writer, run []byte, off int, radix string) {
	switch radix {
	case "d":
		_, _ = fmt.Fprintf(w, "%7d %s\n", off, run)
	case "o":
		_, _ = fmt.Fprintf(w, "%7o %s\n", off, run)
	case "x":
		_, _ = fmt.Fprintf(w, "%7x %s\n", off, run)
	default:
		_, _ = fmt.Fprintf(w, "%s\n", run)
	}
}

func isPrintable(b byte) bool {
	return (b >= 0x20 && b < 0x7f) || b == '\t'
}

func init() { command.RegisterBuiltin(New()) }
