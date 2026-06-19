// Package ls implements the `ls` built-in (SPEC §10 Wave B).
//
// Flags: -l -a -A -d -F -h -i -R -1 -t -S -r -L -n -p --color.
package ls

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "ls [-laAdFhiR1tSrLnp] [--color[=WHEN]] [PATH...]"
const helpText = `Usage: ls [OPTION]... [FILE]...
List information about the FILEs (the current directory by default).

  -l            use a long listing format
  -a, --all     do not ignore entries starting with .
  -A            do not list . and ..
  -d            list directories themselves, not their contents
  -F            append indicator (one of */=>@|) to entries
  -h            with -l, print human-readable sizes
  -i            print the index number of each file
  -R            list subdirectories recursively
  -1            list one file per line
  -t            sort by modification time, newest first
  -S            sort by file size, largest first
  -r            reverse order while sorting
  -L            dereference symlinks
  -n            with -l, list numeric user / group IDs
  -p            append / indicator to directories
  --color[=WHEN] colorize output (no-op)`

type opts struct {
	long, all, almostAll, dirOnly, classify, human, inode bool
	recursive, oneLine, byTime, bySize, reverse           bool
	deref, numeric, slashDir                              bool
}

// New returns the ls command.
func New() command.Command { return command.Define("ls", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var o opts
	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-l":
			o.long = true
		case a == "-a", a == "--all":
			o.all = true
		case a == "-A", a == "--almost-all":
			o.almostAll = true
		case a == "-d", a == "--directory":
			o.dirOnly = true
		case a == "-F", a == "--classify":
			o.classify = true
		case a == "-h", a == "--human-readable":
			o.human = true
		case a == "-i", a == "--inode":
			o.inode = true
		case a == "-R", a == "--recursive":
			o.recursive = true
		case a == "-1":
			o.oneLine = true
		case a == "-t":
			o.byTime = true
		case a == "-S":
			o.bySize = true
		case a == "-r", a == "--reverse":
			o.reverse = true
		case a == "-L", a == "--dereference":
			o.deref = true
		case a == "-n", a == "--numeric-uid-gid":
			o.long, o.numeric = true, true
		case a == "-p":
			o.slashDir = true
		case strings.HasPrefix(a, "--color"):
			// no-op (sandbox has no TTY)
		case a == "--":
			i++
			paths = append(paths, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// Try bundled short flags.
			if !bundleLsOpts(a, &o) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			paths = append(paths, a)
		}
	}
run:
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "ls", 1, "no filesystem")
	}
	if len(paths) == 0 {
		paths = []string{c.Cwd}
		if paths[0] == "" {
			paths[0] = "."
		}
	}
	exit := 0
	// If multiple paths or recursive, print headers per dir.
	for idx, p := range paths {
		if err := listOne(c, p, &o, idx, len(paths)); err != nil {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "ls", 1, "cannot access %q: %v", p, err)
		}
	}
	return command.Result{ExitCode: exit}
}

func bundleLsOpts(a string, o *opts) bool {
	if !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
		return false
	}
	for _, r := range a[1:] {
		switch r {
		case 'l':
			o.long = true
		case 'a':
			o.all = true
		case 'A':
			o.almostAll = true
		case 'd':
			o.dirOnly = true
		case 'F':
			o.classify = true
		case 'h':
			o.human = true
		case 'i':
			o.inode = true
		case 'R':
			o.recursive = true
		case '1':
			o.oneLine = true
		case 't':
			o.byTime = true
		case 'S':
			o.bySize = true
		case 'r':
			o.reverse = true
		case 'L':
			o.deref = true
		case 'n':
			o.long, o.numeric = true, true
		case 'p':
			o.slashDir = true
		default:
			return false
		}
	}
	return true
}

func listOne(c *command.Context, displayPath string, o *opts, idx, totalPaths int) error {
	abs := builtinutil.ResolvePath(c.Cwd, displayPath)
	var fi os.FileInfo
	var err error
	if o.deref {
		fi, err = c.FS.Stat(abs)
	} else {
		fi, err = c.FS.Lstat(abs)
	}
	if err != nil {
		return err
	}
	if o.dirOnly || !fi.IsDir() {
		writeEntries(c, []entry{{name: displayPath, fi: fi, abs: abs}}, o)
		return nil
	}
	return listDir(c, displayPath, abs, o, idx, totalPaths)
}

