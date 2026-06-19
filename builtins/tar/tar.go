// Package tar implements the `tar` built-in (SPEC §10 Wave F).
//
// Supports the GNU/BSD tar flag subset:
//
//	-c, --create        create a new archive
//	-x, --extract       extract files from an archive
//	-t, --list          list contents of an archive
//	-v, --verbose       verbose
//	-f, --file PATH     archive path ("-" = stdin/stdout)
//	-z, --gzip          gzip compress/decompress
//	-j, --bzip2         bzip2 compress/decompress (decompress only)
//	-J, --xz            xz compress/decompress
//	    --zstd          zstd compress/decompress
//	    --no-same-owner ignore owner/group on extract (always implied)
//	    --strip-components N  strip N leading path components on extract
//	-C DIR              change to DIR before any operation
//	--help              show this help and exit
//
// Honors Context.Limits.MaxStringLength on any single member: a tar
// entry whose declared size exceeds the cap is rejected.
//
// All file I/O routes through Context.FS; stdin via Context.Stdin;
// stdout via Context.Stdout.
package tar

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	stdstrings "strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "tar [-cxtvzjJ] [--zstd] [--strip-components N] [--no-same-owner] -f ARCHIVE [PATH...]"

const helpText = `Usage: tar [OPTIONS] -f ARCHIVE [PATH...]
Create, extract, or list a tar archive.

Operation modes:
  -c, --create     create a new archive
  -x, --extract    extract files from an archive
  -t, --list       list contents of an archive

Compression:
  -z, --gzip       use gzip
  -j, --bzip2      use bzip2 (decompress only)
  -J, --xz         use xz
      --zstd       use zstd

Common options:
  -f, --file PATH        archive file (use "-" for stdin/stdout)
  -v, --verbose          verbose output
  -C DIR                 change to DIR before any operation
      --no-same-owner    do not restore file ownership (default)
      --strip-components N  strip N leading components on extract
      --help             show this help and exit
`

type compression int

const (
	compNone compression = iota
	compGzip
	compBzip2
	compXz
	compZstd
)

type op int

const (
	opNone op = iota
	opCreate
	opExtract
	opList
)

type opts struct {
	mode            op
	verbose         bool
	file            string
	fileSet         bool
	comp            compression
	stripComponents int
	chdir           string
	paths           []string
}

// New returns the tar command.
func New() command.Command { return command.Define("tar", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var o opts
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-c", a == "--create":
			o.mode = opCreate
		case a == "-x", a == "--extract", a == "--get":
			o.mode = opExtract
		case a == "-t", a == "--list":
			o.mode = opList
		case a == "-v", a == "--verbose":
			o.verbose = true
		case a == "-z", a == "--gzip", a == "--gunzip", a == "--ungzip":
			o.comp = compGzip
		case a == "-j", a == "--bzip2":
			o.comp = compBzip2
		case a == "-J", a == "--xz":
			o.comp = compXz
		case a == "--zstd":
			o.comp = compZstd
		case a == "-f", a == "--file":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.file = args[i]
			o.fileSet = true
		case stdstrings.HasPrefix(a, "--file="):
			o.file = stdstrings.TrimPrefix(a, "--file=")
			o.fileSet = true
		case a == "-C", a == "--directory":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.chdir = args[i]
		case stdstrings.HasPrefix(a, "--directory="):
			o.chdir = stdstrings.TrimPrefix(a, "--directory=")
		case a == "--no-same-owner", a == "--no-same-permissions", a == "-o":
			// always implied — we have no concept of owner/group in memfs
		case a == "--strip-components":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return builtinutil.Errorf(c.Stderr, "tar", 2, "invalid --strip-components value %q", args[i])
			}
			o.stripComponents = n
		case stdstrings.HasPrefix(a, "--strip-components="):
			n, err := strconv.Atoi(stdstrings.TrimPrefix(a, "--strip-components="))
			if err != nil || n < 0 {
				return builtinutil.Errorf(c.Stderr, "tar", 2, "invalid --strip-components value")
			}
			o.stripComponents = n
		case a == "--":
			i++
			o.paths = append(o.paths, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "--"):
			return builtinutil.UsageError(c.Stderr, usage)
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			// Short-flag bundle (e.g. -czvf archive.tar.gz src/)
			if !parseBundle(args, &i, &o) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			o.paths = append(o.paths, a)
		}
	}
