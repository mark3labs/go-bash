// Package basename implements the `basename` built-in.
//
// Forms (POSIX + GNU):
//
//	basename NAME [SUFFIX]
//	basename -a NAME...
//	basename -s SUFFIX NAME...
//
// Trailing slashes are stripped before extracting the basename
// (matching coreutils). When SUFFIX is supplied it is removed from
// the result only if the result is longer than SUFFIX (so
// `basename .x .x` returns ".x", not "").
package basename

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "basename NAME [SUFFIX] | basename -a NAME... | basename -s SUFFIX NAME..."
const helpText = `Usage: basename NAME [SUFFIX]
       basename -a NAME...
       basename -s SUFFIX NAME...

Print the last component of NAME, optionally stripping SUFFIX.

  -a            treat every argument as a NAME (no SUFFIX argument)
  -s SUFFIX     strip SUFFIX from each NAME
  -z, --zero    separate output with NUL instead of newline`

// New returns the basename command.
func New() command.Command {
	return command.Define("basename", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	multiple := false
	suffix := ""
	useNul := false
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-a", a == "--multiple":
			multiple = true
		case a == "-z", a == "--zero":
			useNul = true
		case a == "-s":
			multiple = true
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			suffix = args[i]
		case strings.HasPrefix(a, "--suffix="):
			multiple = true
			suffix = strings.TrimPrefix(a, "--suffix=")
		case a == "--":
			i++
			goto done
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	names := args[i:]
	if len(names) == 0 {
		return builtinutil.Errorf(c.Stderr, "basename", 1, "missing operand")
	}
	if !multiple && len(names) == 2 {
		// Legacy form: basename NAME SUFFIX
		suffix = names[1]
		names = names[:1]
	}
	if !multiple && len(names) > 2 {
		return builtinutil.Errorf(c.Stderr, "basename", 1, "extra operand %q", names[2])
	}
	sep := byte('\n')
	if useNul {
		sep = 0
	}
	if c.Stdout != nil {
		for _, n := range names {
			b := computeBasename(n, suffix)
			if _, err := io.WriteString(c.Stdout, b); err != nil {
				return builtinutil.Errorf(c.Stderr, "basename", 1, "write: %v", err)
			}
			if _, err := fmt.Fprintf(c.Stdout, "%c", sep); err != nil {
				return builtinutil.Errorf(c.Stderr, "basename", 1, "write: %v", err)
			}
		}
	}
	return command.Result{ExitCode: 0}
}

func computeBasename(path, suffix string) string {
	if path == "" {
		return ""
	}
	// All slashes: result is "/".
	allSlash := true
	for _, c := range path {
		if c != '/' {
			allSlash = false
			break
		}
	}
	if allSlash {
		return "/"
	}
	// Strip trailing slashes.
	p := strings.TrimRight(path, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	if suffix != "" && p != suffix && strings.HasSuffix(p, suffix) {
		p = p[:len(p)-len(suffix)]
	}
	return p
}

func init() {
	command.RegisterBuiltin(New())
}
