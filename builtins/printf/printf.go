// Package printf implements the `printf` built-in (/ §10.18 details).
//
// The implementation handles the bash extensions Go's fmt does not:
//
//   - %b  argument with backslash escapes interpreted (like echo -e)
//   - %q  argument shell-quoted (single-quoted with escapes for ')
//   - %(FMT)T strftime-style timestamp; arg = epoch seconds, -1 = now, -2 = shell start
//
// And the standard conversions %d, %i, %o, %u, %x, %X, %c, %s, %f,
// %e, %E, %g, %G, %%, with the usual flag/width/precision syntax.
// Bash format-reuse over extra args is implemented by cycling the
// format string until all args are consumed (printf "%s\n" a b c
// emits three lines).
//
// mvdan/sh has its own printf builtin handled before our ExecHandler,
// so a bare `printf` invocation in a script never reaches us. This
// built-in serves `/bin/printf`, `command printf`, and direct
// dispatch via the registry for tests.
package printf

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "printf FORMAT [ARG...]"
const helpText = `Usage: printf FORMAT [ARG...]
Format and print ARG values according to FORMAT.

Bash extensions:
  %b    interpret backslash escapes in ARG (like echo -e)
  %q    quote ARG so the shell can re-read it
  %(FMT)T  format ARG as a timestamp using strftime; arg = epoch
           seconds (-1 = now, -2 = shell start)`

// New returns the printf command.
func New() command.Command {
	return command.Define("printf", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	if len(args) >= 2 && args[1] == "--help" {
		builtinutil.PrintHelp(c.Stdout, helpText)
		return command.Result{ExitCode: 0}
	}
	if len(args) < 2 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	format := args[1]
	rest := args[2:]

	// Bash cycles the format over the args; if format has no
	// conversions it's emitted once and extras are ignored. We also
	// guard against a pass that consumes zero args (e.g. format =
	// "%%") so we never spin in an infinite loop.
	hasConversion := strings.Contains(format, "%")
	var out strings.Builder
	exit := 0
	for {
		consumed, code, err := formatOnce(&out, format, rest)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "printf", 1, "%s", err.Error())
		}
		if code != 0 {
			exit = code
		}
		rest = rest[consumed:]
		if len(rest) == 0 || !hasConversion || consumed == 0 {
			break
		}
	}
	if c.Stdout != nil {
		if _, err := io.WriteString(c.Stdout, out.String()); err != nil {
			return builtinutil.Errorf(c.Stderr, "printf", 1, "write: %v", err)
		}
	}
	return command.Result{ExitCode: exit}
}

// formatOnce consumes one pass through format, writing to w. Returns
// the number of args consumed and a soft exit code (non-zero if a
// numeric conversion failed to parse — bash uses 1 there but still
// emits the partial output).
func formatOnce(w *strings.Builder, format string, args []string) (int, int, error) {
	consumed := 0
	exit := 0
	i := 0
	for i < len(format) {
		ch := format[i]
		if ch == '\\' && i+1 < len(format) {
			s, advance, stop := decodeBackslash(format[i:])
			w.WriteString(s)
			i += advance
			if stop {
				return consumed, exit, nil
			}
			continue
		}
		if ch != '%' {
			w.WriteByte(ch)
			i++
			continue
		}
		// Parse a conversion: %[flags][width][.precision][specifier]
		// or %(strftime)T.
		if i+1 < len(format) && format[i+1] == '%' {
			w.WriteByte('%')
			i += 2
			continue
		}
		spec, body, advance := parseSpec(format[i:])
		if advance == 0 {
			// Malformed — emit literally.
			w.WriteByte('%')
			i++
			continue
		}
		i += advance
		var arg string
		hasArg := false
		needsArg := spec.kind != 0 && spec.kind != 'T' || spec.kind == 'T'
		if needsArg {
			if consumed < len(args) {
				arg = args[consumed]
				consumed++
				hasArg = true
			} else {
				arg = ""
			}
		}
		out, code, err := applySpec(spec, body, arg, hasArg)
		if err != nil {
			return consumed, 1, err
		}
		if code != 0 {
			exit = code
		}
		w.WriteString(out)
	}
	return consumed, exit, nil
}

type specParse struct {
	flags     string
	width     string
	precision string
	kind      byte // 0 if none
	strftime  string
}

// parseSpec parses a `%...` conversion starting at format[0]. Returns
// (parsed, raw body, total chars consumed). advance == 0 signals
// "could not parse".
func parseSpec(format string) (specParse, string, int) {
	var sp specParse
	if format[0] != '%' {
		return sp, "", 0
	}
	i := 1
	// %(...)T strftime form.
	if i < len(format) && format[i] == '(' {
		j := i + 1
		for j < len(format) && format[j] != ')' {
			j++
		}
		if j >= len(format) || j+1 >= len(format) || format[j+1] != 'T' {
			return sp, "", 0
		}
		sp.strftime = format[i+1 : j]
		sp.kind = 'T'
		return sp, format[:j+2], j + 2
	}
	// Flags.
	for i < len(format) && strings.ContainsRune("-+ 0#'", rune(format[i])) {
		sp.flags += string(format[i])
		i++
	}
	// Width.
	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		sp.width += string(format[i])
		i++
	}
	// Precision.
	if i < len(format) && format[i] == '.' {
		sp.precision = "."
		i++
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			sp.precision += string(format[i])
			i++
		}
	}
	if i >= len(format) {
		return sp, "", 0
	}
	sp.kind = format[i]
	body := format[:i+1]
	return sp, body, i + 1
}

