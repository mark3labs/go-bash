// Package md5sum implements the `md5sum` built-in (SPEC §10 Wave C).
package md5sum

import (
	"context"
	"crypto/md5" //nolint:gosec // md5sum is a checksum tool, not a security primitive
	"fmt"
	"hash"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "md5sum [-bct] [FILE...]"
const helpText = `Usage: md5sum [OPTION]... [FILE]...
Print or check MD5 (128-bit) checksums.

  -b, --binary    read in binary mode (no-op in sandbox)
  -t, --text      read in text mode (default)
  -c, --check     read sums from FILEs and check them`

// New returns the md5sum command.
func New() command.Command { return command.Define("md5sum", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	return Sum(c, args, "md5sum", func() hash.Hash { return md5.New() })
}

// Sum is the shared implementation used by md5sum / sha1sum / sha256sum.
func Sum(c *command.Context, args []string, cmd string, h func() hash.Hash) command.Result {
	check := false
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-b", a == "--binary", a == "-t", a == "--text":
			// no-op
		case a == "-c", a == "--check":
			check = true
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			ok := true
			for _, ch := range a[1:] {
				switch ch {
				case 'b', 't':
				case 'c':
					check = true
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
	if check {
		return runCheck(c, files, cmd, h)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	exit := 0
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmd, f, err)
			continue
		}
		hh := h()
		if _, err := io.Copy(hh, r); err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmd, f, err)
		}
		if closer != nil {
			_ = closer.Close()
		}
		name := f
		if name == "-" {
			name = "-"
		}
		_, _ = fmt.Fprintf(c.Stdout, "%x  %s\n", hh.Sum(nil), name)
	}
	return command.Result{ExitCode: exit}
}

func runCheck(c *command.Context, files []string, cmd string, h func() hash.Hash) command.Result {
	if len(files) == 0 {
		files = []string{"-"}
	}
	failed := 0
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, cmd, 1, "%s: %v", f, err)
		}
		data, _ := io.ReadAll(r)
		if closer != nil {
			_ = closer.Close()
		}
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "  ", 2)
			if len(parts) != 2 {
				continue
			}
			want := parts[0]
			name := parts[1]
			r2, c2, err := builtinutil.OpenInput(c, name)
			if err != nil {
				_, _ = fmt.Fprintf(c.Stdout, "%s: FAILED open or read\n", name)
				failed++
				continue
			}
			hh := h()
			_, _ = io.Copy(hh, r2)
			if c2 != nil {
				_ = c2.Close()
			}
			got := fmt.Sprintf("%x", hh.Sum(nil))
			if got == want {
				_, _ = fmt.Fprintf(c.Stdout, "%s: OK\n", name)
			} else {
				_, _ = fmt.Fprintf(c.Stdout, "%s: FAILED\n", name)
				failed++
			}
		}
	}
	if failed > 0 {
		return command.Result{ExitCode: 1}
	}
	return command.Result{}
}

func init() { command.RegisterBuiltin(New()) }
