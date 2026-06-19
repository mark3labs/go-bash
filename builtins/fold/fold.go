// Package fold implements the `fold` built-in.
//
// Flags: -w WIDTH, -s break at spaces, -b count bytes (default chars).
package fold

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "fold [-bs] [-w WIDTH] [FILE...]"
const helpText = `Usage: fold [OPTION]... [FILE]...
Wrap input lines in each FILE.

  -b, --bytes      count bytes rather than columns
  -s, --spaces     break at spaces
  -w, --width=WIDTH   use WIDTH columns instead of 80`

// New returns the fold command.
func New() command.Command { return command.Define("fold", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	width := 80
	bytesMode := false
	atSpaces := false
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-w", a == "--width":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			w, err := strconv.Atoi(args[i])
			if err != nil || w < 1 {
				return builtinutil.Errorf(c.Stderr, "fold", 1, "invalid width")
			}
			width = w
		case strings.HasPrefix(a, "--width="):
			w, err := strconv.Atoi(strings.TrimPrefix(a, "--width="))
			if err != nil || w < 1 {
				return builtinutil.Errorf(c.Stderr, "fold", 1, "invalid width")
			}
			width = w
		case strings.HasPrefix(a, "-w") && len(a) > 2:
			w, err := strconv.Atoi(a[2:])
			if err != nil || w < 1 {
				return builtinutil.Errorf(c.Stderr, "fold", 1, "invalid width")
			}
			width = w
		case a == "-b", a == "--bytes":
			bytesMode = true
		case a == "-s", a == "--spaces":
			atSpaces = true
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			// numeric short like -80
			if n, err := strconv.Atoi(a[1:]); err == nil && n > 0 {
				width = n
				break
			}
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			files = append(files, a)
		}
	}
run:
	if len(files) == 0 {
		files = []string{"-"}
	}
	exit := 0
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "fold: %s: %v\n", f, err)
			exit = 1
			continue
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			foldLine(c.Stdout.Write, sc.Text(), width, bytesMode, atSpaces)
		}
		if err := sc.Err(); err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "fold: %s: %v\n", f, err)
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	return command.Result{ExitCode: exit}
}

func foldLine(write func([]byte) (int, error), line string, width int, bytesMode, atSpaces bool) {
	if bytesMode {
		b := []byte(line)
		for len(b) > width {
			cut := width
			if atSpaces {
				if idx := lastSpaceByte(b[:cut]); idx >= 0 {
					cut = idx + 1
				}
			}
			_, _ = write(b[:cut])
			_, _ = write([]byte("\n"))
			b = b[cut:]
		}
		_, _ = write(b)
		_, _ = write([]byte("\n"))
		return
	}
	rs := []rune(line)
	for len(rs) > width {
		cut := width
		if atSpaces {
			if idx := lastSpaceRune(rs[:cut]); idx >= 0 {
				cut = idx + 1
			}
		}
		_, _ = write([]byte(string(rs[:cut])))
		_, _ = write([]byte("\n"))
		rs = rs[cut:]
	}
	_, _ = write([]byte(string(rs)))
	_, _ = write([]byte("\n"))
}

func lastSpaceByte(b []byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == ' ' || b[i] == '\t' {
			return i
		}
	}
	return -1
}

func lastSpaceRune(rs []rune) int {
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] == ' ' || rs[i] == '\t' {
			return i
		}
	}
	return -1
}

func init() { command.RegisterBuiltin(New()) }
