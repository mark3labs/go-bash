// Package paste implements the `paste` built-in (SPEC §10 Wave C).
//
// Flags: -d DELIMS, -s (serial — paste all lines per file on one line).
package paste

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "paste [-s] [-d DELIMS] [FILE...]"
const helpText = `Usage: paste [OPTION]... [FILE]...
Merge lines of FILEs.

  -d, --delimiters=LIST  reuse characters from LIST instead of TAB
  -s, --serial           paste one file at a time instead of in parallel`

// New returns the paste command.
func New() command.Command { return command.Define("paste", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	delims := []string{"\t"}
	serial := false
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-s", a == "--serial":
			serial = true
		case a == "-d", a == "--delimiters":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			delims = splitDelims(args[i])
		case strings.HasPrefix(a, "--delimiters="):
			delims = splitDelims(strings.TrimPrefix(a, "--delimiters="))
		case strings.HasPrefix(a, "-d") && len(a) > 2:
			delims = splitDelims(a[2:])
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
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
	if serial {
		return runSerial(c, files, delims)
	}
	return runParallel(c, files, delims)
}

func runParallel(c *command.Context, files []string, delims []string) command.Result {
	readers := make([]*bufio.Scanner, len(files))
	closers := make([]io.Closer, 0, len(files))
	defer func() {
		for _, cl := range closers {
			_ = cl.Close()
		}
	}()
	for i, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "paste", 1, "%s: %v", f, err)
		}
		if closer != nil {
			closers = append(closers, closer)
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		readers[i] = sc
	}
	for {
		anyOpen := false
		row := make([]string, len(files))
		for i, sc := range readers {
			if sc == nil {
				row[i] = ""
				continue
			}
			if sc.Scan() {
				row[i] = sc.Text()
				anyOpen = true
			} else {
				readers[i] = nil
				row[i] = ""
			}
		}
		if !anyOpen {
			break
		}
		writeRow(c.Stdout, row, delims)
	}
	return command.Result{}
}

func runSerial(c *command.Context, files []string, delims []string) command.Result {
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "paste", 1, "%s: %v", f, err)
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		var parts []string
		for sc.Scan() {
			parts = append(parts, sc.Text())
		}
		scanErr := sc.Err()
		if closer != nil {
			_ = closer.Close()
		}
		if scanErr != nil {
			return builtinutil.Errorf(c.Stderr, "paste", 1, "%s: %v", f, scanErr)
		}
		writeRow(c.Stdout, parts, delims)
	}
	return command.Result{}
}

func writeRow(w io.Writer, row []string, delims []string) {
	var b strings.Builder
	for i, cell := range row {
		b.WriteString(cell)
		if i < len(row)-1 {
			d := delims[i%len(delims)]
			b.WriteString(d)
		}
	}
	_, _ = fmt.Fprintln(w, b.String())
}

// splitDelims handles backslash escapes (\n, \t, \\, \0).
func splitDelims(s string) []string {
	if s == "" {
		return []string{""}
	}
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				out = append(out, "\n")
			case 't':
				out = append(out, "\t")
			case '\\':
				out = append(out, "\\")
			case '0':
				out = append(out, "")
			default:
				out = append(out, string(s[i+1]))
			}
			i++
		} else {
			out = append(out, string(s[i]))
		}
	}
	return out
}

func init() { command.RegisterBuiltin(New()) }
