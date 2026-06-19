// Package unexpand implements the `unexpand` built-in.
//
// Flags: -a convert all blanks (default: only leading); -t LIST tab stops.
package unexpand

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "unexpand [-a] [-t LIST] [FILE...]"
const helpText = `Usage: unexpand [OPTION]... [FILE]...
Convert blanks in each FILE to tabs, writing to standard output.

  -a, --all          convert all blanks instead of just initial blanks
  -t, --tabs=LIST    use comma-separated list of tab positions`

// New returns the unexpand command.
func New() command.Command { return command.Define("unexpand", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	tabs := []int{8}
	all := false
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-a", a == "--all":
			all = true
		case a == "-t", a == "--tabs":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			t, err := parseTabs(args[i])
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "unexpand", 1, "%v", err)
			}
			tabs = t
			all = true
		case strings.HasPrefix(a, "--tabs="):
			t, err := parseTabs(strings.TrimPrefix(a, "--tabs="))
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "unexpand", 1, "%v", err)
			}
			tabs = t
			all = true
		case strings.HasPrefix(a, "-t") && len(a) > 2:
			t, err := parseTabs(a[2:])
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "unexpand", 1, "%v", err)
			}
			tabs = t
			all = true
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
	exit := 0
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "unexpand: %s: %v\n", f, err)
			exit = 1
			continue
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			_, _ = fmt.Fprintln(c.Stdout, unexpandLine(sc.Text(), tabs, all))
		}
		if err := sc.Err(); err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "unexpand: %s: %v\n", f, err)
			exit = 1
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	return command.Result{ExitCode: exit}
}

func unexpandLine(line string, tabs []int, all bool) string {
	// Pass 1: expand existing tabs to spaces (so we don't mix them).
	expanded := expandToSpaces(line, tabs)
	// Pass 2: replace runs of spaces with tabs to reach the next stop.
	var b strings.Builder
	col := 0
	leading := true
	i := 0
	for i < len(expanded) {
		if expanded[i] == ' ' && (all || leading) {
			runStart := col
			j := i
			for j < len(expanded) && expanded[j] == ' ' {
				j++
			}
			runEnd := runStart + (j - i)
			// while we can jump to a tab stop within our run
			for {
				stop := nextStop(col, tabs)
				if stop > col && stop <= runEnd && stop-col >= 2 {
					b.WriteByte('\t')
					col = stop
					continue
				}
				break
			}
			// emit remaining spaces
			for col < runEnd {
				b.WriteByte(' ')
				col++
			}
			i = j
			continue
		}
		c := expanded[i]
		if c != ' ' && c != '\t' {
			leading = false
		}
		b.WriteByte(c)
		col++
		i++
	}
	return b.String()
}

func expandToSpaces(line string, tabs []int) string {
	var b strings.Builder
	col := 0
	for _, r := range line {
		if r == '\t' {
			next := nextStop(col, tabs)
			for col < next {
				b.WriteByte(' ')
				col++
			}
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

func nextStop(col int, tabs []int) int {
	if len(tabs) == 1 {
		w := tabs[0]
		return ((col / w) + 1) * w
	}
	for _, s := range tabs {
		if s > col {
			return s
		}
	}
	last := tabs[len(tabs)-1]
	return col + (last - (col % last))
}

func parseTabs(spec string) ([]int, error) {
	var out []int
	for _, p := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' }) {
		v, err := strconv.Atoi(p)
		if err != nil || v < 1 {
			return nil, fmt.Errorf("invalid tab stop %q", p)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty tab list")
	}
	return out, nil
}

func init() { command.RegisterBuiltin(New()) }
