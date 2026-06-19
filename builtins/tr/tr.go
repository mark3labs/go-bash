// Package tr implements the `tr` built-in.
//
// Supports character classes ([:alpha:], [:digit:], [:upper:],
// [:lower:], [:space:], [:punct:], [:xdigit:], [:alnum:], [:cntrl:],
// [:print:], [:graph:], [:blank:]), ranges (a-z), explicit lists,
// and flags -d delete, -s squeeze, -c complement, -C (== -c).
package tr

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "tr [-cCdst] SET1 [SET2]"
const helpText = `Usage: tr [OPTION]... SET1 [SET2]
Translate, squeeze, and/or delete characters from standard input.

  -c, -C, --complement      use the complement of SET1
  -d, --delete              delete characters in SET1
  -s, --squeeze-repeats     squeeze repeats of characters in SET1 (or SET2 when translating)
  -t, --truncate-set1       first truncate SET1 to length of SET2`

type opts struct {
	complement, deleteMode, squeeze, truncate bool
}

// New returns the tr command.
func New() command.Command { return command.Define("tr", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var o opts
	var pos []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-c", a == "-C", a == "--complement":
			o.complement = true
		case a == "-d", a == "--delete":
			o.deleteMode = true
		case a == "-s", a == "--squeeze-repeats":
			o.squeeze = true
		case a == "-t", a == "--truncate-set1":
			o.truncate = true
		case a == "--":
			i++
			pos = append(pos, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			if !bundle(a, &o) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			pos = append(pos, a)
		}
	}
run:
	if len(pos) < 1 {
		return builtinutil.Errorf(c.Stderr, "tr", 1, "missing operand")
	}
	set1, err := expandSet(pos[0])
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tr", 1, "bad SET1: %v", err)
	}
	var set2 []rune
	if len(pos) >= 2 {
		set2, err = expandSet(pos[1])
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "tr", 1, "bad SET2: %v", err)
		}
	}
	if c.Stdin == nil {
		return command.Result{}
	}
	data, err := io.ReadAll(c.Stdin)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tr", 1, "read: %v", err)
	}
	out := transform(string(data), set1, set2, &o)
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, out)
	}
	return command.Result{}
}

func transform(s string, set1, set2 []rune, o *opts) string {
	set1Map := make(map[rune]int, len(set1))
	for i, r := range set1 {
		set1Map[r] = i
	}
	inSet1 := func(r rune) bool {
		_, ok := set1Map[r]
		if o.complement {
			return !ok
		}
		return ok
	}

	if o.deleteMode && len(set2) == 0 {
		var b strings.Builder
		var last rune
		hasLast := false
		for _, r := range s {
			if inSet1(r) {
				continue
			}
			if o.squeeze && hasLast && r == last {
				continue
			}
			b.WriteRune(r)
			last = r
			hasLast = true
		}
		return b.String()
	}

	// Translation mode. Pad set2 with its last char if shorter; or
	// truncate set1 to set2 length if -t.
	if o.truncate && len(set2) > 0 && len(set1) > len(set2) {
		set1 = set1[:len(set2)]
		set1Map = make(map[rune]int, len(set1))
		for i, r := range set1 {
			set1Map[r] = i
		}
	}
	var lastByte byte // unused
	_ = lastByte
	var b strings.Builder
	var prevOut rune
	hasPrev := false
	set2Map := make(map[rune]struct{}, len(set2))
	for _, r := range set2 {
		set2Map[r] = struct{}{}
	}
	for _, r := range s {
		mapped := r
		if o.complement {
			if _, ok := set1Map[r]; !ok && len(set2) > 0 {
				mapped = set2[len(set2)-1]
			}
		} else {
			if idx, ok := set1Map[r]; ok && len(set2) > 0 {
				if idx >= len(set2) {
					mapped = set2[len(set2)-1]
				} else {
					mapped = set2[idx]
				}
			}
		}
		if o.squeeze {
			squeezable := false
			if len(set2) > 0 {
				if _, ok := set2Map[mapped]; ok {
					squeezable = true
				}
			} else if inSet1(mapped) {
				squeezable = true
			}
			if squeezable && hasPrev && prevOut == mapped {
				continue
			}
		}
		b.WriteRune(mapped)
		prevOut = mapped
		hasPrev = true
	}
	return b.String()
}

