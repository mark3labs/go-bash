// Package htmltomarkdown implements the `html-to-markdown` built-in
// (SPEC §10 Wave H). It reads HTML from stdin or the first positional
// file argument and writes CommonMark Markdown to stdout. The
// conversion is delegated to github.com/JohannesKaufmann/html-to-markdown/v2
// (BSD-3-Clause, pure-Go).
//
// All file I/O routes through Context.FS; stdin / stdout via
// Context.Stdin / Context.Stdout. The command takes no network access
// and is always registered (unlike `curl`).
package htmltomarkdown

import (
	"context"
	"io"
	"strings"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const cmdName = "html-to-markdown"
const usage = cmdName + " [FILE]"

const helpText = `Usage: html-to-markdown [FILE]
Convert HTML to CommonMark Markdown.

With no FILE (or FILE='-'), read HTML from stdin. Output is written
to stdout. File I/O is routed through the virtual filesystem.

Options:
  --help     show this help and exit
`

// New returns the html-to-markdown command.
func New() command.Command { return command.Define(cmdName, run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var (
		file    string
		hasFile bool
	)
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto rest
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto rest
		}
	}
rest:
	if i < len(args) {
		file = args[i]
		hasFile = true
		// Extra positionals are ignored — matches the just-bash
		// reference shape ("first positional file" only).
	}

	htmlBytes, err := readInput(c, file, hasFile)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "%v", err)
	}

	md, err := htmltomd.ConvertString(string(htmlBytes))
	if err != nil {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "%v", err)
	}
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, md)
	}
	return command.Result{ExitCode: 0}
}

// readInput returns the HTML source for the conversion. When hasFile
// is true and file != "-", we load it through c.FS; otherwise we
// consume the entire c.Stdin stream.
func readInput(c *command.Context, file string, hasFile bool) ([]byte, error) {
	if hasFile && file != "-" {
		if c.FS == nil {
			return nil, ioErr("no filesystem available")
		}
		path := builtinutil.ResolvePath(c.Cwd, file)
		return c.FS.ReadFile(path)
	}
	if c.Stdin == nil {
		return nil, nil
	}
	return io.ReadAll(c.Stdin)
}

// ioErr is a tiny convenience that returns a single-line error
// without dragging in fmt for one allocation.
type ioErr string

func (e ioErr) Error() string { return string(e) }

func init() { command.RegisterBuiltin(New()) }
