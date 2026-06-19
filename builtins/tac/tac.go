// Package tac implements the `tac` built-in.
//
// Reads input and writes lines in reverse order.
package tac

import (
	"context"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "tac [-rs SEP] [FILE...]"
const helpText = `Usage: tac [OPTION]... [FILE]...
Write each FILE to standard output, last line first.

  -s, --separator=STRING   use STRING as the record separator
  -b, --before             attach the separator before instead of after`

// New returns the tac command.
func New() command.Command { return command.Define("tac", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	sep := "\n"
	before := false
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-s", a == "--separator":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			sep = args[i]
		case strings.HasPrefix(a, "--separator="):
			sep = strings.TrimPrefix(a, "--separator=")
		case strings.HasPrefix(a, "-s") && len(a) > 2:
			sep = a[2:]
		case a == "-b", a == "--before":
			before = true
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			files = append(files, a)
		}
	}
run:
	if len(files) == 0 {
		files = []string{"-"}
	}
	var buf []byte
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "tac", 1, "%s: %v", f, err)
		}
		data, err := io.ReadAll(r)
		if closer != nil {
			_ = closer.Close()
		}
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "tac", 1, "%s: %v", f, err)
		}
		buf = append(buf, data...)
	}
	records := splitRecords(string(buf), sep, before)
	for i := len(records) - 1; i >= 0; i-- {
		_, _ = io.WriteString(c.Stdout, records[i])
	}
	return command.Result{}
}

func splitRecords(s, sep string, before bool) []string {
	if s == "" {
		return nil
	}
	if before {
		// records start with sep (except possibly first)
		parts := strings.SplitAfter(s, sep)
		// rotate: reattach trailing sep as prefix of next
		var out []string
		for i, p := range parts {
			if p == "" {
				continue
			}
			if i == 0 {
				out = append(out, p)
			} else {
				// trim trailing sep from prev and prefix here
				prev := out[len(out)-1]
				if strings.HasSuffix(prev, sep) {
					out[len(out)-1] = strings.TrimSuffix(prev, sep)
				}
				out = append(out, sep+strings.TrimSuffix(p, sep))
				if strings.HasSuffix(p, sep) {
					out[len(out)-1] += sep
				}
			}
		}
		return out
	}
	parts := strings.SplitAfter(s, sep)
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func init() { command.RegisterBuiltin(New()) }
