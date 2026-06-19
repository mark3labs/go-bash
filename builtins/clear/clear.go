// Package clear implements the `clear` built-in.
// `clear` writes the ANSI clear-screen sequence `\033[H\033[2J` to
// stdout and exits 0.
package clear

import (
	"context"
	"io"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const helpText = `Usage: clear [--help]
Clear the terminal screen by writing the ANSI clear sequence.`

// ANSIClear is the byte sequence written. Exposed so the test (and a
// future trace consumer) can compare against the canonical literal
// without re-deriving it.
const ANSIClear = "\x1b[H\x1b[2J"

// New returns the clear command.
func New() command.Command {
	return command.Define("clear", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, a := range args[1:] {
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		}
	}
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, ANSIClear)
	}
	return command.Result{ExitCode: 0}
}

func init() {
	command.RegisterBuiltin(New())
}
