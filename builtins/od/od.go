// Package od implements the `od` built-in.
//
// Flags: -A {d|o|x|n} address radix, -t TYPE output type,
// -v don't suppress duplicates, -w COLS bytes per row.
package od

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "od [-A radix] [-t TYPE] [-v] [-w COLS] [FILE...]"
const helpText = `Usage: od [OPTION]... [FILE]...
Write an unambiguous representation, octal bytes by default, of FILE to stdout.

  -A, --address-radix=RADIX   d, o, x, n
  -t, --format=TYPE           one of x1, x2, o1, d1, c, a
  -v, --output-duplicates     do not use * to mark line suppression
  -w, --width=BYTES           output BYTES bytes per output line (default 16)`

// New returns the od command.
func New() command.Command { return command.Define("od", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	addr := "o"
	typ := "o2"
	showAll := false
	width := 16
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-A", a == "--address-radix":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			addr = args[i]
		case strings.HasPrefix(a, "--address-radix="):
			addr = strings.TrimPrefix(a, "--address-radix=")
		case strings.HasPrefix(a, "-A") && len(a) > 2:
			addr = a[2:]
		case a == "-t", a == "--format":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			typ = args[i]
		case strings.HasPrefix(a, "--format="):
			typ = strings.TrimPrefix(a, "--format=")
		case strings.HasPrefix(a, "-t") && len(a) > 2:
			typ = a[2:]
		case a == "-v", a == "--output-duplicates":
			showAll = true
		case a == "-w", a == "--width":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "od", 1, "invalid -w")
			}
			width = n
		case strings.HasPrefix(a, "--width="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--width="))
			if err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "od", 1, "invalid -w")
			}
			width = n
		case strings.HasPrefix(a, "-w") && len(a) > 2:
			n, err := strconv.Atoi(a[2:])
			if err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "od", 1, "invalid -w")
			}
			width = n
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			files = append(files, a)
		}
	}
run:
	data, err := builtinutil.ReadAllInputs(c, files)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "od", 1, "%v", err)
	}
	dump(c.Stdout, data, addr, typ, width, showAll)
	return command.Result{}
}

func dump(w io.Writer, data []byte, addr, typ string, width int, showAll bool) {
	addrFmt := func(off int) string {
		switch addr {
		case "d":
			return fmt.Sprintf("%07d", off)
		case "x":
			return fmt.Sprintf("%07x", off)
		case "n":
			return strings.Repeat(" ", 7)
		default:
			return fmt.Sprintf("%07o", off)
		}
	}
	off := 0
	var prev []byte
	suppressed := false
	for off < len(data) {
		end := off + width
		if end > len(data) {
			end = len(data)
		}
		row := data[off:end]
		if !showAll && len(prev) > 0 && bytesEqual(prev, row) && len(row) == width {
			if !suppressed {
				_, _ = io.WriteString(w, "*\n")
				suppressed = true
			}
			off = end
			continue
		}
		suppressed = false
		_, _ = io.WriteString(w, addrFmt(off))
		_, _ = io.WriteString(w, " ")
		_, _ = io.WriteString(w, formatRow(row, typ))
		_, _ = io.WriteString(w, "\n")
		prev = row
		off = end
	}
	_, _ = fmt.Fprintf(w, "%s\n", addrFmt(len(data)))
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func formatRow(b []byte, typ string) string {
	var parts []string
	switch typ {
	case "x1":
		for _, c := range b {
			parts = append(parts, fmt.Sprintf("%02x", c))
		}
	case "x2":
		for i := 0; i+1 < len(b); i += 2 {
			v := uint16(b[i]) | uint16(b[i+1])<<8
			parts = append(parts, fmt.Sprintf("%04x", v))
		}
		if len(b)%2 == 1 {
			parts = append(parts, fmt.Sprintf("  %02x", b[len(b)-1]))
		}
	case "d1":
		for _, c := range b {
			parts = append(parts, fmt.Sprintf("%4d", c))
		}
	case "o1":
		for _, c := range b {
			parts = append(parts, fmt.Sprintf("%03o", c))
		}
	case "c":
		for _, c := range b {
			parts = append(parts, charRepr(c))
		}
	case "a":
		for _, c := range b {
			parts = append(parts, asciiName(c))
		}
	default: // "o2" or "o"
		for i := 0; i+1 < len(b); i += 2 {
			v := uint16(b[i]) | uint16(b[i+1])<<8
			parts = append(parts, fmt.Sprintf("%06o", v))
		}
		if len(b)%2 == 1 {
			parts = append(parts, fmt.Sprintf("    %03o", b[len(b)-1]))
		}
	}
	return strings.Join(parts, " ")
}

func charRepr(b byte) string {
	switch b {
	case 0:
		return "\\0"
	case '\a':
		return "\\a"
	case '\b':
		return "\\b"
	case '\t':
		return "\\t"
	case '\n':
		return "\\n"
	case '\v':
		return "\\v"
	case '\f':
		return "\\f"
	case '\r':
		return "\\r"
	}
	if b >= 0x20 && b < 0x7f {
		return fmt.Sprintf(" %c", b)
	}
	return fmt.Sprintf("%03o", b)
}

func asciiName(b byte) string {
	names := []string{"nul", "soh", "stx", "etx", "eot", "enq", "ack", "bel",
		"bs", "ht", "nl", "vt", "ff", "cr", "so", "si",
		"dle", "dc1", "dc2", "dc3", "dc4", "nak", "syn", "etb",
		"can", "em", "sub", "esc", "fs", "gs", "rs", "us"}
	switch {
	case b < 32:
		return names[b]
	case b == 127:
		return "del"
	}
	return string(b)
}

func init() { command.RegisterBuiltin(New()) }
