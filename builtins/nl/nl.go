// Package nl implements the `nl` built-in (SPEC §10 Wave C).
//
// Flags: -b STYLE (a=all, t=non-empty (default), n=none),
//        -n FORMAT (ln/rn/rz), -w WIDTH (default 6),
//        -s SEP (default tab).
package nl

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "nl [-b STYLE] [-n FORMAT] [-w WIDTH] [-s SEP] [FILE...]"
const helpText = `Usage: nl [OPTION]... [FILE]...
Write each FILE to standard output, with line numbers added.

  -b, --body-numbering=STYLE   use STYLE for numbering body lines (a=all, t=text, n=none)
  -n, --number-format=FORMAT   ln (left), rn (right, no zero), rz (right, zero)
  -w, --number-width=WIDTH     use WIDTH columns for line numbers (default 6)
  -s, --number-separator=SEP   add SEP after the line number (default TAB)`

// New returns the nl command.
func New() command.Command { return command.Define("nl", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	style := "t"
	format := "rn"
	width := 6
	sep := "\t"
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-b", a == "--body-numbering":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			style = args[i]
		case strings.HasPrefix(a, "--body-numbering="):
			style = strings.TrimPrefix(a, "--body-numbering=")
		case a == "-n", a == "--number-format":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			format = args[i]
		case strings.HasPrefix(a, "--number-format="):
			format = strings.TrimPrefix(a, "--number-format=")
		case a == "-w", a == "--number-width":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			w, err := strconv.Atoi(args[i])
			if err != nil || w < 1 {
				return builtinutil.Errorf(c.Stderr, "nl", 1, "invalid width")
			}
			width = w
		case strings.HasPrefix(a, "--number-width="):
			w, err := strconv.Atoi(strings.TrimPrefix(a, "--number-width="))
			if err != nil || w < 1 {
				return builtinutil.Errorf(c.Stderr, "nl", 1, "invalid width")
			}
			width = w
		case a == "-s", a == "--number-separator":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			sep = args[i]
		case strings.HasPrefix(a, "--number-separator="):
			sep = strings.TrimPrefix(a, "--number-separator=")
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
	num := 0
	exit := 0
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "nl: %s: %v\n", f, err)
			exit = 1
			continue
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			show := false
			switch style {
			case "a":
				show = true
			case "t":
				show = line != ""
			case "n":
				show = false
			default:
				show = line != ""
			}
			if show {
				num++
				_, _ = fmt.Fprintln(c.Stdout, formatNum(num, format, width)+sep+line)
			} else {
				// pad with width spaces + sep
				_, _ = fmt.Fprintln(c.Stdout, strings.Repeat(" ", width)+sep[:0]+line)
			}
		}
		if err := sc.Err(); err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "nl: %s: %v\n", f, err)
			exit = 1
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	return command.Result{ExitCode: exit}
}

func formatNum(n int, format string, width int) string {
	switch format {
	case "ln":
		return fmt.Sprintf("%-*d", width, n)
	case "rz":
		return fmt.Sprintf("%0*d", width, n)
	default: // rn
		return fmt.Sprintf("%*d", width, n)
	}
}

func init() { command.RegisterBuiltin(New()) }
