// Package echo implements the `echo` built-in.
//
// Flags:
//   - -n  do not append a trailing newline
//   - -e  enable backslash escape interpretation
//   - -E  disable backslash escape interpretation (default)
//
// Notes:
//   - mvdan/sh handles `echo` as its own internal builtin BEFORE our
//     ExecHandler middleware fires, so a script's bare `echo foo`
//     never reaches this code path. This built-in serves the
//     `/bin/echo`, `command echo`, and `enable -n echo` invocation
//     forms — and provides parity for direct dispatch in tests.
//   - The xpg_echo shopt is not yet wired (Phase 11 will add the
//     shopt builtin); when it lands, this implementation will check
//     it via c.Env or a future c.Shopt accessor.
package echo

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const helpText = `Usage: echo [-neE] [arg ...]
Write arguments to standard output, separated by spaces and followed
by a trailing newline.

  -n  do not append a trailing newline
  -e  enable interpretation of backslash escapes
  -E  disable interpretation of backslash escapes (default)`

// New returns the echo command.
func New() command.Command {
	return command.Define("echo", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	suppressNewline := false
	interpretEscapes := false
	// xpg_echo: when set, -e is the default. We don't yet have shopt
	// state, but honor an opt-in via env so a host can set it.
	if v, ok := c.Env["XPG_ECHO"]; ok && v != "" && v != "0" {
		interpretEscapes = true
	}

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		// Only `-n`, `-e`, `-E`, and combinations thereof (e.g. -nE)
		// are recognized. Anything else is treated as the first
		// non-flag argument, matching bash.
		opts := a[1:]
		valid := true
		for _, ch := range opts {
			if ch != 'n' && ch != 'e' && ch != 'E' {
				valid = false
				break
			}
		}
		if !valid {
			break
		}
		for _, ch := range opts {
			switch ch {
			case 'n':
				suppressNewline = true
			case 'e':
				interpretEscapes = true
			case 'E':
				interpretEscapes = false
			}
		}
	}

	parts := args[i:]
	out := strings.Join(parts, " ")
	if interpretEscapes {
		var stop bool
		out, stop = interpretBashEscapes(out)
		if stop {
			suppressNewline = true
		}
	}
	if c.Stdout != nil {
		if _, err := io.WriteString(c.Stdout, out); err != nil {
			return builtinutil.Errorf(c.Stderr, "echo", 1, "write: %v", err)
		}
		if !suppressNewline {
			_, _ = fmt.Fprintln(c.Stdout)
		}
	}
	return command.Result{ExitCode: 0}
}

// interpretBashEscapes processes the backslash-escape sequences bash
// honors under `-e` (and xpg_echo). Returns (translated, stop) where
// stop is true if a `\c` sequence was encountered (which suppresses
// further output AND the trailing newline).
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
			// \0nnn (1–3 octal digits)
			n := 0
			j := i + 1
			for ; j < len(s) && j < i+4 && isOctal(s[j]); j++ {
				n = n*8 + int(s[j]-'0')
			}
			b.WriteByte(byte(n))
			i = j - 1
		case 'x':
			// \xHH (1–2 hex digits)
			n := 0
			j := i + 1
			cnt := 0
			for ; j < len(s) && cnt < 2 && isHex(s[j]); j++ {
				n = n*16 + hexVal(s[j])
				cnt++
			}
			if cnt == 0 {
				// no hex digits: emit literally
				b.WriteByte('\\')
				b.WriteByte('x')
			} else {
				b.WriteByte(byte(n))
				i = j - 1
			}
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String(), false
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }
func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

func init() {
	command.RegisterBuiltin(New())
}
