// Package wait implements the `wait` shell built-in (SPEC §11).
// Background jobs run synchronously in go-bash so `wait` is a no-op
// that always succeeds. mvdan/sh implements its own `wait`; this
// registration is reachable via /bin/wait.
package wait

import (
	"context"

	"github.com/mark3labs/go-bash/command"
)

// New returns the wait command.
func New() command.Command {
	return command.Define("wait", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{ExitCode: 0}
	})
}

func init() { command.RegisterBuiltin(New()) }
