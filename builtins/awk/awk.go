// Package awk implements the `awk` built-in.
//
// Wraps github.com/benhoyt/goawk/interp. The runtime is sandboxed:
// we disable goawk's file-read / file-write / exec capabilities and
// route all input through c.FS / c.Stdin. Honors
// Context.Limits.MaxAwkIterations as a derived ctx deadline (goawk
// 1.31 exposes ExecuteContext but no per-iteration progress hook —
// upstream issue candidate).
package awk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	stdstrings "strings"
	"time"

	awkinterp "github.com/benhoyt/goawk/interp"
	awkparser "github.com/benhoyt/goawk/parser"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "awk [-F SEP] [-v VAR=VAL] [-f FILE | PROGRAM] [FILE...]"
const helpText = `Usage: awk [OPTION]... [PROGRAM] [FILE]...
Execute the AWK PROGRAM on each FILE (or stdin).

  -F SEP            field separator
  -v VAR=VAL        set AWK variable
  -f FILE           read program from FILE (may repeat)
  --                end of options`

// New returns the awk command.
func New() command.Command { return command.Define("awk", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	fieldSep := ""
	var vars []string
	var progFiles []string
	var program string
	progSet := false
	var files []string

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-F":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			fieldSep = args[i]
		case stdstrings.HasPrefix(a, "-F") && len(a) > 2:
			fieldSep = a[2:]
		case a == "-v":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			vars = appendVar(vars, args[i])
		case stdstrings.HasPrefix(a, "-v") && len(a) > 2:
			vars = appendVar(vars, a[2:])
		case a == "-f":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			progFiles = append(progFiles, args[i])
		case a == "--":
			i++
			if !progSet && len(progFiles) == 0 {
				if i < len(args) {
					program = args[i]
					progSet = true
					i++
				}
			}
			files = append(files, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			if !progSet && len(progFiles) == 0 {
				program = a
				progSet = true
			} else {
				files = append(files, a)
			}
		}
	}
run:
	if !progSet && len(progFiles) == 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}

	// Concatenate program from -f files (and inline program).
	if len(progFiles) > 0 {
		var src bytes.Buffer
		for _, pf := range progFiles {
			data, err := readVFS(c, pf)
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "awk", 2, "%s: %v", pf, err)
			}
			src.Write(data)
			src.WriteByte('\n')
		}
		if progSet {
			src.WriteString(program)
			src.WriteByte('\n')
		}
		program = src.String()
	}

	prog, err := awkparser.ParseProgram([]byte(program), nil)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "awk", 2, "%v", err)
	}
	interp, err := awkinterp.New(prog)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "awk", 2, "%v", err)
	}

	// Build stdin by concatenating file inputs (or use c.Stdin).
	var inputReader = c.Stdin
	if len(files) > 0 {
		var buf bytes.Buffer
		for _, f := range files {
			data, err := readVFS(c, f)
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "awk", 2, "%s: %v", f, err)
			}
			buf.Write(data)
		}
		inputReader = &buf
	}
	if inputReader == nil {
		inputReader = stdstrings.NewReader("")
	}

	cfg := &awkinterp.Config{
		Stdin:        inputReader,
		Output:       c.Stdout,
		Error:        c.Stderr,
		Argv0:        "awk",
		Vars:         vars,
		NoExec:       true,
		NoFileWrites: true,
		NoFileReads:  true,
	}
	if fieldSep != "" {
		cfg.Vars = append(cfg.Vars, "FS", fieldSep)
	}

	// Apply MaxAwkIterations as a time-budget on the derived ctx.
	// goawk 1.31 has no per-iteration callback (upstream issue
	// candidate — see handoff). We approximate by giving the
	// interpreter a generous wall-clock window proportional to the
	// iteration cap (1µs per iteration, capped at 30s).
	subCtx := ctx
	if c.Limits.MaxAwkIterations > 0 {
		budget := time.Duration(c.Limits.MaxAwkIterations) * time.Microsecond
		if budget > 30*time.Second {
			budget = 30 * time.Second
		}
		if budget < 10*time.Millisecond {
			budget = 10 * time.Millisecond
		}
		var cancel context.CancelFunc
		subCtx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}

	code, err := interp.ExecuteContext(subCtx, cfg)
	if err != nil {
		_, _ = fmt.Fprintf(c.Stderr, "awk: %v\n", err)
		if code == 0 {
			code = 2
		}
	}
	return command.Result{ExitCode: code}
}

func appendVar(vars []string, kv string) []string {
	idx := stdstrings.IndexByte(kv, '=')
	if idx < 0 {
		return append(vars, kv, "")
	}
	return append(vars, kv[:idx], kv[idx+1:])
}

func readVFS(c *command.Context, name string) ([]byte, error) {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	return io.ReadAll(r)
}

func init() { command.RegisterBuiltin(New()) }