run:
	if o.mode == opNone {
		return builtinutil.Errorf(c.Stderr, "tar", 2, "you must specify one of -c, -x, or -t")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "tar", 1, "no filesystem")
	}
	if o.chdir != "" {
		// Apply -C by rewriting c.Cwd locally for path resolution.
		c = withCwd(c, builtinutil.ResolvePath(c.Cwd, o.chdir))
	}
	switch o.mode {
	case opCreate:
		return runCreate(c, &o)
	case opExtract:
		return runExtract(c, &o)
	case opList:
		return runList(c, &o)
	}
	return command.Result{ExitCode: 0}
}

// withCwd returns a shallow copy of c with Cwd replaced. Used by -C.
func withCwd(c *command.Context, cwd string) *command.Context {
	cp := *c
	cp.Cwd = cwd
	return &cp
}

// parseBundle consumes a bundled short-flag argument like `-czvf`.
// If the bundle includes `f`, it consumes the next argv element as
// the archive path. Returns false on an unknown flag.
func parseBundle(args []string, idx *int, o *opts) bool {
	a := args[*idx]
	wantFile := false
	for _, ch := range a[1:] {
		switch ch {
		case 'c':
			o.mode = opCreate
		case 'x':
			o.mode = opExtract
		case 't':
			o.mode = opList
		case 'v':
			o.verbose = true
		case 'z':
			o.comp = compGzip
		case 'j':
			o.comp = compBzip2
		case 'J':
			o.comp = compXz
		case 'f':
			wantFile = true
		case 'o':
			// no-op (no-same-owner)
		default:
			return false
		}
	}
	if wantFile {
		if *idx+1 >= len(args) {
			return false
		}
		*idx++
		o.file = args[*idx]
		o.fileSet = true
	}
	return true
}

// openArchiveReader returns a reader for the archive file (or stdin when
// "-" or unset). The returned closer (may be nil) should be invoked
// after use.
func openArchiveReader(c *command.Context, o *opts) (io.Reader, io.Closer, error) {
	if !o.fileSet || o.file == "-" || o.file == "" {
		if c.Stdin == nil {
			return stdstrings.NewReader(""), nil, nil
		}
		return c.Stdin, nil, nil
	}
	abs := builtinutil.ResolvePath(c.Cwd, o.file)
	f, err := c.FS.OpenFile(abs, 0, 0)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

// openArchiveWriter returns a writer for the archive file (or stdout).
func openArchiveWriter(c *command.Context, o *opts) (io.Writer, io.Closer, error) {
	if !o.fileSet || o.file == "-" || o.file == "" {
		return c.Stdout, nil, nil
	}
	abs := builtinutil.ResolvePath(c.Cwd, o.file)
	f, err := c.FS.Create(abs)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

// decompressReader wraps r with the configured codec.
func decompressReader(r io.Reader, comp compression) (io.Reader, io.Closer, error) {
	switch comp {
	case compNone:
		return r, nil, nil
	case compGzip:
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return gr, gr, nil
	case compBzip2:
		return bzip2.NewReader(r), nil, nil
	case compXz:
		xr, err := xz.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return xr, nil, nil
	case compZstd:
		zr, err := zstd.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return zr, &zstdCloser{r: zr}, nil
	}
	return r, nil, nil
}

type zstdCloser struct{ r *zstd.Decoder }

func (z *zstdCloser) Close() error { z.r.Close(); return nil }

// compressWriter wraps w with the configured codec. Bzip2 is
// decompress-only via stdlib; we reject it here.
func compressWriter(w io.Writer, comp compression) (io.WriteCloser, error) {
	switch comp {
	case compNone:
		return nopWriteCloser{w}, nil
	case compGzip:
		return gzip.NewWriter(w), nil
	case compBzip2:
		return nil, fmt.Errorf("bzip2 compression not supported")
	case compXz:
		return xz.NewWriter(w)
	case compZstd:
		return zstd.NewWriter(w)
	}
	return nopWriteCloser{w}, nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// runList prints one entry per archive member, like `tar tvf`.
func runList(c *command.Context, o *opts) command.Result {
	r, closer, err := openArchiveReader(c, o)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
	}
	if closer != nil {
		defer closer.Close() //nolint:errcheck
	}
	dr, dcloser, err := decompressReader(r, o.comp)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
	}
	if dcloser != nil {
		defer dcloser.Close() //nolint:errcheck
	}
	tr := tar.NewReader(dr)
	maxStr := c.Limits.MaxStringLength
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
		}
		if maxStr > 0 && hdr.Size > int64(maxStr) {
			return builtinutil.Errorf(c.Stderr, "tar", 1, "member %q exceeds MaxStringLength", hdr.Name)
		}
		if o.verbose {
			_, _ = fmt.Fprintln(c.Stdout, formatVerboseHeader(hdr))
		} else {
			_, _ = fmt.Fprintln(c.Stdout, hdr.Name)
		}
	}
	return command.Result{ExitCode: 0}
}

