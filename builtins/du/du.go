// Package du implements the `du` built-in (SPEC §10 Wave B).
//
// Flags: -s (summary), -h (human), -a (all files), -c (grand total),
// -d N / --max-depth=N, -b/-k/-m (unit), -x (no-op single FS).
package du

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "du [-sh] [-a] [-c] [-d DEPTH] [-b|-k|-m] PATH..."
const helpText = `Usage: du [OPTION]... [FILE]...
Summarize disk usage of each FILE, recursively for directories.

  -s, --summarize  display only a total for each argument
  -h, --human-readable  print sizes in human readable format
  -a, --all        write counts for all files, not just directories
  -c, --total      produce a grand total
  -d, --max-depth=N  print total for a directory only if it is N or fewer levels below
  -b, --bytes      equivalent to '--apparent-size --block-size=1'
  -k               like --block-size=1K
  -m               like --block-size=1M
  -x               skip directories on different file systems (no-op)`

type opts struct {
	summarize bool
	human     bool
	all       bool
	total     bool
	maxDepth  int
	unit      int64 // bytes per block
}

// New returns the du command.
func New() command.Command { return command.Define("du", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	o := opts{maxDepth: -1, unit: 1024}
	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-s", a == "--summarize":
			o.summarize = true
		case a == "-h", a == "--human-readable":
			o.human = true
		case a == "-a", a == "--all":
			o.all = true
		case a == "-c", a == "--total":
			o.total = true
		case a == "-x":
			// no-op
		case a == "-b", a == "--bytes":
			o.unit = 1
		case a == "-k":
			o.unit = 1024
		case a == "-m":
			o.unit = 1024 * 1024
		case a == "-d", a == "--max-depth":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.maxDepth = n
		case strings.HasPrefix(a, "--max-depth="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--max-depth="))
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.maxDepth = n
		case a == "--":
			i++
			paths = append(paths, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			paths = append(paths, a)
		}
	}
run:
	if len(paths) == 0 {
		paths = []string{"."}
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "du", 1, "no filesystem")
	}
	var grand int64
	for _, p := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, p)
		total, err := walk(c, p, abs, 0, &o, c.Stdout)
		if err != nil {
			_ = builtinutil.Errorf(c.Stderr, "du", 1, "%s: %v", p, err)
			continue
		}
		grand += total
	}
	if o.total && c.Stdout != nil {
		writeRow(c.Stdout, grand, "total", &o)
	}
	return command.Result{}
}

func walk(c *command.Context, displayPath, abs string, depth int, o *opts, w io.Writer) (int64, error) {
	fi, err := c.FS.Lstat(abs)
	if err != nil {
		return 0, err
	}
	if !fi.IsDir() {
		size := fi.Size()
		if (o.all || depth == 0) && !o.summarize && shouldPrint(depth, o) {
			writeRow(w, size, displayPath, o)
		}
		return size, nil
	}
	var total int64
	entries, err := c.FS.ReadDir(abs)
	if err != nil {
		return 0, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		sub := path.Join(abs, e.Name())
		subDisplay := path.Join(displayPath, e.Name())
		s, err := walk(c, subDisplay, sub, depth+1, o, w)
		if err != nil {
			continue
		}
		total += s
	}
	// Directories themselves count as some non-zero size in real du
	// (the inode blocks); we treat dirs as 0 bytes themselves so the
	// total equals sum of contents. coreutils includes a 4096-byte
	// directory entry; for parity with TS we use 0 (just-bash matches
	// content sum).
	if o.summarize {
		if depth == 0 {
			writeRow(w, total, displayPath, o)
		}
	} else if shouldPrint(depth, o) {
		writeRow(w, total, displayPath, o)
	}
	return total, nil
}

func shouldPrint(depth int, o *opts) bool {
	if o.summarize {
		return depth == 0
	}
	if o.maxDepth < 0 {
		return true
	}
	return depth <= o.maxDepth
}

func writeRow(w io.Writer, size int64, name string, o *opts) {
	if w == nil {
		return
	}
	if o.human {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", builtinutil.HumanSize(size), name)
		return
	}
	blocks := size / o.unit
	if size%o.unit != 0 {
		blocks++
	}
	_, _ = fmt.Fprintf(w, "%d\t%s\n", blocks, name)
}

func init() { command.RegisterBuiltin(New()) }
