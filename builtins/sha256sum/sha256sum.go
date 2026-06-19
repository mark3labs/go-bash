// Package sha256sum implements the `sha256sum` built-in (SPEC §10 Wave C).
package sha256sum

import (
	"context"
	"crypto/sha256"
	"hash"

	"github.com/mark3labs/go-bash/builtins/md5sum"
	"github.com/mark3labs/go-bash/command"
)

// New returns the sha256sum command.
func New() command.Command { return command.Define("sha256sum", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	return md5sum.Sum(c, args, "sha256sum", func() hash.Hash { return sha256.New() })
}

func init() { command.RegisterBuiltin(New()) }