func bundle(a string, o *opts) bool {
	for _, ch := range a[1:] {
		switch ch {
		case 'c', 'C':
			o.complement = true
		case 'd':
			o.deleteMode = true
		case 's':
			o.squeeze = true
		case 't':
			o.truncate = true
		default:
			return false
		}
	}
	return true
}

// expandSet returns the rune sequence the SET literal represents:
// class names (e.g. [:digit:]), ranges (a-z), backslash escapes
// (\n, \t, \\, \0nnn), and literal chars.
func expandSet(s string) ([]rune, error) {
	var out []rune
	rs := []rune(s)
	i := 0
	for i < len(rs) {
		// class [:name:]
		if rs[i] == '[' && i+1 < len(rs) && rs[i+1] == ':' {
			end := i + 2
			for end < len(rs)-1 && (rs[end] != ':' || rs[end+1] != ']') {
				end++
			}
			if end < len(rs)-1 {
				name := string(rs[i+2 : end])
				out = append(out, classRunes(name)...)
				i = end + 2
				continue
			}
		}
		// range
		if i+2 < len(rs) && rs[i+1] == '-' && rs[i+2] != ']' {
			lo := rs[i]
			hi := rs[i+2]
			for r := lo; r <= hi; r++ {
				out = append(out, r)
			}
			i += 3
			continue
		}
		// escape
		if rs[i] == '\\' && i+1 < len(rs) {
			esc, n := decodeEscape(rs[i:])
			out = append(out, esc)
			i += n
			continue
		}
		out = append(out, rs[i])
		i++
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty set")
	}
	return out, nil
}

func decodeEscape(s []rune) (rune, int) {
	if len(s) < 2 {
		return s[0], 1
	}
	switch s[1] {
	case 'n':
		return '\n', 2
	case 't':
		return '\t', 2
	case 'r':
		return '\r', 2
	case '\\':
		return '\\', 2
	case 'a':
		return '\a', 2
	case 'b':
		return '\b', 2
	case 'f':
		return '\f', 2
	case 'v':
		return '\v', 2
	case '0':
		// octal up to 3 digits
		val := 0
		j := 2
		for ; j < len(s) && j < 5 && s[j] >= '0' && s[j] <= '7'; j++ {
			val = val*8 + int(s[j]-'0')
		}
		return rune(val), j
	}
	return s[1], 2
}

func classRunes(name string) []rune {
	var out []rune
	for r := rune(0); r < 256; r++ {
		switch name {
		case "alpha":
			if unicode.IsLetter(r) {
				out = append(out, r)
			}
		case "alnum":
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, r)
			}
		case "digit":
			if r >= '0' && r <= '9' {
				out = append(out, r)
			}
		case "upper":
			if r >= 'A' && r <= 'Z' {
				out = append(out, r)
			}
		case "lower":
			if r >= 'a' && r <= 'z' {
				out = append(out, r)
			}
		case "space":
			if unicode.IsSpace(r) {
				out = append(out, r)
			}
		case "blank":
			if r == ' ' || r == '\t' {
				out = append(out, r)
			}
		case "punct":
			if unicode.IsPunct(r) || unicode.IsSymbol(r) {
				out = append(out, r)
			}
		case "xdigit":
			if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
				out = append(out, r)
			}
		case "cntrl":
			if unicode.IsControl(r) {
				out = append(out, r)
			}
		case "print":
			if unicode.IsPrint(r) {
				out = append(out, r)
			}
		case "graph":
			if unicode.IsGraphic(r) && !unicode.IsSpace(r) {
				out = append(out, r)
			}
		}
	}
	return out
}

func init() { command.RegisterBuiltin(New()) }
