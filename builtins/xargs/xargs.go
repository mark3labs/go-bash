// Package xargs implements the `xargs` built-in (SPEC §10 Wave C).
//
// Flags: -0 NUL-delimited, -n MAX-ARGS, -I REPL string replace,
// -P N parallel (goroutine-bounded), --max-args=N, -d DELIM, -a FILE.
//
// In the sandbox xargs invokes the command via c.Registry.Lookup
// (no real processes). Parallelism is goroutine-bounded by -P N.
package xargs

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "xargs [-0n PIRP] [-I REPL] [-P N] [-d DELIM] [-a FILE] [CMD [ARGS...]]"
const helpText = `Usage: xargs [OPTION]... [COMMAND [INITIAL-ARGS...]]
Run COMMAND with arguments built from input.

  -0, --null            input items are NUL-terminated, not whitespace
  -n, --max-args=N      use at most N arguments per command line
  -I REPL               replace occurrences of REPL with input
  -P, --max-procs=N     run up to N processes (goroutines) at a time
  -d, --delimiter=DELIM use DELIM as item delimiter
  -a FILE               read items from FILE instead of standard input`

type opts struct {
	nul       bool
	maxArgs   int
	replace   string
	parallel  int
	delim     byte
	useDelim  bool
	fromFile  string
}

// New returns the xargs command.
func New() command.Command { return command.Define("xargs", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	var o opts
	o.parallel = 1
	var cmdArgs []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-0", a == "--null":
			o.nul = true
		case a == "-n", a == "--max-args":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.maxArgs, _ = strconv.Atoi(args[i])
		case strings.HasPrefix(a, "--max-args="):
			o.maxArgs, _ = strconv.Atoi(strings.TrimPrefix(a, "--max-args="))
		case a == "-I":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.replace = args[i]
		case a == "-P", a == "--max-procs":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, _ := strconv.Atoi(args[i])
			if n < 1 {
				n = 1
			}
			o.parallel = n
		case a == "-d", a == "--delimiter":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			if len(args[i]) > 0 {
				o.delim = args[i][0]
			}
			o.useDelim = true
		case strings.HasPrefix(a, "--delimiter="):
			s := strings.TrimPrefix(a, "--delimiter=")
			if len(s) > 0 {
				o.delim = s[0]
			}
			o.useDelim = true
		case a == "-a":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.fromFile = args[i]
		case a == "--":
			i++
			cmdArgs = append(cmdArgs, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			cmdArgs = append(cmdArgs, args[i:]...)
			goto run
		}
	}
run:
	if len(cmdArgs) == 0 {
		cmdArgs = []string{"echo"}
	}
	if c.Registry == nil {
		return builtinutil.Errorf(c.Stderr, "xargs", 1, "no command registry")
	}
	cmd, ok := c.Registry.Lookup(cmdArgs[0])
	if !ok {
		return builtinutil.Errorf(c.Stderr, "xargs", 127, "%s: command not found", cmdArgs[0])
	}
	// Read items
	src := io.Reader(nil)
	if o.fromFile != "" {
		r, closer, err := builtinutil.OpenInput(c, o.fromFile)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "xargs", 1, "%v", err)
		}
		if closer != nil {
			defer func() { _ = closer.Close() }()
		}
		src = r
	} else if c.Stdin != nil {
		src = c.Stdin
	} else {
		src = strings.NewReader("")
	}
	items, err := readItems(src, &o)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xargs", 1, "read: %v", err)
	}
	// Build groups
	groups := buildGroups(cmdArgs, items, &o)
	// Run with goroutine bounded parallelism
	sem := make(chan struct{}, o.parallel)
	var wg sync.WaitGroup
	var mu sync.Mutex
	exit := 0
	// Serialize output writes when running in parallel.
	subCtx := *c
	if o.parallel > 1 {
		subCtx.Stdout = &lockedWriter{w: c.Stdout, mu: &mu}
		subCtx.Stderr = &lockedWriter{w: c.Stderr, mu: &mu}
	}
	for _, g := range groups {
		sem <- struct{}{}
		wg.Add(1)
		go func(g []string) {
			defer wg.Done()
			defer func() { <-sem }()
			res := cmd.Execute(ctx, g, &subCtx)
			if res.ExitCode != 0 {
				mu.Lock()
				if exit == 0 {
					exit = res.ExitCode
				}
				mu.Unlock()
			}
		}(g)
	}
	wg.Wait()
	return command.Result{ExitCode: exit}
}

// lockedWriter serializes writes from multiple goroutines.
type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if lw.w == nil {
		return len(p), nil
	}
	return lw.w.Write(p)
}

func readItems(r io.Reader, o *opts) ([]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	switch {
	case o.nul:
		return splitDelim(data, 0), nil
	case o.useDelim:
		return splitDelim(data, o.delim), nil
	}
	// whitespace split
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	sc.Split(bufio.ScanWords)
	var out []string
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, nil
}

func splitDelim(data []byte, delim byte) []string {
	var out []string
	start := 0
	for i, b := range data {
		if b == delim {
			out = append(out, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, string(data[start:]))
	}
	return out
}

func buildGroups(cmdArgs []string, items []string, o *opts) [][]string {
	if o.replace != "" {
		// One invocation per item, substituting REPL with the item.
		var groups [][]string
		for _, item := range items {
			g := make([]string, len(cmdArgs))
			for i, a := range cmdArgs {
				g[i] = strings.ReplaceAll(a, o.replace, item)
			}
			groups = append(groups, g)
		}
		return groups
	}
	if o.maxArgs > 0 {
		var groups [][]string
		for start := 0; start < len(items); start += o.maxArgs {
			end := start + o.maxArgs
			if end > len(items) {
				end = len(items)
			}
			g := make([]string, 0, len(cmdArgs)+(end-start))
			g = append(g, cmdArgs...)
			g = append(g, items[start:end]...)
			groups = append(groups, g)
		}
		if len(groups) == 0 {
			groups = append(groups, append([]string{}, cmdArgs...))
		}
		return groups
	}
	g := append([]string{}, cmdArgs...)
	g = append(g, items...)
	return [][]string{g}
}

func init() { command.RegisterBuiltin(New()) }