type entry struct {
	name string
	fi   os.FileInfo
	abs  string
}

func listDir(c *command.Context, displayPath, abs string, o *opts, idx, totalPaths int) error {
	entries, err := c.FS.ReadDir(abs)
	if err != nil {
		return err
	}
	var items []entry
	for _, e := range entries {
		if !o.all && !o.almostAll && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, entry{name: e.Name(), fi: fi, abs: path.Join(abs, e.Name())})
	}
	sortEntries(items, o)
	if totalPaths > 1 || o.recursive {
		if idx > 0 && c.Stdout != nil {
			_, _ = io.WriteString(c.Stdout, "\n")
		}
		if c.Stdout != nil {
			_, _ = fmt.Fprintf(c.Stdout, "%s:\n", displayPath)
		}
	}
	writeEntries(c, items, o)
	if o.recursive {
		for _, e := range items {
			if e.fi.IsDir() {
				if err := listDir(c, path.Join(displayPath, e.name), e.abs, o, 1, 2); err != nil {
					_ = builtinutil.Errorf(c.Stderr, "ls", 1, "%v", err)
				}
			}
		}
	}
	return nil
}

func sortEntries(items []entry, o *opts) {
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch {
		case o.byTime:
			if a.fi.ModTime().Equal(b.fi.ModTime()) {
				return a.name < b.name
			}
			return a.fi.ModTime().After(b.fi.ModTime())
		case o.bySize:
			if a.fi.Size() == b.fi.Size() {
				return a.name < b.name
			}
			return a.fi.Size() > b.fi.Size()
		default:
			return a.name < b.name
		}
	})
	if o.reverse {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
}

func writeEntries(c *command.Context, items []entry, o *opts) {
	if c.Stdout == nil {
		return
	}
	if o.long {
		for _, e := range items {
			writeLong(c.Stdout, e, o)
		}
		return
	}
	if o.oneLine {
		for _, e := range items {
			writeShortLine(c.Stdout, e, o)
		}
		return
	}
	// Default: space-separated on one line, then newline. Real ls
	// uses column layout; we keep it simple to match a CLI sandbox.
	for i, e := range items {
		if i > 0 {
			_, _ = io.WriteString(c.Stdout, "  ")
		}
		_, _ = io.WriteString(c.Stdout, decorate(e, o))
	}
	if len(items) > 0 {
		_, _ = io.WriteString(c.Stdout, "\n")
	}
}

func writeShortLine(w io.Writer, e entry, o *opts) {
	if o.inode {
		_, _ = fmt.Fprintf(w, "%d ", 0)
	}
	_, _ = fmt.Fprintln(w, decorate(e, o))
}

func writeLong(w io.Writer, e entry, o *opts) {
	mode := modeString(e.fi)
	size := sizeStr(e.fi.Size(), o.human)
	mtime := e.fi.ModTime().Format("Jan _2 15:04")
	uid := "user"
	gid := "user"
	if o.numeric {
		uid, gid = "1000", "1000"
	}
	prefix := ""
	if o.inode {
		prefix = "       0 "
	}
	_, _ = fmt.Fprintf(w, "%s%s 1 %-8s %-8s %8s %s %s\n", prefix, mode, uid, gid, size, mtime, decorate(e, o))
}

func decorate(e entry, o *opts) string {
	name := e.name
	switch {
	case o.classify:
		if e.fi.IsDir() {
			name += "/"
		} else if e.fi.Mode()&os.ModeSymlink != 0 {
			name += "@"
		} else if e.fi.Mode().Perm()&0o111 != 0 {
			name += "*"
		}
	case o.slashDir:
		if e.fi.IsDir() {
			name += "/"
		}
	}
	return name
}

func modeString(fi os.FileInfo) string {
	var b strings.Builder
	mode := fi.Mode()
	switch {
	case fi.IsDir():
		b.WriteByte('d')
	case mode&os.ModeSymlink != 0:
		b.WriteByte('l')
	default:
		b.WriteByte('-')
	}
	const rwx = "rwxrwxrwx"
	for i := 0; i < 9; i++ {
		bit := os.FileMode(1) << uint(8-i)
		if mode&bit != 0 {
			b.WriteByte(rwx[i])
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func sizeStr(n int64, human bool) string {
	if human {
		return builtinutil.HumanSize(n)
	}
	return fmt.Sprintf("%d", n)
}

func init() { command.RegisterBuiltin(New()) }
