// Package sqlite3 registers a stub `sqlite3` built-in (SPEC §10 Wave E /
// §14). The real driver lands in Phase 14 as a separate opt-in subpackage
// (github.com/mark3labs/go-bash/sqlite). Until then any invocation of
// `sqlite3` exits non-zero with a "not enabled" diagnostic so scripts
// that probe for it get a clear failure rather than "command not found".
//
// Phase 14 will override this registration by importing the sqlite
// subpackage AFTER the builtins meta-package, which (per
// command.Registry.Register's last-writer-wins contract) replaces the
// stub with the real implementation.
package sqlite3

import (
	"context"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const helpText = `Usage: sqlite3 [OPTIONS] [DATABASE] [SQL]
Run SQL against a SQLite database.

This stub is registered when the SQLite optional runtime has not been
enabled. Import "github.com/mark3labs/go-bash/sqlite" (Phase 14) to
replace the stub with the real driver.

  --help    show this help`

// New returns the sqlite3 stub command.
func New() command.Command { return command.Define("sqlite3", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	for _, a := range args[1:] {
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		}
	}
	return builtinutil.Errorf(c.Stderr, "sqlite3", 1,
		"sqlite3 not enabled (import github.com/mark3labs/go-bash/sqlite to enable)")
}

func init() { command.RegisterBuiltin(New()) }
