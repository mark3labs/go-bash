// Package gunzip implements the `gunzip` built-in — gzip with the
// decompress mode as the default. SPEC §10 Wave F.
package gunzip

import (
	"context"

	"github.com/mark3labs/go-bash/builtins/gzip"
	"github.com/mark3labs/go-bash/command"
)

// New returns the gunzip command.
func New() command.Command { return command.Define("gunzip", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	return gzip.Run(c, args, gzip.ModeDecompress)
}

func init() { command.RegisterBuiltin(New()) }
