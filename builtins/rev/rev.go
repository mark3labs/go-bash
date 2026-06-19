// Package rev implements the `rev` built-in (SPEC §10 Wave C).
package rev

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "rev [FILE...]"
const helpText = `Usage: rev [FILE]...
Reverse lines characterwise.`

// New returns the rev command.
func New() command.Command { return command.Define("rev", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var files []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "--":
			files = append(files, args[i+1:]...)
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
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "rev: %s: %v\n", f, err)
			continue
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			_, _ = fmt.Fprintln(c.Stdout, reverseString(sc.Text()))
		}
		if err := sc.Err(); err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "rev: %s: %v\n", f, err)
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	return command.Result{ExitCode: exit}
}

func reverseString(s string) string {
	rs := []rune(s)
	for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
		rs[i], rs[j] = rs[j], rs[i]
	}
	return string(rs)
}

func init() { command.RegisterBuiltin(New()) }
