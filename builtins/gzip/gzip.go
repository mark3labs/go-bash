// Package gzip implements the `gzip`, `gunzip`, and `zcat` built-ins
// (SPEC §10 Wave F). All three share a single implementation
// parameterized by the default mode (compress, decompress, decompress-to-stdout).
//
// Flags:
//
//	-d           decompress (gzip only; gunzip implies it)
//	-k           keep input file (do not unlink)
//	-c           write output to stdout instead of <name>.gz / <name>
//	-N           use original filename stored in the gzip header
//	-r           recurse into directories
//	-l           list contents (compressed size, uncompressed size, ratio, name)
//	-1..-9       compression level (compress only)
//	--help       show this help and exit 0
//
// All file I/O routes through Context.FS; stdin via Context.Stdin;
// stdout via Context.Stdout.
package gzip

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	iofs "io/fs"
	"path"
	"sort"
	stdstrings "strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

// Mode selects the default behavior for a gzip-family invocation.
type Mode int

const (
	// ModeCompress — `gzip`. Default: compress each named file in
	// place, writing <name>.gz. With -d it decompresses.
	ModeCompress Mode = iota
	// ModeDecompress — `gunzip`. Default: decompress each named .gz
	// file, writing the trimmed name.
	ModeDecompress
	// ModeZcat — `zcat`. Equivalent to `gunzip -c`: decompress to
	// stdout, never touch the original.
	ModeZcat
)

const gzipUsage = "gzip [-dkcNrl] [-1..-9] [FILE...]"
const gunzipUsage = "gunzip [-kcNr] [FILE...]"
const zcatUsage = "zcat [FILE...]"

const gzipHelp = `Usage: gzip [OPTION]... [FILE]...
Compress or decompress FILEs (by default, compress FILES in-place).

  -d, --decompress  decompress (same as gunzip)
  -k, --keep        keep (don't delete) input files
  -c, --stdout      write output to standard output
  -N, --name        save or restore the original name
  -r, --recursive   operate recursively on directories
  -l, --list        list compressed file contents
  -1..-9            compression level (default 6)
      --help        show this help and exit
`

const gunzipHelp = `Usage: gunzip [OPTION]... [FILE]...
Decompress FILEs in-place (or to stdout with -c).

  -k, --keep        keep (don't delete) input files
  -c, --stdout      write output to standard output
  -N, --name        restore the original name from the gzip header
  -r, --recursive   operate recursively on directories
  -l, --list        list compressed file contents
      --help        show this help and exit
`

const zcatHelp = `Usage: zcat [FILE]...
Decompress FILEs to standard output.

      --help        show this help and exit
`

type opts struct {
	decompress bool
	keep       bool
	stdout     bool
	useName    bool
	recursive  bool
	list       bool
	level      int
}

// New returns the gzip command.
func New() command.Command { return command.Define("gzip", runGzip) }

func runGzip(_ context.Context, args []string, c *command.Context) command.Result {
	return Run(c, args, ModeCompress)
}

// Run is the shared entrypoint for gzip / gunzip / zcat.
func Run(c *command.Context, args []string, mode Mode) command.Result {
	cmdName, usage, help := modeStrings(mode)
	o := opts{level: gzip.DefaultCompression}
	switch mode {
	case ModeDecompress:
		o.decompress = true
	case ModeZcat:
		o.decompress = true
		o.stdout = true
		o.keep = true
	}
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, help)
			return command.Result{ExitCode: 0}
		case a == "-d", a == "--decompress", a == "--uncompress":
			o.decompress = true
		case a == "-k", a == "--keep":
			o.keep = true
		case a == "-c", a == "--stdout", a == "--to-stdout":
			o.stdout = true
			o.keep = true
		case a == "-N", a == "--name":
			o.useName = true
		case a == "-n", a == "--no-name":
			o.useName = false
		case a == "-r", a == "--recursive":
			o.recursive = true
		case a == "-l", a == "--list":
			o.list = true
		case a == "-1", a == "--fast":
			o.level = gzip.BestSpeed
		case a == "-2":
			o.level = 2
		case a == "-3":
			o.level = 3
		case a == "-4":
			o.level = 4
		case a == "-5":
			o.level = 5
		case a == "-6":
			o.level = 6
		case a == "-7":
			o.level = 7
		case a == "-8":
			o.level = 8
		case a == "-9", a == "--best":
			o.level = gzip.BestCompression
		case a == "-q", a == "--quiet", a == "-v", a == "--verbose", a == "-f", a == "--force":
			// no-op: we don't have a TTY, never prompt, force is always implicit
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			if !parseBundle(a, &o) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			files = append(files, a)
		}
	}
