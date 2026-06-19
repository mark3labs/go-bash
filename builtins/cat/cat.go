// Package cat implements the `cat` built-in.
//
// Flags: -n number all lines, -b number non-blank lines, -E show $ at
// line-ends, -T show tabs as ^I, -A like -vET, -s squeeze blank lines,
// -v show non-printing.
package cat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "cat [-nbEAtTsv] [FILE...]"
const helpText = `Usage: cat [OPTION]... [FILE]...
Concatenate FILE(s) to standard output. With no FILE, or when FILE is -, read standard input.

  -A, --show-all          equivalent to -vET
  -b, --number-nonblank   number nonempty output lines (overrides -n)
  -E, --show-ends         display $ at end of each line
  -n, --number            number all output lines
  -s, --squeeze-blank     suppress repeated empty output lines
  -T, --show-tabs         display TAB as ^I
  -v, --show-nonprinting  use ^ and M- notation, except for LFD and TAB`

type opts struct {
	number, numberNonblank, showEnds, showTabs, squeeze, showNonprint bool
}

// New returns the cat command.
func New() command.Command { return command.Define("cat", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var o opts
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-n", a == "--number":
			o.number = true
		case a == "-b", a == "--number-nonblank":
			o.numberNonblank = true
		case a == "-E", a == "--show-ends":
			o.showEnds = true
		case a == "-T", a == "--show-tabs":
			o.showTabs = true
		case a == "-A", a == "--show-all":
			o.showEnds, o.showTabs, o.showNonprint = true, true, true
		case a == "-s", a == "--squeeze-blank":
			o.squeeze = true
		case a == "-v", a == "--show-nonprinting":
			o.showNonprint = true
		case a == "-e":
			o.showEnds, o.showNonprint = true, true
		case a == "-t":
			o.showTabs, o.showNonprint = true, true
		case a == "-u":
			// no-op (POSIX: unbuffered)
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			if bundle(a, &o) {
				continue
			}
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			files = append(files, a)
		}
	}
run:
	if len(files) == 0 {
		files = []string{"-"}
	}
	if c.Stdout == nil {
		return command.Result{}
	}
	transform := o.number || o.numberNonblank || o.showEnds || o.showTabs || o.squeeze || o.showNonprint

	exit := 0
	w := bufio.NewWriter(c.Stdout)
	defer func() { _ = w.Flush() }()
	lineNum := 0
	prevBlank := false
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "cat: %s: %v\n", f, err)
			continue
		}
		if !transform {
			_, _ = io.Copy(w, r)
			if closer != nil {
				_ = closer.Close()
			}
			continue
		}
		sc := builtinutil.ScanLines(r)
		for sc.Scan() {
			line := sc.Text()
			blank := line == ""
			if o.squeeze && blank && prevBlank {
				continue
			}
			prevBlank = blank
			prefix := ""
			if o.numberNonblank {
				if !blank {
					lineNum++
					prefix = fmt.Sprintf("%6d\t", lineNum)
				}
			} else if o.number {
				lineNum++
				prefix = fmt.Sprintf("%6d\t", lineNum)
			}
			out := line
			if o.showTabs {
				out = strings.ReplaceAll(out, "\t", "^I")
			}
			if o.showNonprint {
				out = renderNonPrinting(out)
			}
			if o.showEnds {
				out += "$"
			}
			_, _ = fmt.Fprintf(w, "%s%s\n", prefix, out)
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	return command.Result{ExitCode: exit}
}

func renderNonPrinting(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\t':
			b.WriteByte('\t')
		case c == '\n':
			b.WriteByte('\n')
		case c < 32:
			b.WriteByte('^')
			b.WriteByte(c + 64)
		case c == 127:
			b.WriteString("^?")
		case c >= 128 && c < 160:
			b.WriteString("M-^")
			b.WriteByte(c - 128 + 64)
		case c >= 160 && c < 255:
			b.WriteString("M-")
			b.WriteByte(c - 128)
		case c == 255:
			b.WriteString("M-^?")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func bundle(a string, o *opts) bool {
	for _, ch := range a[1:] {
		switch ch {
		case 'n':
			o.number = true
		case 'b':
			o.numberNonblank = true
		case 'E':
			o.showEnds = true
		case 'T':
			o.showTabs = true
		case 'A':
			o.showEnds, o.showTabs, o.showNonprint = true, true, true
		case 's':
			o.squeeze = true
		case 'v':
			o.showNonprint = true
		case 'e':
			o.showEnds, o.showNonprint = true, true
		case 't':
			o.showTabs, o.showNonprint = true, true
		case 'u':
			// no-op
		default:
			return false
		}
	}
	return true
}

func init() { command.RegisterBuiltin(New()) }
