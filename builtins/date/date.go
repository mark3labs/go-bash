// Package date implements the `date` built-in.
//
// Usage:
//
//	date [OPTIONS] [+FORMAT]
//
// Options:
//
//	-u, --utc          force UTC (default; date defaults to UTC
//	                   unless $TZ is set in the environment)
//	-d, --date=STRING  parse STRING as the source time
//	-r, --reference=FILE  use FILE's modification time
//	-R, --rfc-2822     output RFC 2822 / RFC 822 date format
//	-I[FMT]            output ISO 8601 (FMT in date/hours/minutes/seconds/ns)
//	--help             show this help and exit
//
// FORMAT is a strftime-style string (most %-conversions supported);
// Wave G for the supported subset.
//
// All file I/O routes through Context.FS; stdout via Context.Stdout.
package date

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "date [-uR] [-d STRING] [-r FILE] [+FORMAT]"

const helpText = `Usage: date [OPTIONS] [+FORMAT]
Display the current time in the given FORMAT, or set the system date.

Options:
  -u, --utc                force UTC (default unless $TZ is set)
  -d, --date=STRING        parse STRING as the source time
  -r, --reference=FILE     use FILE's modification time
  -R, --rfc-2822           output RFC 2822 format
  -I[FMT]                  output ISO 8601 (FMT=date|hours|minutes|seconds|ns)
      --help               show this help and exit
`

// New returns the date command.
func New() command.Command { return command.Define("date", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	forceUTC := false
	var dateStr string
	dateSet := false
	var refFile string
	refSet := false
	mode := "default" // default | rfc | iso
	var isoFmt string
	var format string

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case a == "-u" || a == "--utc":
			forceUTC = true
		case a == "-R" || a == "--rfc-2822" || a == "--rfc-email":
			mode = "rfc"
		case a == "-I":
			mode = "iso"
			isoFmt = "date"
		case strings.HasPrefix(a, "-I") && len(a) > 2:
			mode = "iso"
			isoFmt = a[2:]
		case strings.HasPrefix(a, "--iso-8601"):
			mode = "iso"
			isoFmt = "date"
			if idx := strings.IndexByte(a, '='); idx >= 0 {
				isoFmt = a[idx+1:]
			}
		case a == "-d" || a == "--date":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			dateStr = args[i]
			dateSet = true
		case strings.HasPrefix(a, "--date="):
			dateStr = strings.TrimPrefix(a, "--date=")
			dateSet = true
		case a == "-r" || a == "--reference":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			refFile = args[i]
			refSet = true
		case strings.HasPrefix(a, "--reference="):
			refFile = strings.TrimPrefix(a, "--reference=")
			refSet = true
		case strings.HasPrefix(a, "+"):
			format = a[1:]
			i++
			goto done
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	// Extra positional operands after + or -- form: ignore (real date
	// would treat unknown positionals as an error, but Wave G keeps
	// the surface minimal).
	_ = args[i:]

	// Resolve the source time.
	var t time.Time
	switch {
	case refSet:
		if c.FS == nil {
			return builtinutil.Errorf(c.Stderr, "date", 1, "no filesystem")
		}
		path := builtinutil.ResolvePath(c.Cwd, refFile)
		fi, err := c.FS.Stat(path)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "date", 1, "%v", err)
		}
		t = fi.ModTime()
	case dateSet:
		parsed, err := parseDateString(dateStr)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "date", 1, "invalid date %q", dateStr)
		}
		t = parsed
	default:
		t = time.Now()
	}

	// Determine the timezone. The spec: defaults to UTC unless $TZ is
	// set; -u forces UTC.
	loc := time.UTC
	if !forceUTC {
		if tz := c.Env["TZ"]; tz != "" {
			if l, err := time.LoadLocation(tz); err == nil {
				loc = l
			}
		}
	}
	t = t.In(loc)

	var out string
	switch mode {
	case "rfc":
		// "Mon, 02 Jan 2006 15:04:05 -0700"
		out = t.Format(time.RFC1123Z)
	case "iso":
		out = isoFormat(t, isoFmt)
		if out == "" {
			return builtinutil.Errorf(c.Stderr, "date", 1, "invalid -I argument %q", isoFmt)
		}
	default:
		if format == "" {
			// GNU date default: "Wed Jun 18 13:30:00 UTC 2026"
			out = t.Format("Mon Jan  2 15:04:05 MST 2006")
		} else {
			out = strftime(format, t)
		}
	}
	if c.Stdout != nil {
		_, _ = fmt.Fprintln(c.Stdout, out)
	}
	return command.Result{ExitCode: 0}
}

