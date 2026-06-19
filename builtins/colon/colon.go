// Package colon implements the `:` (colon) shell built-in (SPEC §11).
// It does nothing and exits 0. mvdan/sh ships its own `:` builtin so
// this implementation is normally shadowed; invoke `/bin/:` to reach
// it through the registry.
package colon

import (
	"context"

	"github.com/mark3labs/go-bash/command"
)

// New returns the colon command.
func New() command.Command {
	return command.Define(":", func(_ context.Context, _ []string, _ *command.Context) command.Result {
		return command.Result{ExitCode: 0}
	})
}

func init() { command.RegisterBuiltin(New()) }
