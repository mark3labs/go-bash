// Package cut implements the `cut` built-in (SPEC §10 Wave C).
//
// Flags:
//   -f LIST   field list
//   -c LIST   character list
//   -b LIST   byte list
//   -d DELIM  field delimiter (default TAB)
//   -s        suppress lines with no delimiters (field mode)
//   --complement
//   --output-delimiter=STR
package cut

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "cut (-b|-c|-f) LIST [-d DELIM] [-s] [--complement] [FILE...]"
const helpText = `Usage: cut OPTION... [FILE]...
Print selected parts of lines from each FILE to standard output.

  -b, --bytes=LIST    select only these bytes
  -c, --chars=LIST    select only these characters
  -f, --fields=LIST   select only these fields
  -d, --delimiter=DELIM   use DELIM as the field delimiter (default TAB)
  -s, --only-delimited    do not print lines not containing delimiters
      --complement   complement the set of selected bytes/chars/fields
      --output-delimiter=STR  use STR as the output delimiter`

type opts struct {
	mode       byte // 'b' 'c' 'f'
	list       []rng
	delim      string
	outDelim   string
	only       bool
	complement bool
}

type rng struct{ lo, hi int }

// New returns the cut command.
func New() command.Command { return command.Define("cut", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var o opts
	o.delim = "\t"
	var files []string
	listSet := false
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-f", a == "--fields":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.mode = 'f'
			var err error
			if o.list, err = parseRanges(args[i]); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case strings.HasPrefix(a, "--fields="):
			o.mode = 'f'
			var err error
			if o.list, err = parseRanges(strings.TrimPrefix(a, "--fields=")); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case a == "-c", a == "--chars":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.mode = 'c'
			var err error
			if o.list, err = parseRanges(args[i]); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case strings.HasPrefix(a, "--chars="):
			o.mode = 'c'
			var err error
			if o.list, err = parseRanges(strings.TrimPrefix(a, "--chars=")); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case a == "-b", a == "--bytes":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.mode = 'b'
			var err error
			if o.list, err = parseRanges(args[i]); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case strings.HasPrefix(a, "--bytes="):
			o.mode = 'b'
			var err error
			if o.list, err = parseRanges(strings.TrimPrefix(a, "--bytes=")); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case a == "-d", a == "--delimiter":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.delim = args[i]
		case strings.HasPrefix(a, "--delimiter="):
			o.delim = strings.TrimPrefix(a, "--delimiter=")
		case a == "-s", a == "--only-delimited":
			o.only = true
		case a == "--complement":
			o.complement = true
		case strings.HasPrefix(a, "--output-delimiter="):
			o.outDelim = strings.TrimPrefix(a, "--output-delimiter=")
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-f") && len(a) > 2:
			o.mode = 'f'
			var err error
			if o.list, err = parseRanges(a[2:]); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case strings.HasPrefix(a, "-c") && len(a) > 2:
			o.mode = 'c'
			var err error
			if o.list, err = parseRanges(a[2:]); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case strings.HasPrefix(a, "-b") && len(a) > 2:
			o.mode = 'b'
			var err error
			if o.list, err = parseRanges(a[2:]); err != nil {
				return builtinutil.Errorf(c.Stderr, "cut", 1, "bad list: %v", err)
			}
			listSet = true
		case strings.HasPrefix(a, "-d") && len(a) > 2:
			o.delim = a[2:]
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			files = append(files, a)
		}
	}
run:
	if !listSet {
		return builtinutil.Errorf(c.Stderr, "cut", 1, "you must specify a list of bytes, characters, or fields")
	}
	if o.outDelim == "" {
		o.outDelim = o.delim
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	exit := 0
	for _, f := range files {
		r, closer, err := builtinutil.OpenInput(c, f)
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "cut: %s: %v\n", f, err)
			exit = 1
			continue
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			out := processLine(line, &o)
			if out == nil {
				continue
			}
			_, _ = fmt.Fprintln(c.Stdout, *out)
		}
		if err := sc.Err(); err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "cut: %s: %v\n", f, err)
			exit = 1
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	return command.Result{ExitCode: exit}
}

func processLine(line string, o *opts) *string {
	switch o.mode {
	case 'f':
		if !strings.Contains(line, o.delim) {
			if o.only {
				return nil
			}
			return &line
		}
		fields := strings.Split(line, o.delim)
		selected := selectIdxs(len(fields), o.list, o.complement)
		out := make([]string, 0, len(selected))
		for _, idx := range selected {
			out = append(out, fields[idx])
		}
		s := strings.Join(out, o.outDelim)
		return &s
	case 'c':
		runes := []rune(line)
		selected := selectIdxs(len(runes), o.list, o.complement)
		out := make([]rune, 0, len(selected))
		for _, idx := range selected {
			out = append(out, runes[idx])
		}
		s := string(out)
		return &s
	case 'b':
		selected := selectIdxs(len(line), o.list, o.complement)
		out := make([]byte, 0, len(selected))
		for _, idx := range selected {
			out = append(out, line[idx])
		}
		s := string(out)
		return &s
	}
	return &line
}

// selectIdxs returns the 0-based indices (into a slice of length n)
// matching the 1-based ranges in list (or their complement).
func selectIdxs(n int, list []rng, complement bool) []int {
	picked := make([]bool, n)
	for _, r := range list {
		lo := r.lo
		hi := r.hi
		if lo < 1 {
			lo = 1
		}
		if hi == 0 || hi > n {
			hi = n
		}
		for i := lo; i <= hi; i++ {
			picked[i-1] = true
		}
	}
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if picked[i] != complement {
			out = append(out, i)
		}
	}
	return out
}

func parseRanges(spec string) ([]rng, error) {
	if spec == "" {
		return nil, fmt.Errorf("empty list")
	}
	var out []rng
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		dash := strings.Index(part, "-")
		switch {
		case dash < 0:
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid %q", part)
			}
			out = append(out, rng{n, n})
		case dash == 0:
			n, err := strconv.Atoi(part[1:])
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid %q", part)
			}
			out = append(out, rng{1, n})
		case dash == len(part)-1:
			n, err := strconv.Atoi(part[:dash])
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid %q", part)
			}
			out = append(out, rng{n, 0})
		default:
			lo, err := strconv.Atoi(part[:dash])
			if err != nil {
				return nil, err
			}
			hi, err := strconv.Atoi(part[dash+1:])
			if err != nil {
				return nil, err
			}
			out = append(out, rng{lo, hi})
		}
	}
	return out, nil
}

func init() { command.RegisterBuiltin(New()) }
