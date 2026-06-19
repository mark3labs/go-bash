// Package jobs implements the `jobs` shell built-in.
// Stub: no background jobs in go-bash (background runs synchronously),
// so this prints nothing and exits 0.
package jobs

import (
	"context"

	"github.com/mark3labs/go-bash/command"
)

// New returns the jobs command.
func New() command.Command {
	return command.Define("jobs", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{ExitCode: 0}
	})
}

func init() { command.RegisterBuiltin(New()) }
