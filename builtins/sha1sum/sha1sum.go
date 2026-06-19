// Package sha1sum implements the `sha1sum` built-in (SPEC §10 Wave C).
package sha1sum

import (
	"context"
	"crypto/sha1" //nolint:gosec // sha1sum is a checksum tool, not a security primitive
	"hash"

	"github.com/mark3labs/go-bash/builtins/md5sum"
	"github.com/mark3labs/go-bash/command"
)

// New returns the sha1sum command.
func New() command.Command { return command.Define("sha1sum", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	return md5sum.Sum(c, args, "sha1sum", func() hash.Hash { return sha1.New() })
}

func init() { command.RegisterBuiltin(New()) }
