// Package grep implements the `grep`, `egrep`, and `fgrep` built-ins
// (SPEC §10 Wave D). All three share a single implementation
// parameterized by the default regex mode (BRE, ERE, fixed).
//
// Flags: -E -F -i -v -n -c -l -L -H -h -r -R -w -x -o -A -B -C
// -e PAT -f FILE -q -s --include --exclude --color.
//
// Multi-file output sorts filenames. Recursive output uses path/list
// order from the FS (filename-sorted at each directory level).
package grep

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	stdstrings "strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "grep [OPTIONS] PATTERN [FILE...]"
const helpText = `Usage: grep [OPTION]... PATTERNS [FILE]...
Search for PATTERNS in each FILE.

  -E, --extended-regexp    PATTERNS are extended regular expressions
  -F, --fixed-strings      PATTERNS are fixed strings
  -i, --ignore-case        ignore case distinctions
  -v, --invert-match       select non-matching lines
  -n, --line-number        print line number with output lines
  -c, --count              print only a count of matching lines per FILE
  -l, --files-with-matches print only names of FILEs with matches
  -L, --files-without-match print only names of FILEs without matches
  -H, --with-filename      print file name with output lines
  -h, --no-filename        suppress the file name prefix
  -r, -R, --recursive      recurse into directories
  -w, --word-regexp        match only whole words
  -x, --line-regexp        match only whole lines
  -o, --only-matching      print only the matching parts of lines
  -A NUM, --after-context  print NUM lines of trailing context
  -B NUM, --before-context print NUM lines of leading context
  -C NUM, --context        print NUM lines of context
  -e PATTERN               use PATTERN as the pattern (may repeat)
  -f FILE                  read patterns from FILE
  -q, --quiet              suppress all normal output
  -s, --no-messages        suppress error messages
      --include=GLOB       search only files matching GLOB
      --exclude=GLOB       skip files matching GLOB
      --color[=WHEN]       no-op (sandbox has no TTY)`

// Mode is the default regex mode for a grep variant.
type Mode int

const (
	// ModeBasic — BRE. We emulate BRE by treating the pattern as
	// "extended with most metacharacters escaped" — but Go has no
	// real BRE engine. In practice we just compile via Go's RE2
	// (which accepts a strict-ERE superset); BRE-specific quirks
	// like `\{N,M\}` are rare in test fixtures. Matches just-bash.
	ModeBasic Mode = iota
	// ModeExtended — ERE, the Go regexp/syntax.Perl flavor minus
	// PCRE extensions. Same engine as ModeBasic in practice.
	ModeExtended
	// ModeFixed — literal substring matching. Patterns are NEVER
	// compiled as regex; `-i` lowercases both sides.
	ModeFixed
)

type opts struct {
	mode             Mode
	ignoreCase       bool
	invert           bool
	lineNumber       bool
	countOnly        bool
	filesWithMatch   bool
	filesWithoutMatch bool
	withFilename     bool
	withFilenameSet  bool
	noFilename       bool
	recursive        bool
	wordRegexp       bool
	lineRegexp       bool
	onlyMatching     bool
	quiet            bool
	noMessages       bool
	after            int
	before           int
	patterns         []string
	includeGlob      []string
	excludeGlob      []string
}

// New returns the `grep` command (basic regex by default).
func New() command.Command { return command.Define("grep", runGrep) }

func runGrep(_ context.Context, args []string, c *command.Context) command.Result {
	return Run(c, args, ModeBasic)
}

