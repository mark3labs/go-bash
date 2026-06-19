// Package stat implements the `stat` built-in (SPEC §10 Wave B).
//
// Supports -c FORMAT with %-codes %n %s %F %a %A %u %g %x %y %z %i
// %h %d %t %T. Default format mirrors GNU stat.
package stat

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "stat [-c FORMAT] FILE..."
const helpText = `Usage: stat [OPTION]... FILE...
Display file or file system status.

  -c, --format=FORMAT  use the specified FORMAT instead of the default;
                       output a newline after each use of FORMAT
  -L, --dereference    follow links`

// New returns the stat command.
func New() command.Command { return command.Define("stat", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	format := ""
	follow := false
	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-c", a == "--format":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			format = args[i]
		case strings.HasPrefix(a, "--format="):
			format = strings.TrimPrefix(a, "--format=")
		case a == "-L", a == "--dereference":
			follow = true
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
		return builtinutil.Errorf(c.Stderr, "stat", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "stat", 1, "no filesystem")
	}
	exit := 0
	for _, p := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, p)
		var fi os.FileInfo
		var err error
		if follow {
			fi, err = c.FS.Stat(abs)
		} else {
			fi, err = c.FS.Lstat(abs)
		}
		if err != nil {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "stat", 1, "cannot stat %q: %v", p, err)
			continue
		}
		if format != "" {
			s := formatStat(format, p, fi)
			if c.Stdout != nil {
				_, _ = io.WriteString(c.Stdout, s)
				_, _ = io.WriteString(c.Stdout, "\n")
			}
		} else {
			writeDefault(c.Stdout, p, fi)
		}
	}
	return command.Result{ExitCode: exit}
}

func formatStat(format, name string, fi os.FileInfo) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			// also handle backslash escapes in the format spec
			if format[i] == '\\' && i+1 < len(format) {
				switch format[i+1] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				case '\\':
					b.WriteByte('\\')
				default:
					b.WriteByte(format[i+1])
				}
				i++
				continue
			}
			b.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case 'n':
			b.WriteString(name)
		case 's':
			_, _ = fmt.Fprintf(&b, "%d", fi.Size())
		case 'F':
			b.WriteString(fileKind(fi))
		case 'a':
			_, _ = fmt.Fprintf(&b, "%o", fi.Mode().Perm())
		case 'A':
			b.WriteString(modeString(fi))
		case 'u':
			b.WriteString("1000")
		case 'g':
			b.WriteString("1000")
		case 'x':
			b.WriteString(fi.ModTime().Format(timeFmt))
		case 'y':
			b.WriteString(fi.ModTime().Format(timeFmt))
		case 'z':
			b.WriteString(fi.ModTime().Format(timeFmt))
		case 'i':
			b.WriteString("0")
		case 'h':
			b.WriteString("1")
		case 'd':
			b.WriteString("0")
		case 't':
			b.WriteString("0")
		case 'T':
			b.WriteString("0")
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}

const timeFmt = "2006-01-02 15:04:05.000000000 -0700"

func fileKind(fi os.FileInfo) string {
	switch {
	case fi.IsDir():
		return "directory"
	case fi.Mode()&os.ModeSymlink != 0:
		return "symbolic link"
	case fi.Mode().IsRegular():
		return "regular file"
	default:
		return "special file"
	}
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

func writeDefault(w io.Writer, name string, fi os.FileInfo) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "  File: %s\n", name)
	_, _ = fmt.Fprintf(w, "  Size: %d\tBlocks: %d\t %s\n", fi.Size(), (fi.Size()+511)/512, fileKind(fi))
	_, _ = fmt.Fprintf(w, "Access: (%04o/%s)  Uid: ( 1000/    user)   Gid: ( 1000/    user)\n",
		fi.Mode().Perm(), modeString(fi))
	mt := fi.ModTime()
	_, _ = fmt.Fprintf(w, "Access: %s\n", mt.Format(timeFmt))
	_, _ = fmt.Fprintf(w, "Modify: %s\n", mt.Format(timeFmt))
	_, _ = fmt.Fprintf(w, "Change: %s\n", mt.Format(timeFmt))
	_ = time.Time{}
}

func init() { command.RegisterBuiltin(New()) }
