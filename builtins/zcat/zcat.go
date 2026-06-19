// Package zcat implements the `zcat` built-in — `gunzip -c` shorthand.
// SPEC §10 Wave F.
package zcat

import (
	"context"

	"github.com/mark3labs/go-bash/builtins/gzip"
	"github.com/mark3labs/go-bash/command"
)

// New returns the zcat command.
func New() command.Command { return command.Define("zcat", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	return gzip.Run(c, args, gzip.ModeZcat)
}

func init() { command.RegisterBuiltin(New()) }
