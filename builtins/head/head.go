// Package head implements the `head` built-in.
//
// Flags:
//   -n N  show first N lines (default 10); -n -N drops trailing N
//   -c N  show first N bytes;              -c -N drops trailing N
//   -q quiet (no headers when multiple files), -v always headers
package head

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "head [-n N] [-c N] [-qv] [FILE...]"
const helpText = `Usage: head [OPTION]... [FILE]...
Print the first 10 lines of each FILE to standard output.

  -c, --bytes=N         print the first N bytes; -N prints all but trailing N
  -n, --lines=N         print the first N lines; -N prints all but trailing N
  -q, --quiet           never print headers
  -v, --verbose         always print headers`

type opts struct {
	mode             byte // 'n' or 'c'
	count            int
	negative         bool // -N meaning "all but the last N"
	quiet, verbose   bool
}

// New returns the head command.
func New() command.Command { return command.Define("head", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	o := opts{mode: 'n', count: 10}
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-n", a == "--lines":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			if err := parseCount(args[i], &o, 'n'); err != nil {
				return builtinutil.Errorf(c.Stderr, "head", 1, "invalid number of lines: %q", args[i])
			}
		case strings.HasPrefix(a, "--lines="):
			if err := parseCount(strings.TrimPrefix(a, "--lines="), &o, 'n'); err != nil {
				return builtinutil.Errorf(c.Stderr, "head", 1, "invalid number of lines")
			}
		case a == "-c", a == "--bytes":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			if err := parseCount(args[i], &o, 'c'); err != nil {
				return builtinutil.Errorf(c.Stderr, "head", 1, "invalid number of bytes: %q", args[i])
			}
		case strings.HasPrefix(a, "--bytes="):
			if err := parseCount(strings.TrimPrefix(a, "--bytes="), &o, 'c'); err != nil {
				return builtinutil.Errorf(c.Stderr, "head", 1, "invalid number of bytes")
			}
		case a == "-q", a == "--quiet", a == "--silent":
			o.quiet = true
		case a == "-v", a == "--verbose":
			o.verbose = true
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && isCount(a[1:]):
			// Old-style -N (lines)
			if err := parseCount(a[1:], &o, 'n'); err != nil {
				return builtinutil.Errorf(c.Stderr, "head", 1, "invalid count")
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			files = append(files, a)
		}
	}
run:
	if len(files) == 0 {
		files = []string{"-"}
	}
	multi := len(files) > 1
	exit := 0
	for idx, f := range files {
		if (multi && !o.quiet) || o.verbose {
			name := f
			if name == "-" {
				name = "standard input"
			}
			if idx > 0 {
				_, _ = fmt.Fprintln(c.Stdout)
			}
			_, _ = fmt.Fprintf(c.Stdout, "==> %s <==\n", name)
		}
		if err := one(c, f, &o); err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "head: %s: %v\n", f, err)
			exit = 1
		}
	}
	return command.Result{ExitCode: exit}
}

func one(c *command.Context, name string, o *opts) error {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		return err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if o.mode == 'c' {
		if o.negative {
			if o.count >= len(data) {
				return nil
			}
			data = data[:len(data)-o.count]
		} else if o.count < len(data) {
			data = data[:o.count]
		}
		_, _ = c.Stdout.Write(data)
		return nil
	}
	// line mode
	lines := splitLinesKeep(data)
	if o.negative {
		if o.count >= len(lines) {
			return nil
		}
		lines = lines[:len(lines)-o.count]
	} else if o.count < len(lines) {
		lines = lines[:o.count]
	}
	for _, l := range lines {
		_, _ = c.Stdout.Write(l)
	}
	return nil
}

// splitLinesKeep splits data into lines, keeping trailing newlines.
func splitLinesKeep(data []byte) [][]byte {
	var out [][]byte
	for len(data) > 0 {
		i := 0
		for i < len(data) && data[i] != '\n' {
			i++
		}
		if i < len(data) {
			out = append(out, data[:i+1])
			data = data[i+1:]
		} else {
			out = append(out, data)
			data = nil
		}
	}
	return out
}

func parseCount(s string, o *opts, mode byte) error {
	o.mode = mode
	o.negative = false
	if strings.HasPrefix(s, "-") {
		o.negative = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	// suffix multipliers
	mult := int64(1)
	if len(s) > 0 {
		switch s[len(s)-1] {
		case 'b':
			mult = 512
			s = s[:len(s)-1]
		case 'k':
			mult = 1024
			s = s[:len(s)-1]
		case 'K':
			mult = 1024
			s = s[:len(s)-1]
		case 'M':
			mult = 1024 * 1024
			s = s[:len(s)-1]
		case 'G':
			mult = 1024 * 1024 * 1024
			s = s[:len(s)-1]
		}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	o.count = int(v * mult)
	return nil
}

func isCount(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func init() { command.RegisterBuiltin(New()) }
