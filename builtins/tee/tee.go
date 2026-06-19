// Package tee implements the `tee` built-in.
//
// Flags: -a append, -i ignore SIGINT (no-op).
// Honors Context.Limits.MaxFileDescriptors — too many file operands
// is rejected as "too many open files".
package tee

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "tee [-ai] [FILE...]"
const helpText = `Usage: tee [OPTION]... [FILE]...
Copy standard input to each FILE, and also to standard output.

  -a, --append    append to the given FILEs, do not overwrite
  -i, --ignore-interrupts  no-op in sandbox`

// New returns the tee command.
func New() command.Command { return command.Define("tee", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	appendMode := false
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-a", a == "--append":
			appendMode = true
		case a == "-i", a == "--ignore-interrupts":
			// no-op
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			ok := true
			for _, ch := range a[1:] {
				switch ch {
				case 'a':
					appendMode = true
				case 'i':
				default:
					ok = false
				}
			}
			if !ok {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			files = append(files, a)
		}
	}
run:
	if c.FS == nil && len(files) > 0 {
		return builtinutil.Errorf(c.Stderr, "tee", 1, "no filesystem")
	}
	// Enforce MaxFileDescriptors (each open file is a FD; +1 for stdout).
	if c.Limits.MaxFileDescriptors > 0 && len(files)+1 > c.Limits.MaxFileDescriptors {
		return builtinutil.Errorf(c.Stderr, "tee", 1, "too many open files (limit %d)", c.Limits.MaxFileDescriptors)
	}
	writers := []io.Writer{c.Stdout}
	closers := []io.Closer{}
	defer func() {
		for _, cl := range closers {
			_ = cl.Close()
		}
	}()
	for _, f := range files {
		abs := builtinutil.ResolvePath(c.Cwd, f)
		var flag int
		if appendMode {
			flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		} else {
			flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		w, err := c.FS.OpenFile(abs, flag, 0o644)
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "tee: %s: %v\n", f, err)
			continue
		}
		writers = append(writers, w)
		closers = append(closers, w)
	}
	if c.Stdin == nil {
		return command.Result{}
	}
	mw := io.MultiWriter(writers...)
	if _, err := io.Copy(mw, c.Stdin); err != nil {
		return builtinutil.Errorf(c.Stderr, "tee", 1, "%v", err)
	}
	return command.Result{}
}

func init() { command.RegisterBuiltin(New()) }