// Run is the shared entrypoint for grep / egrep / fgrep. `defaultMode`
// controls how patterns are interpreted when neither -E nor -F is on
// the command line.
func Run(c *command.Context, args []string, defaultMode Mode) command.Result {
	o := opts{mode: defaultMode}
	var files []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-E", a == "--extended-regexp":
			o.mode = ModeExtended
		case a == "-F", a == "--fixed-strings":
			o.mode = ModeFixed
		case a == "-G", a == "--basic-regexp":
			o.mode = ModeBasic
		case a == "-i", a == "--ignore-case", a == "-y":
			o.ignoreCase = true
		case a == "-v", a == "--invert-match":
			o.invert = true
		case a == "-n", a == "--line-number":
			o.lineNumber = true
		case a == "-c", a == "--count":
			o.countOnly = true
		case a == "-l", a == "--files-with-matches":
			o.filesWithMatch = true
		case a == "-L", a == "--files-without-match":
			o.filesWithoutMatch = true
		case a == "-H", a == "--with-filename":
			o.withFilename = true
			o.withFilenameSet = true
		case a == "-h", a == "--no-filename":
			o.noFilename = true
			o.withFilenameSet = true
		case a == "-r", a == "-R", a == "--recursive":
			o.recursive = true
		case a == "-w", a == "--word-regexp":
			o.wordRegexp = true
		case a == "-x", a == "--line-regexp":
			o.lineRegexp = true
		case a == "-o", a == "--only-matching":
			o.onlyMatching = true
		case a == "-q", a == "--quiet", a == "--silent":
			o.quiet = true
		case a == "-s", a == "--no-messages":
			o.noMessages = true
		case a == "-A", a == "--after-context":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := parseInt(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.after = n
		case a == "-B", a == "--before-context":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := parseInt(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.before = n
		case a == "-C", a == "--context":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := parseInt(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.before, o.after = n, n
		case stdstrings.HasPrefix(a, "-A"):
			n, err := parseInt(a[2:])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.after = n
		case stdstrings.HasPrefix(a, "-B"):
			n, err := parseInt(a[2:])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.before = n
		case stdstrings.HasPrefix(a, "-C"):
			n, err := parseInt(a[2:])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			o.before, o.after = n, n
		case a == "-e", a == "--regexp":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.patterns = append(o.patterns, splitPatterns(args[i])...)
		case stdstrings.HasPrefix(a, "--regexp="):
			o.patterns = append(o.patterns, splitPatterns(a[len("--regexp="):])...)
		case a == "-f", a == "--file":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			pats, err := readPatternFile(c, args[i])
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "grep", 2, "%s: %v", args[i], err)
			}
			o.patterns = append(o.patterns, pats...)
		case stdstrings.HasPrefix(a, "--include="):
			o.includeGlob = append(o.includeGlob, a[len("--include="):])
		case stdstrings.HasPrefix(a, "--exclude="):
			o.excludeGlob = append(o.excludeGlob, a[len("--exclude="):])
		case a == "--color", stdstrings.HasPrefix(a, "--color="), a == "--colour", stdstrings.HasPrefix(a, "--colour="):
			// no-op (sandbox: no TTY)
		case a == "--":
			i++
			files = append(files, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "--"):
			// Unknown long option.
			return builtinutil.UsageError(c.Stderr, usage)
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			if !bundle(a[1:], &o) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			files = append(files, a)
		}
	}
run:
	if len(o.patterns) == 0 {
		if len(files) == 0 {
			return builtinutil.UsageError(c.Stderr, usage)
		}
		o.patterns = append(o.patterns, splitPatterns(files[0])...)
		files = files[1:]
	}

	matcher, err := buildMatcher(o)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "grep", 2, "%v", err)
	}

	if len(files) == 0 {
		if o.recursive {
			files = []string{"."}
		} else {
			files = []string{"-"}
		}
	}

	// Multi-file output sorts filenames.
	if len(files) > 1 {
		sorted := append([]string(nil), files...)
		sort.Strings(sorted)
		files = sorted
	}

	// Default with-filename behavior: ON for multiple files or
	// recursive, OFF for single-file/stdin — unless -H or -h is set.
	if !o.withFilenameSet {
		if len(files) > 1 || o.recursive {
			o.withFilename = true
		}
	}
	if o.noFilename {
		o.withFilename = false
	}

	anyMatch := false
	anyError := false
	for _, f := range files {
		matched, err := searchFile(c, &o, matcher, f)
		if matched {
			anyMatch = true
		}
		if err != nil {
			anyError = true
		}
	}

	switch {
	case anyMatch:
		return command.Result{ExitCode: 0}
	case anyError:
		return command.Result{ExitCode: 2}
	default:
		return command.Result{ExitCode: 1}
	}
}