// parseDateString accepts a small but useful subset of GNU date input:
//   - "now", "today"
//   - RFC3339 / RFC3339Nano
//   - "@<unix_seconds>" (possibly fractional)
//   - Several common Go layouts (date-only, ISO date-time, ...)
func parseDateString(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "now", "today":
		return time.Now(), nil
	}
	if strings.HasPrefix(s, "@") {
		raw := s[1:]
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return time.Time{}, err
		}
		sec := int64(f)
		nsec := int64((f - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC(), nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.UnixDate,
		"Jan 2 15:04:05 2006",
		"Jan 2 2006",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date: %q", s)
}

// isoFormat builds an ISO 8601 string per the -I argument.
func isoFormat(t time.Time, fmtName string) string {
	switch fmtName {
	case "", "date":
		return t.Format("2006-01-02")
	case "hours", "hour", "h":
		return t.Format("2006-01-02T15-0700")
	case "minutes", "minute", "m":
		return t.Format("2006-01-02T15:04-0700")
	case "seconds", "second", "s":
		return t.Format("2006-01-02T15:04:05-0700")
	case "ns", "nanoseconds":
		return t.Format("2006-01-02T15:04:05.999999999-0700")
	}
	return ""
}

// strftime renders t according to the strftime format string.
// Unsupported conversions are emitted verbatim (`%X` → `%X`).
func strftime(format string, t time.Time) string {
	var b strings.Builder
	b.Grow(len(format))
	for i := 0; i < len(format); i++ {
		ch := format[i]
		if ch != '%' || i+1 >= len(format) {
			b.WriteByte(ch)
			continue
		}
		i++
		switch format[i] {
		case 'a':
			b.WriteString(t.Format("Mon"))
		case 'A':
			b.WriteString(t.Format("Monday"))
		case 'b', 'h':
			b.WriteString(t.Format("Jan"))
		case 'B':
			b.WriteString(t.Format("January"))
		case 'c':
			b.WriteString(t.Format("Mon Jan  2 15:04:05 2006"))
		case 'C':
			fmt.Fprintf(&b, "%02d", t.Year()/100)
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'D':
			b.WriteString(t.Format("01/02/06"))
		case 'e':
			fmt.Fprintf(&b, "%2d", t.Day())
		case 'F':
			b.WriteString(t.Format("2006-01-02"))
		case 'g':
			y, _ := t.ISOWeek()
			fmt.Fprintf(&b, "%02d", y%100)
		case 'G':
			y, _ := t.ISOWeek()
			fmt.Fprintf(&b, "%04d", y)
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			fmt.Fprintf(&b, "%02d", h)
		case 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case 'k':
			fmt.Fprintf(&b, "%2d", t.Hour())
		case 'l':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			fmt.Fprintf(&b, "%2d", h)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'n':
			b.WriteByte('\n')
		case 'N':
			fmt.Fprintf(&b, "%09d", t.Nanosecond())
		case 'p':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'P':
			if t.Hour() < 12 {
				b.WriteString("am")
			} else {
				b.WriteString("pm")
			}
		case 'r':
			b.WriteString(t.Format("03:04:05 PM"))
		case 'R':
			b.WriteString(t.Format("15:04"))
		case 's':
			fmt.Fprintf(&b, "%d", t.Unix())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 't':
			b.WriteByte('\t')
		case 'T':
			b.WriteString(t.Format("15:04:05"))
		case 'u':
			d := int(t.Weekday())
			if d == 0 {
				d = 7
			}
			fmt.Fprintf(&b, "%d", d)
		case 'U':
			fmt.Fprintf(&b, "%02d", weekOfYearSunday(t))
		case 'V':
			_, w := t.ISOWeek()
			fmt.Fprintf(&b, "%02d", w)
		case 'w':
			fmt.Fprintf(&b, "%d", int(t.Weekday()))
		case 'W':
			fmt.Fprintf(&b, "%02d", weekOfYearMonday(t))
		case 'x':
			b.WriteString(t.Format("01/02/06"))
		case 'X':
			b.WriteString(t.Format("15:04:05"))
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'Y':
			fmt.Fprintf(&b, "%04d", t.Year())
		case 'z':
			b.WriteString(t.Format("-0700"))
		case 'Z':
			b.WriteString(t.Format("MST"))
		case '%':
			b.WriteByte('%')
		default:
			// Unknown conversion: emit verbatim.
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}

func weekOfYearSunday(t time.Time) int {
	yday := t.YearDay() - 1
	wday := int(t.Weekday())
	return (yday + 7 - wday) / 7
}

func weekOfYearMonday(t time.Time) int {
	yday := t.YearDay() - 1
	wday := int(t.Weekday())
	if wday == 0 {
		wday = 7
	}
	return (yday + 7 - (wday - 1)) / 7
}

func init() { command.RegisterBuiltin(New()) }
