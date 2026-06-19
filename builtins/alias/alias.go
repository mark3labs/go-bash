// Package alias implements the `alias` built-in (SPEC §10 Wave G).
//
// Usage:
//
//	alias                  list every alias as `alias NAME='VALUE'`
//	alias NAME             print the binding for NAME (exit 1 if unset)
//	alias NAME=VALUE...    set bindings; NAMEs may be repeated
//
// Aliases live on Context.Aliases (a command.AliasTable shared
// across every dispatch on a single *Bash). Aliases only expand at
// parse time when `shopt expand_aliases` is on — the parse-side
// wiring lands in Phase 11; Wave G implements the read/write surface
// only.
package alias

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "alias [-p] [NAME[=VALUE]]..."

const helpText = `Usage: alias [-p] [NAME[=VALUE]]...
Define or display aliases.

With no arguments, list every alias. With NAME[=VALUE] operands,
define each alias. With NAMEs only, print each named alias's value
(exit 1 if any name is unset).

Options:
  -p          list every alias prefixed with "alias " (default)
  --help      show this help and exit
`

// New returns the alias command.
func New() command.Command { return command.Define("alias", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case a == "-p":
			// listing format flag — default behavior already
		case strings.HasPrefix(a, "-") && len(a) > 1 && !strings.Contains(a, "="):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	operands := args[i:]
	tbl := c.Aliases

	// No operands: list every alias.
	if len(operands) == 0 {
		listAll(c, tbl)
		return command.Result{ExitCode: 0}
	}

	// Walk operands; NAME=VALUE sets, NAME alone prints.
	exit := 0
	for _, op := range operands {
		if eq := strings.IndexByte(op, '='); eq > 0 {
			name := op[:eq]
			value := op[eq+1:]
			if !isAliasName(name) {
				_ = builtinutil.Errorf(c.Stderr, "alias", 1, "`%s': invalid alias name", name)
				exit = 1
				continue
			}
			tbl.Set(name, value)
			continue
		}
		// NAME: print if defined.
		v, ok := tbl.Get(op)
		if !ok {
			_ = builtinutil.Errorf(c.Stderr, "alias", 1, "%s: not found", op)
			exit = 1
			continue
		}
		if c.Stdout != nil {
			_, _ = fmt.Fprintf(c.Stdout, "alias %s='%s'\n", op, escapeSingleQuotes(v))
		}
	}
	return command.Result{ExitCode: exit}
}

func listAll(c *command.Context, tbl command.AliasTable) {
	if c.Stdout == nil {
		return
	}
	names := tbl.Names()
	for _, n := range names {
		v, _ := tbl.Get(n)
		_, _ = fmt.Fprintf(c.Stdout, "alias %s='%s'\n", n, escapeSingleQuotes(v))
	}
}

// escapeSingleQuotes returns s with each single-quote replaced by the
// bash idiom `'\''` so the surrounding `alias NAME='VALUE'` form
// remains parseable.
func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

// isAliasName accepts most punctuation (bash is more permissive than
// it is for env var names; e.g. `alias ll=ls` is fine and so is
// `alias .=cd`). Reject only names containing whitespace, '=', '/',
// or starting with '-'.
func isAliasName(name string) bool {
	if name == "" || name[0] == '-' {
		return false
	}
	for _, r := range name {
		switch r {
		case ' ', '\t', '\n', '=', '/':
			return false
		}
	}
	return true
}

func init() { command.RegisterBuiltin(New()) }
