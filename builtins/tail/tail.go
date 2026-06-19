// Package tail implements the `tail` built-in.
//
// Flags:
//   -n N  last N lines (default 10); -n +N start from line N
//   -c N  last N bytes;               -c +N start from byte N
//   -q quiet, -v verbose, -f rejected in sandbox
package tail

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "tail [-n N] [-c N] [-qv] [FILE...]"
const helpText = `Usage: tail [OPTION]... [FILE]...
Print the last 10 lines of each FILE to standard output.

  -c, --bytes=N         output the last N bytes; or use +N to skip first
  -n, --lines=N         output the last N lines; or use +N to start at line N
  -q, --quiet           never output headers
  -v, --verbose         always output headers
  -f                    follow file (REJECTED in sandbox)`

type opts struct {
	mode               byte // 'n' or 'c'
	count              int
	fromStart          bool // +N
	quiet, verbose     bool
}

// New returns the tail command.
func New() command.Command { return command.Define("tail", run) }

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
		case a == "-f", a == "--follow", strings.HasPrefix(a, "--follow="):
			_, _ = fmt.Fprintln(c.Stderr, "tail -f not supported in sandbox")
			return command.Result{ExitCode: 1}
		case a == "-n", a == "--lines":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			if err := parseCount(args[i], &o, 'n'); err != nil {
				return builtinutil.Errorf(c.Stderr, "tail", 1, "invalid number of lines: %q", args[i])
			}
		case strings.HasPrefix(a, "--lines="):
			if err := parseCount(strings.TrimPrefix(a, "--lines="), &o, 'n'); err != nil {
				return builtinutil.Errorf(c.Stderr, "tail", 1, "invalid number of lines")
			}
		case a == "-c", a == "--bytes":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			if err := parseCount(args[i], &o, 'c'); err != nil {
				return builtinutil.Errorf(c.Stderr, "tail", 1, "invalid number of bytes: %q", args[i])
			}
		case strings.HasPrefix(a, "--bytes="):
			if err := parseCount(strings.TrimPrefix(a, "--bytes="), &o, 'c'); err != nil {
				return builtinutil.Errorf(c.Stderr, "tail", 1, "invalid number of bytes")
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
			if err := parseCount(a[1:], &o, 'n'); err != nil {
				return builtinutil.Errorf(c.Stderr, "tail", 1, "invalid count")
			}
		case strings.HasPrefix(a, "+") && isCount(a[1:]):
			if err := parseCount(a, &o, 'n'); err != nil {
				return builtinutil.Errorf(c.Stderr, "tail", 1, "invalid count")
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
			_, _ = fmt.Fprintf(c.Stderr, "tail: %s: %v\n", f, err)
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
		if o.fromStart {
			off := o.count - 1
			if off < 0 {
				off = 0
			}
			if off >= len(data) {
				return nil
			}
			_, _ = c.Stdout.Write(data[off:])
		} else if o.count < len(data) {
			_, _ = c.Stdout.Write(data[len(data)-o.count:])
		} else {
			_, _ = c.Stdout.Write(data)
		}
		return nil
	}
	lines := splitLinesKeep(data)
	if o.fromStart {
		start := o.count - 1
		if start < 0 {
			start = 0
		}
		if start >= len(lines) {
			return nil
		}
		lines = lines[start:]
	} else if o.count < len(lines) {
		lines = lines[len(lines)-o.count:]
	}
	for _, l := range lines {
		_, _ = c.Stdout.Write(l)
	}
	return nil
}

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
	o.fromStart = false
	if strings.HasPrefix(s, "+") {
		o.fromStart = true
		s = s[1:]
	} else if strings.HasPrefix(s, "-") {
		s = s[1:]
	}
	mult := int64(1)
	if len(s) > 0 {
		switch s[len(s)-1] {
		case 'b':
			mult = 512
			s = s[:len(s)-1]
		case 'k', 'K':
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