run:
	if o.list {
		return runList(c, cmdName, files)
	}
	// stdin/stdout-only mode: no files OR a single "-".
	if len(files) == 0 {
		if o.decompress {
			return decompressStream(c, c.Stdin, c.Stdout, cmdName)
		}
		return compressStream(c, c.Stdin, c.Stdout, &o, "")
	}
	exit := 0
	for _, f := range files {
		if f == "-" {
			var rerr command.Result
			if o.decompress {
				rerr = decompressStream(c, c.Stdin, c.Stdout, cmdName)
			} else {
				rerr = compressStream(c, c.Stdin, c.Stdout, &o, "")
			}
			if rerr.ExitCode != 0 {
				exit = rerr.ExitCode
			}
			continue
		}
		if err := processPath(c, cmdName, f, &o); err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmdName, f, err)
		}
	}
	return command.Result{ExitCode: exit}
}

func modeStrings(m Mode) (cmdName, usage, help string) {
	switch m {
	case ModeDecompress:
		return "gunzip", gunzipUsage, gunzipHelp
	case ModeZcat:
		return "zcat", zcatUsage, zcatHelp
	default:
		return "gzip", gzipUsage, gzipHelp
	}
}

func parseBundle(a string, o *opts) bool {
	// Try to interpret `-dkc`/`-9k`/etc. as a bundle of short flags
	// (digit characters set the level).
	for _, ch := range a[1:] {
		switch ch {
		case 'd':
			o.decompress = true
		case 'k':
			o.keep = true
		case 'c':
			o.stdout = true
			o.keep = true
		case 'N':
			o.useName = true
		case 'n':
			o.useName = false
		case 'r':
			o.recursive = true
		case 'l':
			o.list = true
		case 'q', 'v', 'f':
			// no-op
		case '1':
			o.level = 1
		case '2':
			o.level = 2
		case '3':
			o.level = 3
		case '4':
			o.level = 4
		case '5':
			o.level = 5
		case '6':
			o.level = 6
		case '7':
			o.level = 7
		case '8':
			o.level = 8
		case '9':
			o.level = 9
		default:
			return false
		}
	}
	return true
}

// processPath dispatches a single named path (file or, with -r, directory).
func processPath(c *command.Context, cmdName, p string, o *opts) error {
	abs := builtinutil.ResolvePath(c.Cwd, p)
	fi, err := c.FS.Stat(abs)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		if !o.recursive {
			return fmt.Errorf("is a directory")
		}
		return walkDir(c, cmdName, abs, p, o)
	}
	return processFile(c, cmdName, abs, p, o)
}

func walkDir(c *command.Context, cmdName, abs, rel string, o *opts) error {
	entries, err := c.FS.ReadDir(abs)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		childAbs := path.Join(abs, e.Name())
		childRel := path.Join(rel, e.Name())
		fi, err := c.FS.Lstat(childAbs)
		if err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmdName, childRel, err)
			continue
		}
		if fi.Mode()&iofs.ModeSymlink != 0 {
			continue
		}
		if fi.IsDir() {
			if werr := walkDir(c, cmdName, childAbs, childRel, o); werr != nil {
				_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmdName, childRel, werr)
			}
			continue
		}
		// Skip files that don't have/lack the .gz suffix as appropriate.
		if o.decompress && !stdstrings.HasSuffix(e.Name(), ".gz") {
			continue
		}
		if !o.decompress && stdstrings.HasSuffix(e.Name(), ".gz") {
			continue
		}
		if err := processFile(c, cmdName, childAbs, childRel, o); err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmdName, childRel, err)
		}
	}
	return nil
}

func processFile(c *command.Context, cmdName, abs, rel string, o *opts) error {
	if o.decompress {
		return decompressFile(c, cmdName, abs, rel, o)
	}
	return compressFile(c, cmdName, abs, rel, o)
}

func compressFile(c *command.Context, cmdName, abs, rel string, o *opts) error {
	data, err := c.FS.ReadFile(abs)
	if err != nil {
		return err
	}
	dstName := rel
	if !o.stdout {
		if stdstrings.HasSuffix(rel, ".gz") {
			return fmt.Errorf("already has .gz suffix")
		}
		dstName = rel + ".gz"
	}
	var w io.Writer
	if o.stdout {
		w = c.Stdout
	} else {
		dstAbs := builtinutil.ResolvePath(c.Cwd, dstName)
		f, ferr := c.FS.Create(dstAbs)
		if ferr != nil {
			return ferr
		}
		defer f.Close() //nolint:errcheck
		w = f
	}
	gw, err := gzip.NewWriterLevel(w, o.level)
	if err != nil {
		return err
	}
	gw.Name = path.Base(rel)
	if fi, ferr := c.FS.Stat(abs); ferr == nil {
		gw.ModTime = fi.ModTime()
	}
	if _, err := gw.Write(data); err != nil {
		return err
	}
	if err := gw.Close(); err != nil {
		return err
	}
	if !o.stdout && !o.keep {
		_ = c.FS.Remove(abs)
	}
	_ = cmdName // reserved for future diagnostics
	return nil
}