// matcher abstracts over RE2 and fixed-string match modes.
type matcher struct {
	regex      *regexp.Regexp
	fixed      []string
	ignoreCase bool
	wordRegexp bool
	lineRegexp bool
	invert     bool
	fixedMode  bool
}

func buildMatcher(o opts) (*matcher, error) {
	m := &matcher{
		ignoreCase: o.ignoreCase,
		wordRegexp: o.wordRegexp,
		lineRegexp: o.lineRegexp,
		invert:     o.invert,
		fixedMode:  o.mode == ModeFixed,
	}
	if o.mode == ModeFixed {
		fixed := make([]string, 0, len(o.patterns))
		for _, p := range o.patterns {
			if o.ignoreCase {
				p = stdstrings.ToLower(p)
			}
			fixed = append(fixed, p)
		}
		m.fixed = fixed
		return m, nil
	}
	// Combine all patterns with alternation.
	pat := stdstrings.Join(o.patterns, "|")
	if o.wordRegexp {
		pat = `(?:^|\W)(?:` + pat + `)(?:$|\W)`
	}
	if o.lineRegexp {
		pat = `\A(?:` + pat + `)\z`
	}
	flags := ""
	if o.ignoreCase {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pat)
	if err != nil {
		return nil, err
	}
	m.regex = re
	return m, nil
}

// findMatch returns the match range or (-1,-1) if no match. For fixed
// mode the first matching pattern wins.
func (m *matcher) findMatch(line string) (start, end int, ok bool) {
	if m.fixedMode {
		hay := line
		if m.ignoreCase {
			hay = stdstrings.ToLower(line)
		}
		for _, p := range m.fixed {
			if m.lineRegexp {
				if hay == p {
					return 0, len(line), true
				}
				continue
			}
			if m.wordRegexp {
				idx := wordIndex(hay, p)
				if idx >= 0 {
					return idx, idx + len(p), true
				}
				continue
			}
			if idx := stdstrings.Index(hay, p); idx >= 0 {
				return idx, idx + len(p), true
			}
		}
		return -1, -1, false
	}
	loc := m.regex.FindStringIndex(line)
	if loc == nil {
		return -1, -1, false
	}
	return loc[0], loc[1], true
}

// allMatches returns every non-overlapping match in line.
func (m *matcher) allMatches(line string) [][2]int {
	if m.fixedMode {
		var out [][2]int
		for _, p := range m.fixed {
			hay := line
			if m.ignoreCase {
				hay = stdstrings.ToLower(line)
			}
			start := 0
			for start < len(hay) {
				idx := stdstrings.Index(hay[start:], p)
				if idx < 0 {
					break
				}
				out = append(out, [2]int{start + idx, start + idx + len(p)})
				start += idx + len(p)
				if len(p) == 0 {
					break
				}
			}
		}
		// Sort by start; later we could merge overlaps.
		sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
		return out
	}
	locs := m.regex.FindAllStringIndex(line, -1)
	out := make([][2]int, 0, len(locs))
	for _, l := range locs {
		out = append(out, [2]int{l[0], l[1]})
	}
	return out
}

// matches reports whether the line matches honoring invert.
func (m *matcher) matches(line string) bool {
	_, _, ok := m.findMatch(line)
	if m.invert {
		return !ok
	}
	return ok
}

