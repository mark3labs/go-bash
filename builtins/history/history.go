// Package history implements the `history` built-in (SPEC §10 Wave G).
//
// Usage:
//
//	history             list every recorded command, prefixed with
//	                    its 1-based sequence number
//	history N           list only the most recent N entries
//	history -c          clear the ring
//	history -d OFFSET   delete the entry at OFFSET
//	history -s CMD...   append CMD as a single new entry
//
// The ring lives on Context.History (a command.HistoryRing
// initialized at gobash.New time, default 500 entries). The runtime
// itself does NOT currently push parsed commands into the ring;
// hosts can populate it manually.
package history

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "history [-c] [-d OFFSET] [-s CMD...] [N]"

const helpText = `Usage: history [OPTIONS] [N]
List or modify the command history.

Options:
  -c              clear the entire history
  -d OFFSET       delete the entry at OFFSET
  -s CMD...       append CMD (joined by spaces) as a single entry
  --help          show this help and exit
`

// New returns the history command.
func New() command.Command { return command.Define("history", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	clear := false
	var del int
	delSet := false
	store := false

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
		case a == "-c":
			clear = true
		case a == "-d":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "history", 1, "%s: invalid offset", args[i])
			}
			del = n
			delSet = true
		case a == "-s":
			store = true
			i++
			goto done
		case strings.HasPrefix(a, "-") && len(a) > 1 && !isAllDigits(a[1:]):
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	ring := c.History
	rest := args[i:]

	if clear {
		ring.Clear()
		return command.Result{ExitCode: 0}
	}
	if store {
		if len(rest) == 0 {
			return command.Result{ExitCode: 0}
		}
		ring.Add(strings.Join(rest, " "))
		return command.Result{ExitCode: 0}
	}
	if delSet {
		// We don't implement true delete-by-offset since the ring is
		// append+evict only; report the unsupported request as exit 1.
		// Real bash supports it; punt to Phase 19 if a fixture needs it.
		_ = del
		return builtinutil.Errorf(c.Stderr, "history", 1, "-d not supported in this runtime")
	}

	// List up to limit entries.
	limit := -1
	if len(rest) > 0 {
		n, err := strconv.Atoi(rest[0])
		if err != nil || n < 0 {
			return builtinutil.Errorf(c.Stderr, "history", 1, "%s: numeric argument required", rest[0])
		}
		limit = n
	}
	seqs, cmds := ring.List()
	if limit >= 0 && limit < len(seqs) {
		seqs = seqs[len(seqs)-limit:]
		cmds = cmds[len(cmds)-limit:]
	}
	if c.Stdout != nil {
		for i, s := range seqs {
			_, _ = fmt.Fprintf(c.Stdout, "%5d  %s\n", s, cmds[i])
		}
	}
	return command.Result{ExitCode: 0}
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func init() { command.RegisterBuiltin(New()) }
