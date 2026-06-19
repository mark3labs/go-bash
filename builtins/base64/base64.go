// Package base64 implements the `base64` built-in (SPEC §10 Wave C).
//
// Flags: -d decode, -w N line-wrap at N columns (0 disables).
package base64

import (
	"bytes"
	"context"
	stdb64 "encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "base64 [-d] [-w N] [FILE]"
const helpText = `Usage: base64 [OPTION]... [FILE]
Base64 encode or decode FILE, or standard input, to standard output.

  -d, --decode          decode data
  -w, --wrap=COLS       wrap encoded lines after COLS character (default 76)
                        Use 0 to disable line wrapping`

// New returns the base64 command.
func New() command.Command { return command.Define("base64", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	decode := false
	wrap := 76
	var pos []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-d", a == "--decode":
			decode = true
		case a == "-i", a == "--ignore-garbage":
			// no-op (we always ignore garbage on decode)
		case a == "-w", a == "--wrap":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			w, err := strconv.Atoi(args[i])
			if err != nil || w < 0 {
				return builtinutil.Errorf(c.Stderr, "base64", 1, "invalid -w")
			}
			wrap = w
		case strings.HasPrefix(a, "--wrap="):
			w, err := strconv.Atoi(strings.TrimPrefix(a, "--wrap="))
			if err != nil || w < 0 {
				return builtinutil.Errorf(c.Stderr, "base64", 1, "invalid -w")
			}
			wrap = w
		case strings.HasPrefix(a, "-w") && len(a) > 2:
			w, err := strconv.Atoi(a[2:])
			if err != nil || w < 0 {
				return builtinutil.Errorf(c.Stderr, "base64", 1, "invalid -w")
			}
			wrap = w
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
	name := "-"
	if len(pos) >= 1 {
		name = pos[0]
	}
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "base64", 1, "%v", err)
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "base64", 1, "read: %v", err)
	}
	if decode {
		// strip whitespace
		clean := bytes.Map(func(r rune) rune {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				return -1
			}
			return r
		}, data)
		out, err := stdb64.StdEncoding.DecodeString(string(clean))
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "base64", 1, "invalid input")
		}
		_, _ = c.Stdout.Write(out)
		return command.Result{}
	}
	enc := stdb64.StdEncoding.EncodeToString(data)
	if wrap == 0 {
		_, _ = io.WriteString(c.Stdout, enc)
		_, _ = io.WriteString(c.Stdout, "\n")
		return command.Result{}
	}
	for i := 0; i < len(enc); i += wrap {
		end := i + wrap
		if end > len(enc) {
			end = len(enc)
		}
		_, _ = fmt.Fprintln(c.Stdout, enc[i:end])
	}
	return command.Result{}
}

func init() { command.RegisterBuiltin(New()) }
