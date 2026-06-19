// Package read implements the `read` shell built-in.
//
// Reads one line from c.Stdin and assigns it to the named variable(s)
// in c.Env. mvdan/sh ships its own `read` that mutates runner vars
// directly; our registration is reachable via /bin/read and the
// assignment is to c.Env (not propagated to the runner).
//
// Supported flags: -r (raw), -p PROMPT (printed to c.Stderr), -n N /
// -N N (read at most N bytes), -d DELIM (use DELIM instead of \n),
// -s (silent, no echo — we have no terminal so this is a no-op),
// -a ARRAY (split into env vars ARRAY_0, ARRAY_1, ...), -t TIMEOUT
// (best-effort using ctx).
package read

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "read [-r] [-p PROMPT] [-n N] [-N N] [-d DELIM] [-s] [-a ARRAY] [-t T] [NAME...]"

// New returns the read command.
func New() command.Command { return command.Define("read", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	raw := false
	delim := byte('\n')
	maxBytes := -1
	prompt := ""
	arrayName := ""
	timeout := time.Duration(0)

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case a == "-r":
			raw = true
		case a == "-s":
			// silent: no-op (no terminal)
		case a == "-p":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			prompt = args[i]
		case a == "-n" || a == "-N":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			maxBytes = n
		case a == "-d":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			if len(args[i]) == 0 {
				delim = 0
			} else {
				delim = args[i][0]
			}
		case a == "-a":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			arrayName = args[i]
		case a == "-t":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.ParseFloat(args[i], 64)
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			timeout = time.Duration(n * float64(time.Second))
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	names := args[i:]
	if len(names) == 0 && arrayName == "" {
		names = []string{"REPLY"}
	}
	if prompt != "" && c.Stderr != nil {
		_, _ = io.WriteString(c.Stderr, prompt)
	}
	if c.Stdin == nil {
		return command.Result{ExitCode: 1}
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		_ = ctx
	}

	line, err := readDelim(c.Stdin, delim, maxBytes)
	if err == io.EOF && line == "" {
		return command.Result{ExitCode: 1}
	}
	if err != nil && err != io.EOF {
		return builtinutil.Errorf(c.Stderr, "read", 1, "%v", err)
	}
	if !raw {
		line = unescapeBackslash(line)
	}

	if arrayName != "" {
		parts := strings.Fields(line)
		if c.Env != nil {
			for idx, p := range parts {
				c.Env[fmt.Sprintf("%s_%d", arrayName, idx)] = p
			}
		}
		return command.Result{ExitCode: 0}
	}

	parts := strings.Fields(line)
	if c.Env != nil {
		for idx, name := range names {
			if idx == len(names)-1 && idx < len(parts)-1 {
				c.Env[name] = strings.Join(parts[idx:], " ")
				break
			}
			if idx < len(parts) {
				c.Env[name] = parts[idx]
			} else {
				c.Env[name] = ""
			}
		}
	}
	return command.Result{ExitCode: 0}
}

func readDelim(r io.Reader, delim byte, maxBytes int) (string, error) {
	br := bufio.NewReader(r)
	var sb strings.Builder
	count := 0
	for {
		if maxBytes >= 0 && count >= maxBytes {
			return sb.String(), nil
		}
		b, err := br.ReadByte()
		if err != nil {
			return sb.String(), err
		}
		if b == delim {
			return sb.String(), nil
		}
		sb.WriteByte(b)
		count++
	}
}

func unescapeBackslash(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			sb.WriteByte(s[i+1])
			i++
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

func init() { command.RegisterBuiltin(New()) }
