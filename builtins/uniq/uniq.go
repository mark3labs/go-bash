// Package uniq implements the `uniq` built-in.
//
// Flags: -c count, -d only repeated, -D all repeated, -u only unique,
// -i case-fold, -f N skip fields, -s N skip chars, -w N compare chars.
package uniq

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "uniq [-cduDi] [-f N] [-s N] [-w N] [INPUT [OUTPUT]]"
const helpText = `Usage: uniq [OPTION]... [INPUT [OUTPUT]]
Filter adjacent matching lines from INPUT (or stdin).

  -c, --count        prefix lines by the number of occurrences
  -d, --repeated     only print duplicate lines, one for each group
  -D                 print all duplicate lines
  -u, --unique       only print unique lines
  -i, --ignore-case
  -f N, --skip-fields=N
  -s N, --skip-chars=N
  -w N, --check-chars=N`

type opts struct {
	count, dup, allDup, uniq, foldCase bool
	skipFields, skipChars, checkChars  int
}

// New returns the uniq command.
func New() command.Command { return command.Define("uniq", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var o opts
	var pos []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-c", a == "--count":
			o.count = true
		case a == "-d", a == "--repeated":
			o.dup = true
		case a == "-D":
			o.allDup = true
		case a == "-u", a == "--unique":
			o.uniq = true
		case a == "-i", a == "--ignore-case":
			o.foldCase = true
		case a == "-f", a == "--skip-fields":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.skipFields, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(a, "--skip-fields="):
			o.skipFields, _ = strconv.Atoi(strings.TrimPrefix(a, "--skip-fields="))
		case a == "-s", a == "--skip-chars":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.skipChars, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(a, "--skip-chars="):
			o.skipChars, _ = strconv.Atoi(strings.TrimPrefix(a, "--skip-chars="))
		case a == "-w", a == "--check-chars":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.checkChars, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(a, "--check-chars="):
			o.checkChars, _ = strconv.Atoi(strings.TrimPrefix(a, "--check-chars="))
		case a == "--":
			i++
			pos = append(pos, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			pos = append(pos, a)
		}
	}
run:
	input := "-"
	if len(pos) >= 1 {
		input = pos[0]
	}
	r, closer, err := builtinutil.OpenInput(c, input)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "uniq", 1, "%v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var groups []group
	prev := ""
	prevKey := ""
	first := true
	for sc.Scan() {
		line := sc.Text()
		k := uniqKey(line, &o)
		if !first && k == prevKey {
			groups[len(groups)-1].count++
			groups[len(groups)-1].lines = append(groups[len(groups)-1].lines, line)
			continue
		}
		groups = append(groups, group{line: line, count: 1, lines: []string{line}})
		prev = line
		prevKey = k
		first = false
		_ = prev
	}
	if err := sc.Err(); err != nil {
		return builtinutil.Errorf(c.Stderr, "uniq", 1, "%v", err)
	}
	w := c.Stdout
	for _, g := range groups {
		if o.uniq && g.count > 1 {
			continue
		}
		if o.dup && g.count < 2 {
			continue
		}
		if o.allDup && g.count < 2 {
			continue
		}
		if o.allDup {
			for _, l := range g.lines {
				if o.count {
					_, _ = fmt.Fprintf(w, "%7d %s\n", g.count, l)
				} else {
					_, _ = fmt.Fprintln(w, l)
				}
			}
			continue
		}
		if o.count {
			_, _ = fmt.Fprintf(w, "%7d %s\n", g.count, g.line)
		} else {
			_, _ = fmt.Fprintln(w, g.line)
		}
	}
	return command.Result{}
}

type group struct {
	line  string
	count int
	lines []string
}

func uniqKey(s string, o *opts) string {
	if o.skipFields > 0 {
		fields := strings.Fields(s)
		if o.skipFields >= len(fields) {
			s = ""
		} else {
			// rebuild from skipFields onwards
			s = strings.Join(fields[o.skipFields:], " ")
		}
	}
	if o.skipChars > 0 {
		if o.skipChars >= len(s) {
			s = ""
		} else {
			s = s[o.skipChars:]
		}
	}
	if o.checkChars > 0 && o.checkChars < len(s) {
		s = s[:o.checkChars]
	}
	if o.foldCase {
		s = strings.ToLower(s)
	}
	return s
}

func init() { command.RegisterBuiltin(New()) }
