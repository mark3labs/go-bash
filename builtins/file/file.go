// Package file implements the `file` built-in (SPEC §10 Wave B).
//
// Provides a small subset of `file(1)`'s detection: directories,
// symlinks, empty files, ASCII text, UTF-8 text, and binary data.
// For richer mime detection a future Phase can introduce a mimetype
// dependency; today the detection is hand-rolled to avoid pulling in
// CGO or large data tables.
package file

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "file [-b] FILE..."
const helpText = `Usage: file [OPTION]... FILE...
Determine the type of FILE(s).

  -b, --brief    do not prepend filenames to output lines`

// New returns the file command.
func New() command.Command { return command.Define("file", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	brief := false
	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-b", a == "--brief":
			brief = true
		case a == "--":
			i++
			paths = append(paths, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			paths = append(paths, a)
		}
	}
run:
	if len(paths) == 0 {
		return builtinutil.Errorf(c.Stderr, "file", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "file", 1, "no filesystem")
	}
	for _, p := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, p)
		desc := describe(c, abs)
		if c.Stdout != nil {
			if brief {
				_, _ = fmt.Fprintln(c.Stdout, desc)
			} else {
				_, _ = fmt.Fprintf(c.Stdout, "%s: %s\n", p, desc)
			}
		}
	}
	return command.Result{}
}

func describe(c *command.Context, p string) string {
	fi, err := c.FS.Lstat(p)
	if err != nil {
		return fmt.Sprintf("cannot open: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, _ := c.FS.Readlink(p)
		return fmt.Sprintf("symbolic link to %s", target)
	}
	if fi.IsDir() {
		return "directory"
	}
	if fi.Size() == 0 {
		return "empty"
	}
	data, err := c.FS.ReadFile(p)
	if err != nil {
		return fmt.Sprintf("cannot read: %v", err)
	}
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	return classify(sample)
}

func classify(data []byte) string {
	if isASCII(data) {
		return "ASCII text"
	}
	if utf8.Valid(data) {
		return "UTF-8 Unicode text"
	}
	return "data"
}

func isASCII(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 {
			return false
		}
		if b < 0x09 || (b > 0x0d && b < 0x20) || b == 0x7f {
			return false
		}
	}
	return true
}

func init() { command.RegisterBuiltin(New()) }
