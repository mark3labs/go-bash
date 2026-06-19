// Package sort implements the `sort` built-in.
//
// Flags: -n numeric, -r reverse, -u unique, -k FIELD, -t SEP, -f
// fold-case, -V version, -h human-suffix-aware, -s stable, -c check,
// -m merge (treat all inputs as already sorted — we still sort), -z
// NUL-delimited records.
package sort

import (
	"context"
	stdsort "sort"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "sort [-nrufVhscmz] [-k FIELD] [-t SEP] [FILE...]"
const helpText = `Usage: sort [OPTION]... [FILE]...
Write sorted concatenation of all FILE(s) to standard output.

  -n, --numeric-sort       compare according to string numerical value
  -r, --reverse            reverse the result of comparisons
  -u, --unique             output only unique lines
  -k, --key=KEYDEF         sort via a key
  -t, --field-separator=SEP   use SEP instead of non-blank to blank transition
  -f, --ignore-case
  -V, --version-sort
  -h, --human-numeric-sort
  -s, --stable             stable sort
  -c, --check
  -m, --merge              merge already sorted files; do not sort
  -z, --zero-terminated    line delimiter is NUL, not newline`

type opts struct {
	numeric, reverse, unique, foldCase, versionSort, humanSort, stable, check, zero bool
	keyField                                                                       int
	keyEnd                                                                         int
	sep                                                                            string
}

// New returns the sort command.
func New() command.Command { return command.Define("sort", run) }

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
		case a == "-n", a == "--numeric-sort":
			o.numeric = true
		case a == "-r", a == "--reverse":
			o.reverse = true
		case a == "-u", a == "--unique":
			o.unique = true
		case a == "-f", a == "--ignore-case":
			o.foldCase = true
		case a == "-V", a == "--version-sort":
			o.versionSort = true
		case a == "-h", a == "--human-numeric-sort":
			o.humanSort = true
		case a == "-s", a == "--stable":
			o.stable = true
		case a == "-c", a == "--check":
			o.check = true
		case a == "-m", a == "--merge":
			// no-op (we still sort)
		case a == "-z", a == "--zero-terminated":
			o.zero = true
		case a == "-R", a == "--random-sort":
			// not supported; treat as no-op stable
		case a == "-k", a == "--key":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			parseKey(args[i], &o)
		case strings.HasPrefix(a, "--key="):
			parseKey(strings.TrimPrefix(a, "--key="), &o)
		case strings.HasPrefix(a, "-k") && len(a) > 2:
			parseKey(a[2:], &o)
		case a == "-t", a == "--field-separator":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.sep = args[i]
		case strings.HasPrefix(a, "--field-separator="):
			o.sep = strings.TrimPrefix(a, "--field-separator=")
		case strings.HasPrefix(a, "-t") && len(a) > 2:
			o.sep = a[2:]
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			if !bundle(a, &o) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			files = append(files, a)
		}
	}
run:
	if len(files) == 0 {
		files = []string{"-"}
	}
	// Read all input
	data, err := builtinutil.ReadAllInputs(c, files)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "sort", 2, "%v", err)
	}
	delim := byte('\n')
	if o.zero {
		delim = 0
	}
	lines := splitDelim(string(data), delim)
	if o.check {
		if !isSorted(lines, &o) {
			return builtinutil.Errorf(c.Stderr, "sort", 1, "disorder")
		}
		return command.Result{}
	}
	stdsort.SliceStable(lines, func(a, b int) bool {
		return less(lines[a], lines[b], &o)
	})
	if o.unique {
		lines = uniq(lines, &o)
	}
	for _, line := range lines {
		_, _ = c.Stdout.Write([]byte(line))
		_, _ = c.Stdout.Write([]byte{delim})
	}
	return command.Result{}
}

func parseKey(s string, o *opts) {
	// FIELD[,FIELD] simplified (we ignore the .CHAR sub-position).
	parts := strings.SplitN(s, ",", 2)
	if n, err := strconv.Atoi(strings.SplitN(parts[0], ".", 2)[0]); err == nil {
		o.keyField = n
	}
	if len(parts) > 1 {
		if n, err := strconv.Atoi(strings.SplitN(parts[1], ".", 2)[0]); err == nil {
			o.keyEnd = n
		}
	}
}

