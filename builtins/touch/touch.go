// Package touch implements the `touch` built-in.
//
// Flags: -c (no-create), -m (mtime only), -a (atime only),
// -t STAMP, -r REF, -d DATE.
package touch

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "touch [-acm] [-t STAMP|-d DATE|-r REF] FILE..."
const helpText = `Usage: touch [OPTION]... FILE...
Update the access and modification times of each FILE to the current time.

  -a              change only the access time
  -c, --no-create do not create any files
  -d, --date=STR  parse STRING and use it instead of current time
  -m              change only the modification time
  -r, --reference=FILE  use this file's times instead of current time
  -t STAMP        use [[CC]YY]MMDDhhmm[.ss] instead of current time`

// New returns the touch command.
func New() command.Command { return command.Define("touch", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	noCreate := false
	onlyMtime := false
	onlyAtime := false
	var stampStr, dateStr, refFile string

	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-c", a == "--no-create":
			noCreate = true
		case a == "-m":
			onlyMtime = true
		case a == "-a":
			onlyAtime = true
		case a == "-t":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			stampStr = args[i]
		case a == "-d", a == "--date":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			dateStr = args[i]
		case strings.HasPrefix(a, "--date="):
			dateStr = strings.TrimPrefix(a, "--date=")
		case a == "-r", a == "--reference":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			refFile = args[i]
		case strings.HasPrefix(a, "--reference="):
			refFile = strings.TrimPrefix(a, "--reference=")
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
		return builtinutil.Errorf(c.Stderr, "touch", 1, "missing operand")
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "touch", 1, "no filesystem")
	}
	now := time.Now()
	atime, mtime := now, now
	switch {
	case refFile != "":
		fi, err := c.FS.Stat(builtinutil.ResolvePath(c.Cwd, refFile))
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "touch", 1, "failed to get attributes of %q: %v", refFile, err)
		}
		atime = fi.ModTime()
		mtime = fi.ModTime()
	case stampStr != "":
		t, err := parseTouchStamp(stampStr)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "touch", 1, "invalid date format %q", stampStr)
		}
		atime, mtime = t, t
	case dateStr != "":
		t, err := parseTouchDate(dateStr)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "touch", 1, "invalid date format %q", dateStr)
		}
		atime, mtime = t, t
	}
	exit := 0
	for _, p := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, p)
		fi, err := c.FS.Stat(abs)
		exists := err == nil
		if !exists && !errors.Is(err, iofs.ErrNotExist) {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "touch", 1, "cannot touch %q: %v", p, err)
			continue
		}
		if !exists {
			if noCreate {
				continue
			}
			f, ferr := c.FS.OpenFile(abs, os.O_CREATE|os.O_WRONLY, 0o644)
			if ferr != nil {
				exit = 1
				_ = builtinutil.Errorf(c.Stderr, "touch", 1, "cannot touch %q: %v", p, ferr)
				continue
			}
			_ = f.Close()
		}
		a, m := atime, mtime
		if exists && (onlyMtime || onlyAtime) {
			if !onlyAtime {
				a = fi.ModTime()
			}
			if !onlyMtime {
				m = fi.ModTime()
			}
		}
		if err := c.FS.Chtimes(abs, a, m); err != nil {
			exit = 1
			_ = builtinutil.Errorf(c.Stderr, "touch", 1, "setting times of %q: %v", p, err)
		}
	}
	return command.Result{ExitCode: exit}
}

// parseTouchStamp accepts [[CC]YY]MMDDhhmm[.ss].
func parseTouchStamp(s string) (time.Time, error) {
	var sec int
	if dot := strings.LastIndex(s, "."); dot >= 0 {
		if _, err := fmt.Sscanf(s[dot+1:], "%d", &sec); err != nil {
			return time.Time{}, err
		}
		s = s[:dot]
	}
	now := time.Now()
	year := now.Year()
	switch len(s) {
	case 8: // MMDDhhmm
	case 10: // YYMMDDhhmm
		y, err := atoi(s[:2])
		if err != nil {
			return time.Time{}, err
		}
		if y >= 69 {
			year = 1900 + y
		} else {
			year = 2000 + y
		}
		s = s[2:]
	case 12: // CCYYMMDDhhmm
		y, err := atoi(s[:4])
		if err != nil {
			return time.Time{}, err
		}
		year = y
		s = s[4:]
	default:
		return time.Time{}, fmt.Errorf("bad stamp length")
	}
	if len(s) != 8 {
		return time.Time{}, fmt.Errorf("bad stamp")
	}
	mo, _ := atoi(s[0:2])
	d, _ := atoi(s[2:4])
	h, _ := atoi(s[4:6])
	mi, _ := atoi(s[6:8])
	return time.Date(year, time.Month(mo), d, h, mi, sec, 0, time.Local), nil
}

// parseTouchDate handles only a small subset of -d formats:
// RFC3339, "YYYY-MM-DD", "YYYY-MM-DD HH:MM:SS", and "@unixstamp".
func parseTouchDate(s string) (time.Time, error) {
	if strings.HasPrefix(s, "@") {
		v, err := atoi(s[1:])
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(int64(v), 0), nil
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad date")
}

func atoi(s string) (int, error) {
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit %q", c)
		}
		v = v*10 + int(c-'0')
	}
	return v, nil
}

func init() { command.RegisterBuiltin(New()) }