func wordIndex(hay, needle string) int {
	for start := 0; start <= len(hay)-len(needle); {
		idx := stdstrings.Index(hay[start:], needle)
		if idx < 0 {
			return -1
		}
		pos := start + idx
		left := pos == 0 || !isWord(rune(hay[pos-1]))
		right := pos+len(needle) == len(hay) || !isWord(rune(hay[pos+len(needle)]))
		if left && right {
			return pos
		}
		start = pos + 1
	}
	return -1
}

func isWord(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '_'
}

func searchFile(c *command.Context, o *opts, m *matcher, name string) (matched bool, err error) {
	if o.recursive && name != "-" {
		return searchRecursive(c, o, m, name)
	}
	return searchOne(c, o, m, name)
}

func searchRecursive(c *command.Context, o *opts, m *matcher, root string) (bool, error) {
	abs := root
	if root != "-" {
		abs = builtinutil.ResolvePath(c.Cwd, root)
	}
	fi, err := c.FS.Stat(abs)
	if err != nil {
		if !o.noMessages {
			_, _ = fmt.Fprintf(c.Stderr, "grep: %s: %v\n", root, err)
		}
		return false, err
	}
	any := false
	var firstErr error
	if !fi.IsDir() {
		mt, e := searchOne(c, o, m, root)
		if mt {
			any = true
		}
		if e != nil {
			firstErr = e
		}
		return any, firstErr
	}
	var files []string
	collect(c, abs, root, &files)
	sort.Strings(files)
	for _, f := range files {
		if !matchGlob(o.includeGlob, o.excludeGlob, path.Base(f)) {
			continue
		}
		mt, e := searchOne(c, o, m, f)
		if mt {
			any = true
		}
		if e != nil && firstErr == nil {
			firstErr = e
		}
	}
	return any, firstErr
}

func collect(c *command.Context, abs, display string, out *[]string) {
	entries, err := c.FS.ReadDir(abs)
	if err != nil {
		return
	}
	for _, e := range entries {
		childAbs := path.Join(abs, e.Name())
		childDisp := path.Join(display, e.Name())
		if e.IsDir() {
			collect(c, childAbs, childDisp, out)
			continue
		}
		*out = append(*out, childDisp)
	}
}

