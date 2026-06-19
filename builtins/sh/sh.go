// Package sh implements the `sh` built-in — a thin shim around the
// `bash` built-in (SPEC §10 Wave G). The Go runtime exposes a single
// shell flavor; `sh` differs only in name and helper text.
package sh

import (
	"context"

	"github.com/mark3labs/go-bash/builtins/bash"
	"github.com/mark3labs/go-bash/command"
)

// New returns the sh command.
func New() command.Command {
	return command.Define("sh", func(ctx context.Context, args []string, c *command.Context) command.Result {
		return bash.Run(ctx, args, c, bash.ModeSh)
	})
}

func init() { command.RegisterBuiltin(New()) }