func formatVerboseHeader(hdr *tar.Header) string {
	modeStr := iofs.FileMode(hdr.Mode & 0o7777).String()
	switch hdr.Typeflag {
	case tar.TypeDir:
		modeStr = "d" + modeStr[1:]
	case tar.TypeSymlink:
		modeStr = "l" + modeStr[1:]
	default:
		modeStr = "-" + modeStr[1:]
	}
	mt := hdr.ModTime
	if mt.IsZero() {
		mt = time.Unix(0, 0).UTC()
	}
	name := hdr.Name
	if hdr.Typeflag == tar.TypeSymlink && hdr.Linkname != "" {
		name = fmt.Sprintf("%s -> %s", name, hdr.Linkname)
	}
	return fmt.Sprintf("%s %d/%d %d %s %s",
		modeStr, hdr.Uid, hdr.Gid, hdr.Size, mt.UTC().Format("2006-01-02 15:04"), name)
}

// runExtract extracts members into c.FS, rooted at c.Cwd (already
// adjusted for -C).
func runExtract(c *command.Context, o *opts) command.Result {
	r, closer, err := openArchiveReader(c, o)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
	}
	if closer != nil {
		defer closer.Close() //nolint:errcheck
	}
	dr, dcloser, err := decompressReader(r, o.comp)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
	}
	if dcloser != nil {
		defer dcloser.Close() //nolint:errcheck
	}
	tr := tar.NewReader(dr)
	maxStr := c.Limits.MaxStringLength
	wanted := pathSet(o.paths)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
		}
		if maxStr > 0 && hdr.Size > int64(maxStr) {
			return builtinutil.Errorf(c.Stderr, "tar", 1, "member %q exceeds MaxStringLength", hdr.Name)
		}
		name := stripPath(hdr.Name, o.stripComponents)
		if name == "" {
			continue
		}
		if wanted != nil && !wantedMember(wanted, hdr.Name, name) {
			continue
		}
		if o.verbose {
			_, _ = fmt.Fprintln(c.Stdout, name)
		}
		abs := builtinutil.ResolvePath(c.Cwd, name)
		mode := iofs.FileMode(hdr.Mode & 0o7777)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := c.FS.MkdirAll(abs, mode|0o700); err != nil {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
			}
		case tar.TypeSymlink:
			_ = c.FS.Remove(abs)
			if err := mkdirParents(c, abs); err != nil {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
			}
			if err := c.FS.Symlink(hdr.Linkname, abs); err != nil {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
			}
		case tar.TypeLink:
			_ = c.FS.Remove(abs)
			target := builtinutil.ResolvePath(c.Cwd, stripPath(hdr.Linkname, o.stripComponents))
			if err := mkdirParents(c, abs); err != nil {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
			}
			if err := c.FS.Link(target, abs); err != nil {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
			}
		case tar.TypeReg:
			if err := mkdirParents(c, abs); err != nil {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
			}
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, tr); err != nil {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
			}
			if maxStr > 0 && buf.Len() > maxStr {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "member %q exceeds MaxStringLength", hdr.Name)
			}
			if err := c.FS.WriteFile(abs, buf.Bytes(), mode|0o600); err != nil {
				return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
			}
			if !hdr.ModTime.IsZero() {
				_ = c.FS.Chtimes(abs, hdr.ModTime, hdr.ModTime)
			}
		default:
			// silently skip unknown types (block, char, fifo, etc.)
		}
	}
	return command.Result{ExitCode: 0}
}

func mkdirParents(c *command.Context, abs string) error {
	dir := path.Dir(abs)
	if dir == "" || dir == "/" || dir == "." {
		return nil
	}
	return c.FS.MkdirAll(dir, 0o755)
}