func matchGlob(include, exclude []string, name string) bool {
	if len(include) > 0 {
		ok := false
		for _, g := range include {
			if matched, _ := path.Match(g, name); matched {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, g := range exclude {
		if matched, _ := path.Match(g, name); matched {
			return false
		}
	}
	return true
}

func searchOne(c *command.Context, o *opts, m *matcher, name string) (bool, error) {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		if !o.noMessages {
			_, _ = fmt.Fprintf(c.Stderr, "grep: %s: %v\n", name, err)
		}
		return false, err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	data, err := io.ReadAll(r)
	if err != nil {
		if !o.noMessages {
			_, _ = fmt.Fprintf(c.Stderr, "grep: %s: %v\n", name, err)
		}
		return false, err
	}
	// Preserve trailing newline awareness. split on \n; last empty is dropped.
	lines := splitLines(data)

	displayName := name
	if name == "-" {
		displayName = "(standard input)"
	}

	count := 0
	var buf bytes.Buffer
	// For -A/-B context.
	type pending struct {
		num  int
		text string
	}
	var trailing int // number of trailing context lines still to print
	var lastPrinted = -1
	// Sliding window for "before" context.
	var beforeBuf []pending

	emitLine := func(num int, text string, isMatch bool) {
		// Separator when there is a gap.
		if lastPrinted >= 0 && num > lastPrinted+1 && (o.after > 0 || o.before > 0) {
			buf.WriteString("--\n")
		}
		writeLine(&buf, o, m, displayName, num, text, isMatch)
		lastPrinted = num
	}

	for idx, line := range lines {
		num := idx + 1
		isMatch := m.matches(line)
		if isMatch {
			count++
			// Flush before-context.
			for _, p := range beforeBuf {
				if p.num <= lastPrinted {
					continue
				}
				emitLine(p.num, p.text, false)
			}
			beforeBuf = nil
			emitLine(num, line, true)
			trailing = o.after
		} else if trailing > 0 {
			emitLine(num, line, false)
			trailing--
		} else if o.before > 0 {
			beforeBuf = append(beforeBuf, pending{num: num, text: line})
			if len(beforeBuf) > o.before {
				beforeBuf = beforeBuf[1:]
			}
		}
	}

	switch {
	case o.quiet:
		// no output
	case o.filesWithMatch:
		if count > 0 {
			_, _ = fmt.Fprintf(c.Stdout, "%s\n", displayName)
		}
	case o.filesWithoutMatch:
		if count == 0 {
			_, _ = fmt.Fprintf(c.Stdout, "%s\n", displayName)
		}
	case o.countOnly:
		if o.withFilename {
			_, _ = fmt.Fprintf(c.Stdout, "%s:%d\n", displayName, count)
		} else {
			_, _ = fmt.Fprintf(c.Stdout, "%d\n", count)
		}
	default:
		_, _ = c.Stdout.Write(buf.Bytes())
	}
	return count > 0, nil
}

func writeLine(buf *bytes.Buffer, o *opts, m *matcher, name string, num int, line string, isMatch bool) {
	if o.onlyMatching && isMatch {
		matches := m.allMatches(line)
		for _, mm := range matches {
			prefix := ""
			if o.withFilename {
				prefix += name + ":"
			}
			if o.lineNumber {
				prefix += fmt.Sprintf("%d:", num)
			}
			buf.WriteString(prefix)
			buf.WriteString(line[mm[0]:mm[1]])
			buf.WriteByte('\n')
		}
		return
	}
	prefix := ""
	if o.withFilename {
		prefix += name
		if isMatch {
			prefix += ":"
		} else {
			prefix += "-"
		}
	}
	if o.lineNumber {
		if isMatch {
			prefix += fmt.Sprintf("%d:", num)
		} else {
			prefix += fmt.Sprintf("%d-", num)
		}
	}
	buf.WriteString(prefix)
	buf.WriteString(line)
	buf.WriteByte('\n')
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	s := stdstrings.TrimSuffix(string(data), "\n")
	if s == "" {
		return []string{""}
	}
	return stdstrings.Split(s, "\n")
}

func splitPatterns(s string) []string {
	if s == "" {
		return []string{""}
	}
	return stdstrings.Split(s, "\n")
}

func readPatternFile(c *command.Context, name string) ([]string, error) {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	// One pattern per line; trim trailing newline; keep blank lines
	// — they match every line per GNU grep.
	s := stdstrings.TrimSuffix(string(data), "\n")
	if s == "" {
		return nil, nil
	}
	return stdstrings.Split(s, "\n"), nil
}

func parseInt(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

func init() { command.RegisterBuiltin(New()) }

func bundle(s string, o *opts) bool {
	for _, ch := range s {
		switch ch {
		case 'E':
			o.mode = ModeExtended
		case 'F':
			o.mode = ModeFixed
		case 'G':
			o.mode = ModeBasic
		case 'i', 'y':
			o.ignoreCase = true
		case 'v':
			o.invert = true
		case 'n':
			o.lineNumber = true
		case 'c':
			o.countOnly = true
		case 'l':
			o.filesWithMatch = true
		case 'L':
			o.filesWithoutMatch = true
		case 'H':
			o.withFilename = true
			o.withFilenameSet = true
		case 'h':
			o.noFilename = true
			o.withFilenameSet = true
		case 'r', 'R':
			o.recursive = true
		case 'w':
			o.wordRegexp = true
		case 'x':
			o.lineRegexp = true
		case 'o':
			o.onlyMatching = true
		case 'q':
			o.quiet = true
		case 's':
			o.noMessages = true
		default:
			return false
		}
	}
	return true
}
