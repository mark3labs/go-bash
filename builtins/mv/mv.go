// Package mv implements the `mv` built-in.
//
// Preserves permissions; falls back to copy+remove when c.FS.Rename
// returns a cross-device-like error.
package mv

import (
	"context"
	"errors"
	iofs "io/fs"
	"path"
	"strings"
	"syscall"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "mv [-fi] SOURCE... DEST"
const helpText = `Usage: mv [OPTION]... SOURCE... DEST
Rename SOURCE to DEST, or move SOURCE(s) to DIRECTORY.

  -f, --force         do not prompt before overwriting
  -i                  interactive (REJECTED in sandbox)
  -n, --no-clobber    do not overwrite an existing file`

// New returns the mv command.
func New() command.Command { return command.Define("mv", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	force := false
	noClobber := false
	var positional []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-f", a == "--force":
			force = true
		case a == "-n", a == "--no-clobber":
			noClobber = true
		case a == "-i":
			return builtinutil.Errorf(c.Stderr, "mv", 1, "interactive mode not supported in sandbox")
		case a == "--":
			i++
			positional = append(positional, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			positional = append(positional, a)
		}
	}
run:
	if len(positional) < 2 {
		return builtinutil.Errorf(c.Stderr, "mv", 1, "missing file operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "mv", 1, "no filesystem")
	}
	srcs := positional[:len(positional)-1]
	dst := positional[len(positional)-1]
	dstAbs := builtinutil.ResolvePath(c.Cwd, dst)

	dstFI, dstErr := c.FS.Stat(dstAbs)
	dstIsDir := dstErr == nil && dstFI.IsDir()
	if len(srcs) > 1 && !dstIsDir {
		return builtinutil.Errorf(c.Stderr, "mv", 1, "target %q is not a directory", dst)
	}

	exit := 0
	for _, s := range srcs {
		srcAbs := builtinutil.ResolvePath(c.Cwd, s)
		var target string
		if dstIsDir {
			target = path.Join(dstAbs, path.Base(srcAbs))
		} else {
			target = dstAbs
		}
		if _, err := c.FS.Lstat(target); err == nil {
			if noClobber {
				continue
			}
			if force || !errors.Is(err, iofs.ErrNotExist) {
				_ = c.FS.RemoveAll(target)
			}
		}
		if err := c.FS.Rename(srcAbs, target); err != nil {
			if isCrossDevice(err) {
				if cerr := copyThenRemove(c, srcAbs, target); cerr != nil {
					exit = 1
					_ = builtinutil.Errorf(c.Stderr, "mv", 1, "%s -> %s: %v", s, dst, cerr)
				}
				continue
			}
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "mv", 1, "%s -> %s: %v", s, dst, err)
		}
	}
	return command.Result{ExitCode: exit}
}

func isCrossDevice(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	// String fallback for FS impls that don't wrap syscall errors.
	msg := err.Error()
	return strings.Contains(msg, "cross-device") || strings.Contains(msg, "EXDEV")
}

func copyThenRemove(c *command.Context, src, dst string) error {
	fi, err := c.FS.Lstat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		if err := c.FS.MkdirAll(dst, fi.Mode().Perm()); err != nil {
			return err
		}
		entries, err := c.FS.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyThenRemove(c, path.Join(src, e.Name()), path.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return c.FS.RemoveAll(src)
	}
	data, err := c.FS.ReadFile(src)
	if err != nil {
		return err
	}
	if err := c.FS.WriteFile(dst, data, fi.Mode().Perm()); err != nil {
		return err
	}
	_ = c.FS.Chtimes(dst, fi.ModTime(), fi.ModTime())
	return c.FS.Remove(src)
}

func init() { command.RegisterBuiltin(New()) }
