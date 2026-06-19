// Package column implements the `column` built-in.
//
// Flags: -t format into table, -s SEP input separator, -o SEP output
// separator, -n disable empty-column merging.
package column

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "column [-tn] [-s SEP] [-o SEP] [FILE...]"
const helpText = `Usage: column [OPTION]... [FILE]...
Columnate lists.

  -t              determine the number of columns the input contains and create a table
  -s SEP          specify a set of characters to be used to delimit columns
  -o SEP          specify column delimiter for table output (default two spaces)
  -n              disable merging multiple adjacent delimiters into one`

// New returns the column command.
func New() command.Command { return command.Define("column", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	table := false
	sep := ""
	outSep := "  "
	noMerge := false
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-t", a == "--table":
			table = true
		case a == "-s", a == "--separator":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			sep = args[i]
		case strings.HasPrefix(a, "-s") && len(a) > 2:
			sep = a[2:]
		case a == "-o", a == "--output-separator":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			outSep = args[i]
		case strings.HasPrefix(a, "-o") && len(a) > 2:
			outSep = a[2:]
		case a == "-n":
			noMerge = true
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
	var rows [][]string
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "column", 1, "%s: %v", f, err)
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !table {
				rows = append(rows, []string{line})
				continue
			}
			rows = append(rows, splitRow(line, sep, noMerge))
		}
		if err := sc.Err(); err != nil {
			if closer != nil {
				_ = closer.Close()
			}
			return builtinutil.Errorf(c.Stderr, "column", 1, "%s: %v", f, err)
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	if !table {
		// Without -t, the GNU column would try to layout in columns
		// based on TTY width. We don't have width, so just pass through.
		for _, row := range rows {
			_, _ = fmt.Fprintln(c.Stdout, row[0])
		}
		return command.Result{}
	}
	emit(c.Stdout, rows, outSep)
	return command.Result{}
}

func splitRow(s, sep string, noMerge bool) []string {
	if sep == "" {
		return strings.Fields(s)
	}
	if noMerge {
		return strings.Split(s, sep)
	}
	// Treat each char in sep as a delimiter; merge runs of delimiters.
	delims := sep
	return strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(delims, r)
	})
}

func emit(w io.Writer, rows [][]string, outSep string) {
	for _, line := range strings.Split(builtinutil.Align(rows, outSep), "\n") {
		_, _ = fmt.Fprintln(w, line)
	}
}

func init() { command.RegisterBuiltin(New()) }