func bundle(a string, o *opts) bool {
	for _, ch := range a[1:] {
		switch ch {
		case 'n':
			o.numeric = true
		case 'r':
			o.reverse = true
		case 'u':
			o.unique = true
		case 'f':
			o.foldCase = true
		case 'V':
			o.versionSort = true
		case 'h':
			o.humanSort = true
		case 's':
			o.stable = true
		case 'c':
			o.check = true
		case 'm':
		case 'z':
			o.zero = true
		default:
			return false
		}
	}
	return true
}

func splitDelim(s string, delim byte) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == delim {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func less(a, b string, o *opts) bool {
	ka, kb := key(a, o), key(b, o)
	cmp := compare(ka, kb, o)
	if o.reverse {
		cmp = -cmp
	}
	return cmp < 0
}

func key(s string, o *opts) string {
	if o.keyField == 0 {
		return s
	}
	sep := o.sep
	var fields []string
	if sep == "" {
		fields = strings.Fields(s)
	} else {
		fields = strings.Split(s, sep)
	}
	if o.keyField > len(fields) {
		return ""
	}
	start := o.keyField - 1
	end := o.keyEnd
	if end == 0 || end > len(fields) {
		end = len(fields)
	}
	joiner := sep
	if joiner == "" {
		joiner = " "
	}
	return strings.Join(fields[start:end], joiner)
}

func compare(a, b string, o *opts) int {
	if o.foldCase {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	if o.numeric {
		fa, _ := strconv.ParseFloat(strings.TrimSpace(a), 64)
		fb, _ := strconv.ParseFloat(strings.TrimSpace(b), 64)
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		}
		return strings.Compare(a, b)
	}
	if o.humanSort {
		fa := humanParse(a)
		fb := humanParse(b)
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		}
		return strings.Compare(a, b)
	}
	if o.versionSort {
		return versionCompare(a, b)
	}
	return strings.Compare(a, b)
}

func humanParse(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	end := len(s)
	mult := 1.0
	if end > 0 {
		switch s[end-1] {
		case 'K', 'k':
			mult = 1024
			end--
		case 'M':
			mult = 1024 * 1024
			end--
		case 'G':
			mult = 1024 * 1024 * 1024
			end--
		case 'T':
			mult = 1024 * 1024 * 1024 * 1024
			end--
		case 'P':
			mult = 1024 * 1024 * 1024 * 1024 * 1024
			end--
		}
	}
	v, _ := strconv.ParseFloat(s[:end], 64)
	return v * mult
}

// versionCompare implements a semver-ish ordering: split on
// non-alphanumeric boundaries, compare numeric parts numerically and
// alpha parts lexically.
func versionCompare(a, b string) int {
	ta, tb := tokVersion(a), tokVersion(b)
	for i := 0; i < len(ta) && i < len(tb); i++ {
		na, isNumA := atoiOK(ta[i])
		nb, isNumB := atoiOK(tb[i])
		switch {
		case isNumA && isNumB:
			switch {
			case na < nb:
				return -1
			case na > nb:
				return 1
			}
		case isNumA:
			return -1
		case isNumB:
			return 1
		default:
			if c := strings.Compare(ta[i], tb[i]); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(ta) < len(tb):
		return -1
	case len(ta) > len(tb):
		return 1
	}
	return 0
}

func tokVersion(s string) []string {
	var out []string
	i := 0
	for i < len(s) {
		// digit run
		if s[i] >= '0' && s[i] <= '9' {
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			out = append(out, s[i:j])
			i = j
			continue
		}
		// alpha run
		if isAlpha(s[i]) {
			j := i
			for j < len(s) && isAlpha(s[j]) {
				j++
			}
			out = append(out, s[i:j])
			i = j
			continue
		}
		i++ // skip non-alphanumeric
	}
	return out
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func atoiOK(s string) (int64, bool) {
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

func isSorted(lines []string, o *opts) bool {
	for i := 1; i < len(lines); i++ {
		if less(lines[i], lines[i-1], o) {
			return false
		}
	}
	return true
}

func uniq(lines []string, o *opts) []string {
	if len(lines) == 0 {
		return lines
	}
	out := lines[:1]
	for i := 1; i < len(lines); i++ {
		if compare(key(lines[i], o), key(out[len(out)-1], o), o) != 0 {
			out = append(out, lines[i])
		}
	}
	return out
}

func init() { command.RegisterBuiltin(New()) }
