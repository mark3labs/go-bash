// Package umask implements the `umask` shell built-in (SPEC §11).
// With no args, prints the current mask as a 4-digit octal. With a
// numeric arg, would set the mask — we accept and discard. mvdan/sh
// does NOT implement umask; this is the canonical surface.
package umask

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "umask [-S] [MODE]"

// defaultMask is the SPEC §11 stub default.
const defaultMask = 0o022

// New returns the umask command.
func New() command.Command { return command.Define("umask", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	symbolic := false
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case a == "-S":
			symbolic = true
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
	rest := args[i:]
	if len(rest) == 0 {
		if c.Stdout != nil {
			if symbolic {
				_, _ = fmt.Fprintln(c.Stdout, "u=rwx,g=rx,o=rx")
			} else {
				_, _ = fmt.Fprintf(c.Stdout, "%04o\n", defaultMask)
			}
		}
		return command.Result{ExitCode: 0}
	}
	// Validate numeric MODE (we don't actually apply it).
	if _, err := strconv.ParseInt(rest[0], 8, 32); err != nil {
		return builtinutil.Errorf(c.Stderr, "umask", 1, "%s: invalid octal number", rest[0])
	}
	return command.Result{ExitCode: 0}
}

func init() { command.RegisterBuiltin(New()) }
