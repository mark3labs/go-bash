// Package comm implements the `comm` built-in.
//
// Flags: -1 -2 -3 suppress the corresponding column.
package comm

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "comm [-123] FILE1 FILE2"
const helpText = `Usage: comm [OPTION]... FILE1 FILE2
Compare sorted files FILE1 and FILE2 line by line.

  -1   suppress column 1 (lines unique to FILE1)
  -2   suppress column 2 (lines unique to FILE2)
  -3   suppress column 3 (lines that appear in both)`

// New returns the comm command.
func New() command.Command { return command.Define("comm", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	suppress1, suppress2, suppress3 := false, false, false
	var pos []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-1":
			suppress1 = true
		case a == "-2":
			suppress2 = true
		case a == "-3":
			suppress3 = true
		case a == "--":
			i++
			pos = append(pos, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			ok := true
			for _, ch := range a[1:] {
				switch ch {
				case '1':
					suppress1 = true
				case '2':
					suppress2 = true
				case '3':
					suppress3 = true
				default:
					ok = false
				}
			}
			if !ok {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			pos = append(pos, a)
		}
	}
run:
	if len(pos) != 2 {
		return builtinutil.Errorf(c.Stderr, "comm", 1, "need exactly two files")
	}
	a, err := readLines(c, pos[0])
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "comm", 1, "%s: %v", pos[0], err)
	}
	b, err := readLines(c, pos[1])
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "comm", 1, "%s: %v", pos[1], err)
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			if !suppress1 {
				_, _ = fmt.Fprintln(c.Stdout, a[i])
			}
			i++
		case a[i] > b[j]:
			if !suppress2 {
				_, _ = fmt.Fprintln(c.Stdout, indent(b[j], !suppress1))
			}
			j++
		default:
			if !suppress3 {
				prefix := ""
				if !suppress1 {
					prefix += "\t"
				}
				if !suppress2 {
					prefix += "\t"
				}
				_, _ = fmt.Fprintln(c.Stdout, prefix+a[i])
			}
			i++
			j++
		}
	}
	for ; i < len(a); i++ {
		if !suppress1 {
			_, _ = fmt.Fprintln(c.Stdout, a[i])
		}
	}
	for ; j < len(b); j++ {
		if !suppress2 {
			_, _ = fmt.Fprintln(c.Stdout, indent(b[j], !suppress1))
		}
	}
	return command.Result{}
}

func indent(s string, withTab bool) string {
	if withTab {
		return "\t" + s
	}
	return s
}

func readLines(c *command.Context, name string) ([]string, error) {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out []string
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

func init() { command.RegisterBuiltin(New()) }