func compressStream(c *command.Context, r io.Reader, w io.Writer, o *opts, name string) command.Result {
	if r == nil {
		r = stdstrings.NewReader("")
	}
	if w == nil {
		return builtinutil.Errorf(c.Stderr, "gzip", 1, "no stdout writer")
	}
	gw, err := gzip.NewWriterLevel(w, o.level)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "gzip", 1, "%v", err)
	}
	if name != "" {
		gw.Name = name
	}
	if _, err := io.Copy(gw, r); err != nil {
		return builtinutil.Errorf(c.Stderr, "gzip", 1, "%v", err)
	}
	if err := gw.Close(); err != nil {
		return builtinutil.Errorf(c.Stderr, "gzip", 1, "%v", err)
	}
	return command.Result{ExitCode: 0}
}

func decompressFile(c *command.Context, cmdName, abs, rel string, o *opts) error {
	f, err := c.FS.OpenFile(abs, 0, 0)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close() //nolint:errcheck

	if o.stdout {
		if _, cerr := io.Copy(c.Stdout, gr); cerr != nil {
			return cerr
		}
		return nil
	}

	// Determine output filename.
	dstName := rel
	if stdstrings.HasSuffix(dstName, ".gz") {
		dstName = stdstrings.TrimSuffix(dstName, ".gz")
	} else if stdstrings.HasSuffix(dstName, ".tgz") {
		dstName = stdstrings.TrimSuffix(dstName, ".tgz") + ".tar"
	} else {
		dstName += ".out"
	}
	if o.useName && gr.Name != "" {
		dstName = path.Join(path.Dir(rel), path.Base(gr.Name))
	}
	dstAbs := builtinutil.ResolvePath(c.Cwd, dstName)
	out, err := c.FS.Create(dstAbs)
	if err != nil {
		return err
	}
	if _, cerr := io.Copy(out, gr); cerr != nil {
		_ = out.Close()
		return cerr
	}
	if err := out.Close(); err != nil {
		return err
	}
	if !o.keep {
		_ = c.FS.Remove(abs)
	}
	_ = cmdName
	return nil
}

func decompressStream(c *command.Context, r io.Reader, w io.Writer, cmdName string) command.Result {
	if r == nil {
		r = stdstrings.NewReader("")
	}
	gr, err := gzip.NewReader(r)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "%v", err)
	}
	defer gr.Close() //nolint:errcheck
	if _, err := io.Copy(w, gr); err != nil {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "%v", err)
	}
	return command.Result{ExitCode: 0}
}

// runList implements -l: print a header + one row per file with the
// compressed size, uncompressed size, ratio, and name.
func runList(c *command.Context, cmdName string, files []string) command.Result {
	if len(files) == 0 {
		return builtinutil.Errorf(c.Stderr, cmdName, 1, "-l requires file operands")
	}
	_, _ = fmt.Fprintln(c.Stdout, "         compressed        uncompressed  ratio uncompressed_name")
	exit := 0
	for _, f := range files {
		abs := builtinutil.ResolvePath(c.Cwd, f)
		fi, err := c.FS.Stat(abs)
		if err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmdName, f, err)
			continue
		}
		data, err := c.FS.ReadFile(abs)
		if err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmdName, f, err)
			continue
		}
		gr, err := gzip.NewReader(stdstrings.NewReader(string(data)))
		if err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmdName, f, err)
			continue
		}
		decoded, err := io.ReadAll(gr)
		_ = gr.Close()
		if err != nil {
			exit = 1
			_, _ = fmt.Fprintf(c.Stderr, "%s: %s: %v\n", cmdName, f, err)
			continue
		}
		comp := fi.Size()
		uncomp := int64(len(decoded))
		ratio := 0.0
		if uncomp > 0 {
			ratio = 100 * (1 - float64(comp)/float64(uncomp))
		}
		name := f
		name = stdstrings.TrimSuffix(name, ".gz")
		_, _ = fmt.Fprintf(c.Stdout, "%19d %19d %5.1f%% %s\n", comp, uncomp, ratio, name)
	}
	return command.Result{ExitCode: exit}
}

func init() { command.RegisterBuiltin(New()) }