func applySpec(sp specParse, body, arg string, hasArg bool) (string, int, error) {
	switch sp.kind {
	case 'd', 'i', 'o', 'u', 'x', 'X':
		n, code := parseInt(arg, hasArg)
		// Go's fmt doesn't accept %u; remap to %d.
		spec := body
		if sp.kind == 'u' || sp.kind == 'i' {
			spec = body[:len(body)-1] + "d"
		}
		return fmt.Sprintf(spec, n), code, nil
	case 'c':
		if arg == "" {
			return "", 0, nil
		}
		return string(arg[0]), 0, nil
	case 's':
		return fmt.Sprintf(body, arg), 0, nil
	case 'f', 'e', 'E', 'g', 'G':
		f, code := parseFloat(arg, hasArg)
		return fmt.Sprintf(body, f), code, nil
	case 'b':
		out, _ := interpretBashEscapes(arg)
		return out, 0, nil
	case 'q':
		return shellQuote(arg), 0, nil
	case 'T':
		ts := parseStrftimeArg(arg)
		return formatStrftime(sp.strftime, ts), 0, nil
	default:
		return "", 1, fmt.Errorf("%%%c: invalid directive", sp.kind)
	}
}

func parseInt(s string, hasArg bool) (int64, int) {
	if !hasArg || s == "" {
		return 0, 0
	}
	if len(s) >= 2 && s[0] == '\'' {
		// 'c → ASCII value of the next character (bash extension).
		return int64(s[1]), 0
	}
	if len(s) >= 1 && s[0] == '"' {
		return int64(s[1]), 0
	}
	n, err := strconv.ParseInt(s, 0, 64) // 0 base honors 0x / 0 prefixes
	if err != nil {
		return 0, 1
	}
	return n, 0
}

func parseFloat(s string, hasArg bool) (float64, int) {
	if !hasArg || s == "" {
		return 0, 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, 1
	}
	return f, 0
}

// shellQuote returns s quoted in a form the shell can re-read. Uses
// single-quotes when possible; falls back to $'...' for control
// characters or when s contains a single-quote.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if safeForBare(s) {
		return s
	}
	needsAnsiC := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7f {
			needsAnsiC = true
			break
		}
	}
	if needsAnsiC {
		var b strings.Builder
		b.WriteString("$'")
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch c {
			case '\\':
				b.WriteString(`\\`)
			case '\'':
				b.WriteString(`\'`)
			case '\n':
				b.WriteString(`\n`)
			case '\t':
				b.WriteString(`\t`)
			case '\r':
				b.WriteString(`\r`)
			default:
				if c < 0x20 || c == 0x7f {
					fmt.Fprintf(&b, `\%03o`, c)
				} else {
					b.WriteByte(c)
				}
			}
		}
		b.WriteByte('\'')
		return b.String()
	}
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	// Replace ' with '\''.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func safeForBare(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		safe := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' ||
			c == '/' || c == ':' || c == '@' || c == '%' || c == '+' || c == ','
		if !safe {
			return false
		}
	}
	return true
}

func parseStrftimeArg(arg string) time.Time {
	if arg == "" || arg == "-1" {
		return time.Now()
	}
	if arg == "-2" {
		// "Shell start" — we don't track that; fall back to now.
		return time.Now()
	}
	n, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return time.Now()
	}
	return time.Unix(n, 0)
}

// formatStrftime renders t under a strftime format string. Supports
// the subset bash exercises in practice; falls through the unknown
// conversions literally.
func formatStrftime(format string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			b.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			fmt.Fprintf(&b, "%04d", t.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 'F':
			fmt.Fprintf(&b, "%04d-%02d-%02d", t.Year(), int(t.Month()), t.Day())
		case 'T':
			fmt.Fprintf(&b, "%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
		case 'D':
			fmt.Fprintf(&b, "%02d/%02d/%02d", int(t.Month()), t.Day(), t.Year()%100)
		case 's':
			fmt.Fprintf(&b, "%d", t.Unix())
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}

// decodeBackslash returns the literal expansion of a `\?` sequence at
// the start of s plus the number of source bytes consumed. The third
// return value is true for `\c` which stops output entirely.
func decodeBackslash(s string) (string, int, bool) {
	if len(s) < 2 || s[0] != '\\' {
		return s[:1], 1, false
	}
	switch s[1] {
	case 'a':
		return "\a", 2, false
	case 'b':
		return "\b", 2, false
	case 'c':
		return "", 2, true
	case 'e', 'E':
		return "\x1b", 2, false
	case 'f':
		return "\f", 2, false
	case 'n':
		return "\n", 2, false
	case 'r':
		return "\r", 2, false
	case 't':
		return "\t", 2, false
	case 'v':
		return "\v", 2, false
	case '\\':
		return "\\", 2, false
	case '"':
		return "\"", 2, false
	default:
		return s[:2], 2, false
	}
}

// interpretBashEscapes is the %b helper. Mirrors echo -e semantics.
func interpretBashEscapes(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'c':
			return b.String(), true
		case 'e', 'E':
			b.WriteByte(0x1b)
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\\':
			b.WriteByte('\\')
		case '0':
			n := 0
			j := i + 1
			for ; j < len(s) && j < i+4 && isOctal(s[j]); j++ {
				n = n*8 + int(s[j]-'0')
			}
			b.WriteByte(byte(n))
			i = j - 1
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String(), false
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }

func init() {
	command.RegisterBuiltin(New())
}
