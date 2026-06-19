// Package egrep implements the `egrep` built-in — grep with the
// extended-regex (-E) mode as the default. The spec Wave D.
package egrep

import (
	"context"

	"github.com/mark3labs/go-bash/builtins/grep"
	"github.com/mark3labs/go-bash/command"
)

// New returns the egrep command.
func New() command.Command { return command.Define("egrep", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	return grep.Run(c, args, grep.ModeExtended)
}

func init() { command.RegisterBuiltin(New()) }
