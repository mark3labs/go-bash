// Package join implements the `join` built-in.
//
// Flags: -1 N -2 N, -t SEP, -a FILE (print unpairable from FILE),
// -e EMPTY, -o FORMAT (FIELD spec or 'auto').
package join

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "join [-1 N] [-2 N] [-t SEP] [-a 1|2] [-e EMPTY] [-o FORMAT] FILE1 FILE2"
const helpText = `Usage: join [OPTION]... FILE1 FILE2
For each pair of input lines with identical join fields, write to standard output a line containing the join field followed by the rest of the lines from each file.

  -1 N            join on FIELD of FILE1 (default 1)
  -2 N            join on FIELD of FILE2 (default 1)
  -t CHAR         use CHAR as input and output field separator
  -a NUM          print unpairable lines from FILE NUM
  -e EMPTY        replace missing fields with EMPTY
  -o FORMAT       obey FORMAT while constructing output line`

type opts struct {
	f1, f2  int
	sep     string
	empty   string
	format  string
	unpair1 bool
	unpair2 bool
}

// New returns the join command.
func New() command.Command { return command.Define("join", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	o := opts{f1: 1, f2: 1, sep: " "}
	var pos []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-1":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.f1, _ = strconv.Atoi(args[i])
		case a == "-2":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.f2, _ = strconv.Atoi(args[i])
		case a == "-t":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.sep = args[i]
		case strings.HasPrefix(a, "-t") && len(a) > 2:
			o.sep = a[2:]
		case a == "-e":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.empty = args[i]
		case a == "-o":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.format = args[i]
		case a == "-a":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			switch args[i] {
			case "1":
				o.unpair1 = true
			case "2":
				o.unpair2 = true
			}
		case a == "--":
			i++
			pos = append(pos, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			pos = append(pos, a)
		}
	}
run:
	if len(pos) != 2 {
		return builtinutil.Errorf(c.Stderr, "join", 1, "need exactly two files")
	}
	a, err := readLines(c, pos[0])
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "join", 1, "%s: %v", pos[0], err)
	}
	b, err := readLines(c, pos[1])
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "join", 1, "%s: %v", pos[1], err)
	}
	splitOf := func(s string) []string {
		if o.sep == " " {
			return strings.Fields(s)
		}
		return strings.Split(s, o.sep)
	}
	keyOf := func(fields []string, n int) string {
		if n-1 >= 0 && n-1 < len(fields) {
			return fields[n-1]
		}
		return ""
	}
	joiner := o.sep
	if joiner == " " {
		joiner = " "
	}
	writeJoined := func(key string, fa, fb []string) {
		if o.format != "" {
			parts := strings.Split(o.format, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "0" {
					out = append(out, key)
					continue
				}
				ss := strings.Split(p, ".")
				if len(ss) == 2 {
					n, _ := strconv.Atoi(ss[1])
					var src []string
					if ss[0] == "1" {
						src = fa
					} else {
						src = fb
					}
					if n-1 < len(src) {
						out = append(out, src[n-1])
					} else {
						out = append(out, o.empty)
					}
				}
			}
			_, _ = fmt.Fprintln(c.Stdout, strings.Join(out, joiner))
			return
		}
		out := []string{key}
		for i, v := range fa {
			if i+1 == o.f1 {
				continue
			}
			out = append(out, v)
		}
		for i, v := range fb {
			if i+1 == o.f2 {
				continue
			}
			out = append(out, v)
		}
		_, _ = fmt.Fprintln(c.Stdout, strings.Join(out, joiner))
	}
	// Assume sorted by key (lexically).
	i2, j2 := 0, 0
	for i2 < len(a) && j2 < len(b) {
		fa := splitOf(a[i2])
		fb := splitOf(b[j2])
		ka := keyOf(fa, o.f1)
		kb := keyOf(fb, o.f2)
		switch {
		case ka < kb:
			if o.unpair1 {
				_, _ = fmt.Fprintln(c.Stdout, a[i2])
			}
			i2++
		case ka > kb:
			if o.unpair2 {
				_, _ = fmt.Fprintln(c.Stdout, b[j2])
			}
			j2++
		default:
			writeJoined(ka, fa, fb)
			i2++
			j2++
		}
	}
	for ; i2 < len(a); i2++ {
		if o.unpair1 {
			_, _ = fmt.Fprintln(c.Stdout, a[i2])
		}
	}
	for ; j2 < len(b); j2++ {
		if o.unpair2 {
			_, _ = fmt.Fprintln(c.Stdout, b[j2])
		}
	}
	return command.Result{}
}

func readLines(c *command.Context, name string) ([]string, error) {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out []string
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

func init() { command.RegisterBuiltin(New()) }
