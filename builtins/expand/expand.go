// Package expand implements the `expand` built-in (SPEC §10 Wave C).
//
// Flags: -t LIST (tab stops), -i convert leading tabs only.
package expand

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "expand [-i] [-t LIST] [FILE...]"
const helpText = `Usage: expand [OPTION]... [FILE]...
Convert tabs to spaces.

  -i, --initial      do not convert tabs after non-blanks
  -t, --tabs=LIST    use comma-separated list of tab positions (or single N for tab width)`

// New returns the expand command.
func New() command.Command { return command.Define("expand", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	tabs := []int{8}
	initialOnly := false
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-i", a == "--initial":
			initialOnly = true
		case a == "-t", a == "--tabs":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			t, err := parseTabs(args[i])
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "expand", 1, "%v", err)
			}
			tabs = t
		case strings.HasPrefix(a, "--tabs="):
			t, err := parseTabs(strings.TrimPrefix(a, "--tabs="))
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "expand", 1, "%v", err)
			}
			tabs = t
		case strings.HasPrefix(a, "-t") && len(a) > 2:
			t, err := parseTabs(a[2:])
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "expand", 1, "%v", err)
			}
			tabs = t
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
			_, _ = fmt.Fprintf(c.Stderr, "expand: %s: %v\n", f, err)
			exit = 1
			continue
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			_, _ = fmt.Fprintln(c.Stdout, expandLine(sc.Text(), tabs, initialOnly))
		}
		if err := sc.Err(); err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "expand: %s: %v\n", f, err)
			exit = 1
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	return command.Result{ExitCode: exit}
}

func expandLine(line string, tabs []int, initialOnly bool) string {
	var b strings.Builder
	col := 0
	leading := true
	for _, r := range line {
		if r == '\t' && (!initialOnly || leading) {
			next := nextStop(col, tabs)
			for col < next {
				b.WriteByte(' ')
				col++
			}
			continue
		}
		if r != ' ' && r != '\t' {
			leading = false
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
