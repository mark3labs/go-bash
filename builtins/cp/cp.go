// Package cp implements the `cp` built-in.
//
// Flags: -r/-R/-a (recursive/archive), -p (preserve perms+times),
// -f (force overwrite), -L (follow symlinks), -P (no-follow), -T,
// -i (interactive, REJECTED).
package cp

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "cp [-rRapfLPT] SOURCE... DEST"
const helpText = `Usage: cp [OPTION]... SOURCE... DEST
Copy SOURCE to DEST, or multiple SOURCE(s) to DIRECTORY.

  -a, --archive           same as -RpP
  -r, -R, --recursive     copy directories recursively
  -p                      preserve mode and modification times
  -f, --force             remove existing destinations as needed
  -L                      always follow symlinks in SOURCE
  -P, --no-dereference    never follow symlinks in SOURCE
  -T, --no-target-directory  treat DEST as a normal file
  -i                      interactive (REJECTED in sandbox)`

type opts struct {
	recursive, preserve, force, derefL, derefP, noTargetDir bool
}

// New returns the cp command.
func New() command.Command { return command.Define("cp", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	var o opts
	var positional []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-r", a == "-R", a == "--recursive":
			o.recursive = true
		case a == "-a", a == "--archive":
			o.recursive, o.preserve, o.derefP = true, true, true
		case a == "-p":
			o.preserve = true
		case a == "-f", a == "--force":
			o.force = true
		case a == "-L":
			o.derefL = true
		case a == "-P", a == "--no-dereference":
			o.derefP = true
		case a == "-T", a == "--no-target-directory":
			o.noTargetDir = true
		case a == "-i":
			if c.Stderr != nil {
				_, _ = fmt.Fprintln(c.Stderr, "cp: interactive mode not supported in sandbox")
			}
			return command.Result{ExitCode: 1}
		case a == "--":
			i++
			positional = append(positional, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1:
			// Try short-option bundle
			if bundleCpOpts(a, &o) {
				continue
			}
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			positional = append(positional, a)
		}
	}
run:
	if len(positional) < 2 {
		return builtinutil.Errorf(c.Stderr, "cp", 1, "missing file operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "cp", 1, "no filesystem")
	}
	srcs := positional[:len(positional)-1]
	dst := positional[len(positional)-1]
	dstAbs := builtinutil.ResolvePath(c.Cwd, dst)

	dstFI, dstErr := c.FS.Stat(dstAbs)
	dstIsDir := dstErr == nil && dstFI.IsDir()
	if o.noTargetDir && dstIsDir {
		return builtinutil.Errorf(c.Stderr, "cp", 1, "cannot overwrite directory %q with non-directory", dst)
	}
	if len(srcs) > 1 && !dstIsDir {
		return builtinutil.Errorf(c.Stderr, "cp", 1, "target %q is not a directory", dst)
	}

	exit := 0
	for _, s := range srcs {
		srcAbs := builtinutil.ResolvePath(c.Cwd, s)
		var target string
		if dstIsDir && !o.noTargetDir {
			target = path.Join(dstAbs, path.Base(srcAbs))
		} else {
			target = dstAbs
		}
		if err := copyAny(c, srcAbs, target, &o); err != nil {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "cp", 1, "%s: %v", s, err)
		}
	}
	return command.Result{ExitCode: exit}
}

func bundleCpOpts(a string, o *opts) bool {
	if !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
		return false
	}
	for _, r := range a[1:] {
		switch r {
		case 'r', 'R':
			o.recursive = true
		case 'a':
			o.recursive, o.preserve, o.derefP = true, true, true
		case 'p':
			o.preserve = true
		case 'f':
			o.force = true
		case 'L':
			o.derefL = true
		case 'P':
			o.derefP = true
		case 'T':
			o.noTargetDir = true
		default:
			return false
		}
	}
	return true
}

func copyAny(c *command.Context, src, dst string, o *opts) error {
	var fi iofs.FileInfo
	var err error
	if o.derefL || (!o.derefP) {
		fi, err = c.FS.Stat(src)
	} else {
		fi, err = c.FS.Lstat(src)
	}
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		// Recreate symlink at dst.
		link, lerr := c.FS.Readlink(src)
		if lerr != nil {
			return lerr
		}
		_ = c.FS.Remove(dst)
		return c.FS.Symlink(link, dst)
	case fi.IsDir():
		if !o.recursive {
			return errors.New("omitting directory")
		}
		if err := c.FS.MkdirAll(dst, fi.Mode().Perm()); err != nil {
			return err
		}
		entries, err := c.FS.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyAny(c, path.Join(src, e.Name()), path.Join(dst, e.Name()), o); err != nil {
				return err
			}
		}
		if o.preserve {
			_ = c.FS.Chmod(dst, fi.Mode().Perm())
			_ = c.FS.Chtimes(dst, fi.ModTime(), fi.ModTime())
		}
		return nil
	default:
		return copyFile(c, src, dst, fi, o)
	}
}

func copyFile(c *command.Context, src, dst string, fi iofs.FileInfo, o *opts) error {
	if o.force {
		_ = c.FS.Remove(dst)
	}
	data, err := c.FS.ReadFile(src)
	if err != nil {
		return err
	}
	mode := fi.Mode().Perm()
	if !o.preserve {
		// Use default 0o644 unless source had odd bits; coreutils
		// uses src perms minus umask. We approximate with src perms.
		if mode == 0 {
			mode = 0o644
		}
	}
	if err := c.FS.WriteFile(dst, data, mode); err != nil {
		return err
	}
	if o.preserve {
		_ = c.FS.Chmod(dst, mode)
		_ = c.FS.Chtimes(dst, fi.ModTime(), fi.ModTime())
	}
	return nil
}

func init() { command.RegisterBuiltin(New()) }