// pathSet returns a lookup set if user supplied member filters, else nil.
func pathSet(paths []string) map[string]struct{} {
	if len(paths) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		out[p] = struct{}{}
	}
	return out
}

// wantedMember reports whether the archive header matches a filter.
// Matches by exact name OR prefix-with-slash (so passing "dir" extracts
// every entry under it).
func wantedMember(set map[string]struct{}, hdrName, strippedName string) bool {
	for k := range set {
		if hdrName == k || strippedName == k {
			return true
		}
		if stdstrings.HasPrefix(hdrName, k+"/") || stdstrings.HasPrefix(strippedName, k+"/") {
			return true
		}
	}
	return false
}

// stripPath drops n leading path components from p.
func stripPath(p string, n int) string {
	if n <= 0 {
		return p
	}
	parts := stdstrings.Split(p, "/")
	if len(parts) <= n {
		return ""
	}
	return stdstrings.Join(parts[n:], "/")
}

// runCreate builds an archive from o.paths.
func runCreate(c *command.Context, o *opts) command.Result {
	if len(o.paths) == 0 {
		return builtinutil.Errorf(c.Stderr, "tar", 2, "create: no files specified")
	}
	w, closer, err := openArchiveWriter(c, o)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
	}
	if closer != nil {
		defer closer.Close() //nolint:errcheck
	}
	cw, err := compressWriter(w, o.comp)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tar", 2, "%v", err)
	}
	tw := tar.NewWriter(cw)
	maxStr := c.Limits.MaxStringLength
	for _, p := range o.paths {
		if err := addToArchive(c, tw, p, o, maxStr); err != nil {
			_ = tw.Close()
			_ = cw.Close()
			return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
		}
	}
	if err := tw.Close(); err != nil {
		return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
	}
	if err := cw.Close(); err != nil {
		return builtinutil.Errorf(c.Stderr, "tar", 1, "%v", err)
	}
	return command.Result{ExitCode: 0}
}

// addToArchive walks p (file or directory) and writes entries.
// The header.Name preserves the user-supplied root (e.g. "src/foo.txt").
func addToArchive(c *command.Context, tw *tar.Writer, root string, o *opts, maxStr int) error {
	abs := builtinutil.ResolvePath(c.Cwd, root)
	return walkArchive(c, tw, abs, root, o, maxStr)
}

func walkArchive(c *command.Context, tw *tar.Writer, abs, rel string, o *opts, maxStr int) error {
	fi, err := c.FS.Lstat(abs)
	if err != nil {
		return err
	}
	hdr, err := tarHeaderFromFI(c, fi, abs, rel)
	if err != nil {
		return err
	}
	if maxStr > 0 && hdr.Size > int64(maxStr) {
		return fmt.Errorf("member %q exceeds MaxStringLength", rel)
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if o.verbose {
		_, _ = fmt.Fprintln(c.Stdout, rel)
	}
	switch {
	case fi.Mode()&iofs.ModeSymlink != 0:
		// symlink content is in header
	case fi.IsDir():
		entries, err := c.FS.ReadDir(abs)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			if err := walkArchive(c, tw, path.Join(abs, e.Name()), path.Join(rel, e.Name()), o, maxStr); err != nil {
				return err
			}
		}
	default:
		data, err := c.FS.ReadFile(abs)
		if err != nil {
			return err
		}
		if maxStr > 0 && len(data) > maxStr {
			return fmt.Errorf("member %q exceeds MaxStringLength", rel)
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func tarHeaderFromFI(c *command.Context, fi os.FileInfo, abs, rel string) (*tar.Header, error) {
	hdr := &tar.Header{
		Name:    rel,
		Mode:    int64(fi.Mode().Perm()),
		ModTime: fi.ModTime(),
	}
	switch {
	case fi.Mode()&iofs.ModeSymlink != 0:
		hdr.Typeflag = tar.TypeSymlink
		link, err := c.FS.Readlink(abs)
		if err != nil {
			return nil, err
		}
		hdr.Linkname = link
	case fi.IsDir():
		hdr.Typeflag = tar.TypeDir
		if !stdstrings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
	default:
		hdr.Typeflag = tar.TypeReg
		hdr.Size = fi.Size()
	}
	return hdr, nil
}

func init() { command.RegisterBuiltin(New()) }
