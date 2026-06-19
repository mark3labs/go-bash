// Package whoami implements the `whoami` built-in (SPEC §10 Wave A).
// Always prints "user" (matches just-bash; the sandbox does not expose
// a real user identity to scripts).
package whoami

import (
	"context"
	"fmt"
	"io"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "whoami [--help]"
const helpText = `Usage: whoami [--help]
Print the effective user name. In the gobash sandbox this is always
"user".`

// New returns the whoami command.
func New() command.Command {
	return command.Define("whoami", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, a := range args[1:] {
		switch a {
		case "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		default:
			return builtinutil.UsageError(c.Stderr, usage)
		}
	}
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, "user")
		_, _ = fmt.Fprintln(c.Stdout)
	}
	return command.Result{ExitCode: 0}
}

func init() {
	command.RegisterBuiltin(New())
}
