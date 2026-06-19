// Package split implements the `split` built-in (SPEC §10 Wave B).
//
// Flags: -l N (lines per file), -b N[K|M|G] (bytes per file),
// -a SUFFIXLEN, -d (numeric suffixes).
package split

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "split [-l N | -b N[K|M|G]] [-a SUFFIXLEN] [-d] [FILE [PREFIX]]"
const helpText = `Usage: split [OPTION]... [FILE [PREFIX]]
Output pieces of FILE to PREFIXaa, PREFIXab, ...

  -b, --bytes=SIZE     put SIZE bytes per output file
  -l, --lines=NUMBER   put NUMBER lines per output file (default 1000)
  -a, --suffix-length=N use suffixes of length N (default 2)
  -d                    use numeric suffixes starting at 0`

// New returns the split command.
func New() command.Command { return command.Define("split", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	mode := "lines"
	lines := int64(1000)
	bytesN := int64(0)
	suffixLen := 2
	numeric := false
	var positional []string

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-l", a == "--lines":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "split", 1, "invalid number of lines: %s", args[i])
			}
			mode, lines = "lines", n
		case strings.HasPrefix(a, "--lines="):
			n, err := strconv.ParseInt(strings.TrimPrefix(a, "--lines="), 10, 64)
			if err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "split", 1, "invalid lines")
			}
			mode, lines = "lines", n
		case a == "-b", a == "--bytes":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := parseSize(args[i])
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "split", 1, "invalid byte count: %s", args[i])
			}
			mode, bytesN = "bytes", n
		case strings.HasPrefix(a, "--bytes="):
			n, err := parseSize(strings.TrimPrefix(a, "--bytes="))
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "split", 1, "invalid byte count")
			}
			mode, bytesN = "bytes", n
		case a == "-a", a == "--suffix-length":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "split", 1, "invalid suffix length: %s", args[i])
			}
			suffixLen = n
		case a == "-d", a == "--numeric-suffixes":
			numeric = true
		case a == "--":
			i++
			positional = append(positional, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			positional = append(positional, a)
		}
	}
run:
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "split", 1, "no filesystem")
	}
	var src io.Reader
	if len(positional) == 0 || positional[0] == "-" {
		src = c.Stdin
		if src == nil {
			src = bytes.NewReader(nil)
		}
	} else {
		data, err := c.FS.ReadFile(builtinutil.ResolvePath(c.Cwd, positional[0]))
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "split", 1, "cannot open %q: %v", positional[0], err)
		}
		src = bytes.NewReader(data)
	}
	prefix := "x"
	if len(positional) >= 2 {
		prefix = positional[1]
	}

	if mode == "bytes" {
		return splitBytes(c, src, prefix, bytesN, suffixLen, numeric)
	}
	return splitLines(c, src, prefix, lines, suffixLen, numeric)
}

func splitLines(c *command.Context, src io.Reader, prefix string, n int64, suffixLen int, numeric bool) command.Result {
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var buf bytes.Buffer
	var count int64
	idx := 0
	flush := func() error {
		if buf.Len() == 0 {
			return nil
		}
		name := prefix + suffix(idx, suffixLen, numeric)
		idx++
		path := builtinutil.ResolvePath(c.Cwd, name)
		if err := c.FS.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			return err
		}
		buf.Reset()
		return nil
	}
	for scanner.Scan() {
		buf.Write(scanner.Bytes())
		buf.WriteByte('\n')
		count++
		if count >= n {
			if err := flush(); err != nil {
				return builtinutil.Errorf(c.Stderr, "split", 1, "%v", err)
			}
			count = 0
		}
	}
	if err := flush(); err != nil {
		return builtinutil.Errorf(c.Stderr, "split", 1, "%v", err)
	}
	return command.Result{}
}

func splitBytes(c *command.Context, src io.Reader, prefix string, n int64, suffixLen int, numeric bool) command.Result {
	buf := make([]byte, n)
	idx := 0
	for {
		got, err := io.ReadFull(src, buf)
		if got > 0 {
			name := prefix + suffix(idx, suffixLen, numeric)
			idx++
			path := builtinutil.ResolvePath(c.Cwd, name)
			if werr := c.FS.WriteFile(path, buf[:got], 0o644); werr != nil {
				return builtinutil.Errorf(c.Stderr, "split", 1, "%v", werr)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return builtinutil.Errorf(c.Stderr, "split", 1, "%v", err)
		}
	}
	return command.Result{}
}

func suffix(i, n int, numeric bool) string {
	if numeric {
		return fmt.Sprintf("%0*d", n, i)
	}
	out := make([]byte, n)
	for k := n - 1; k >= 0; k-- {
		out[k] = byte('a' + i%26)
		i /= 26
	}
	return string(out)
}

func parseSize(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	mult := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'K', 'k':
		mult = 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'G', 'g':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("bad size")
	}
	return n * mult, nil
}

func init() { command.RegisterBuiltin(New()) }
