// Package dirname implements the `dirname` built-in (SPEC §10 Wave A).
//
// Strips the last component of each NAME (after trimming trailing
// slashes) and prints the remainder. "/" stays "/", a name with no
// slash becomes ".", trailing slashes are stripped before the cut.
package dirname

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "dirname [-z|--zero] NAME..."
const helpText = `Usage: dirname [OPTION] NAME...
Print NAME with its trailing /component removed; if NAME contains no
slashes, output '.'.

  -z, --zero    separate output with NUL instead of newline`

// New returns the dirname command.
func New() command.Command {
	return command.Define("dirname", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	useNul := false
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case "-z", "--zero":
			useNul = true
		case "--":
			i++
			goto done
		default:
			if strings.HasPrefix(a, "-") && len(a) > 1 {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			goto done
		}
	}
done:
	names := args[i:]
	if len(names) == 0 {
		return builtinutil.Errorf(c.Stderr, "dirname", 1, "missing operand")
	}
	sep := byte('\n')
	if useNul {
		sep = 0
	}
	if c.Stdout != nil {
		for _, n := range names {
			d := computeDirname(n)
			if _, err := io.WriteString(c.Stdout, d); err != nil {
				return builtinutil.Errorf(c.Stderr, "dirname", 1, "write: %v", err)
			}
			if _, err := fmt.Fprintf(c.Stdout, "%c", sep); err != nil {
				return builtinutil.Errorf(c.Stderr, "dirname", 1, "write: %v", err)
			}
		}
	}
	return command.Result{ExitCode: 0}
}

func computeDirname(p string) string {
	if p == "" {
		return "."
	}
	// All slashes → "/".
	allSlash := true
	for _, c := range p {
		if c != '/' {
			allSlash = false
			break
		}
	}
	if allSlash {
		return "/"
	}
	// Strip trailing slashes.
	q := strings.TrimRight(p, "/")
	i := strings.LastIndex(q, "/")
	if i < 0 {
		return "."
	}
	// Strip the basename, then strip any redundant trailing slashes
	// from the prefix (but keep at least one when the prefix is the
	// root "/").
	prefix := strings.TrimRight(q[:i], "/")
	if prefix == "" {
		return "/"
	}
	return prefix
}

func init() {
	command.RegisterBuiltin(New())
}
