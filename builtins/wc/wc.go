// Package wc implements the `wc` built-in.
//
// Flags: -l lines, -w words, -c bytes, -m chars, -L max-line-length.
package wc

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "wc [-lwcmL] [FILE...]"
const helpText = `Usage: wc [OPTION]... [FILE]...
Print newline, word, and byte counts for each FILE.

  -c, --bytes            print the byte counts
  -m, --chars            print the character counts
  -l, --lines            print the newline counts
  -L, --max-line-length  print the maximum display width
  -w, --words            print the word counts`

type opts struct {
	lines, words, bytes, chars, maxLine bool
}

type counts struct {
	lines, words, bytes, chars, maxLine int
}

// New returns the wc command.
func New() command.Command { return command.Define("wc", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var o opts
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-l", a == "--lines":
			o.lines = true
		case a == "-w", a == "--words":
			o.words = true
		case a == "-c", a == "--bytes":
			o.bytes = true
		case a == "-m", a == "--chars":
			o.chars = true
		case a == "-L", a == "--max-line-length":
			o.maxLine = true
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			if !bundle(a, &o) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			files = append(files, a)
		}
	}
run:
	// default: -l -w -c
	if !o.lines && !o.words && !o.bytes && !o.chars && !o.maxLine {
		o.lines, o.words, o.bytes = true, true, true
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	exit := 0
	var total counts
	for _, f := range files {
		cnts, err := count(c, f)
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "wc: %s: %v\n", f, err)
			exit = 1
			continue
		}
		total.lines += cnts.lines
		total.words += cnts.words
		total.bytes += cnts.bytes
		total.chars += cnts.chars
		if cnts.maxLine > total.maxLine {
			total.maxLine = cnts.maxLine
		}
		label := f
		if label == "-" {
			label = ""
		}
		printRow(c.Stdout, &o, cnts, label)
	}
	if len(files) > 1 {
		printRow(c.Stdout, &o, total, "total")
	}
	return command.Result{ExitCode: exit}
}

func printRow(w io.Writer, o *opts, c counts, label string) {
	parts := make([]string, 0, 5)
	if o.lines {
		parts = append(parts, fmt.Sprintf("%7d", c.lines))
	}
	if o.words {
		parts = append(parts, fmt.Sprintf("%7d", c.words))
	}
	if o.chars {
		parts = append(parts, fmt.Sprintf("%7d", c.chars))
	}
	if o.bytes {
		parts = append(parts, fmt.Sprintf("%7d", c.bytes))
	}
	if o.maxLine {
		parts = append(parts, fmt.Sprintf("%7d", c.maxLine))
	}
	line := strings.Join(parts, "")
	if label != "" {
		line += " " + label
	}
	_, _ = fmt.Fprintln(w, line)
}

func count(c *command.Context, name string) (counts, error) {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		return counts{}, err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return counts{}, err
	}
	var cn counts
	cn.bytes = len(data)
	cn.chars = utf8.RuneCount(data)
	// lines = number of \n
	curLine := 0
	inWord := false
	for i := 0; i < len(data); {
		r, sz := utf8.DecodeRune(data[i:])
		if r == '\n' {
			if curLine > cn.maxLine {
				cn.maxLine = curLine
			}
			cn.lines++
			curLine = 0
		} else {
			curLine++
		}
		if unicode.IsSpace(r) {
			if inWord {
				cn.words++
				inWord = false
			}
		} else {
			inWord = true
		}
		i += sz
	}
	if inWord {
		cn.words++
	}
	if curLine > cn.maxLine {
		cn.maxLine = curLine
	}
	return cn, nil
}

func bundle(a string, o *opts) bool {
	for _, ch := range a[1:] {
		switch ch {
		case 'l':
			o.lines = true
		case 'w':
			o.words = true
		case 'c':
			o.bytes = true
		case 'm':
			o.chars = true
		case 'L':
			o.maxLine = true
		default:
			return false
		}
	}
	return true
}

func init() { command.RegisterBuiltin(New()) }
