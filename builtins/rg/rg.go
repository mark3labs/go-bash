// Package rg implements the `rg` (ripgrep) built-in subset (/ Wave D). Only the flags just-bash supports are implemented:
// -i -v -n -c -l -A -B -C -e -t TYPE -g GLOB --hidden --no-ignore --json.
//
// The `--json` flag emits JSON Lines matching ripgrep's
// `begin`/`match`/`end`/`summary` shape.
package rg

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	stdstrings "strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "rg [OPTIONS] PATTERN [PATH...]"
const helpText = `Usage: rg [OPTION]... PATTERNS [PATH]...
Recursively search the current directory for lines matching PATTERNS.

  -i, --ignore-case        ignore case distinctions
  -v, --invert-match       invert match
  -n, --line-number        print line numbers (default)
  -N, --no-line-number     suppress line numbers
  -c, --count              count matching lines per file
  -l, --files-with-matches print only matching files
  -A NUM                   trailing context
  -B NUM                   leading context
  -C NUM                   surrounding context
  -e PATTERN               add pattern (may repeat)
  -t TYPE                  only search files of TYPE (e.g. py, go, md)
  -g GLOB                  include/exclude paths matching GLOB (! to negate)
      --hidden             include hidden files
      --no-ignore          do not honor .gitignore (we never do)
      --json               emit JSON Lines (ripgrep schema)`

// fileType maps a ripgrep TYPE alias to a set of filename globs.
// Subset matching what just-bash ships; not exhaustive ripgrep types.
var fileTypes = map[string][]string{
	"go":   {"*.go"},
	"py":   {"*.py"},
	"js":   {"*.js", "*.mjs", "*.cjs"},
	"ts":   {"*.ts", "*.tsx"},
	"json": {"*.json"},
	"md":   {"*.md", "*.markdown"},
	"yaml": {"*.yaml", "*.yml"},
	"toml": {"*.toml"},
	"sh":   {"*.sh", "*.bash"},
	"rs":   {"*.rs"},
	"c":    {"*.c", "*.h"},
	"cpp":  {"*.cpp", "*.cc", "*.cxx", "*.hpp", "*.hh"},
	"txt":  {"*.txt"},
	"html": {"*.html", "*.htm"},
	"css":  {"*.css"},
	"xml":  {"*.xml"},
}

type opts struct {
	patterns     []string
	ignoreCase   bool
	invert       bool
	lineNumber   bool
	noLineNumber bool
	countOnly    bool
	filesOnly    bool
	after        int
	before       int
	types        []string
	globs        []string
	hidden       bool
	asJSON       bool
}

// New returns the rg command.
func New() command.Command { return command.Define("rg", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	o := opts{lineNumber: true}
	var paths []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-i", a == "--ignore-case":
			o.ignoreCase = true
		case a == "-v", a == "--invert-match":
			o.invert = true
		case a == "-n", a == "--line-number":
			o.lineNumber = true
			o.noLineNumber = false
		case a == "-N", a == "--no-line-number":
			o.noLineNumber = true
			o.lineNumber = false
		case a == "-c", a == "--count":
			o.countOnly = true
		case a == "-l", a == "--files-with-matches":
			o.filesOnly = true
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
		case a == "-e", a == "--regexp":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.patterns = append(o.patterns, args[i])
		case a == "-t", a == "--type":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.types = append(o.types, args[i])
		case a == "-g", a == "--glob":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			o.globs = append(o.globs, args[i])
		case a == "--hidden":
			o.hidden = true
		case a == "--no-ignore":
			// we never honor .gitignore — no-op.
		case a == "--json":
			o.asJSON = true
		case a == "--":
			i++
			paths = append(paths, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			paths = append(paths, a)
		}
	}
run:
	if len(o.patterns) == 0 {
		if len(paths) == 0 {
			return builtinutil.UsageError(c.Stderr, usage)
		}
		o.patterns = append(o.patterns, paths[0])
		paths = paths[1:]
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}

	flags := ""
	if o.ignoreCase {
		flags = "(?i)"
	}
	pat := stdstrings.Join(o.patterns, "|")
	re, err := regexp.Compile(flags + pat)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "rg", 2, "regex: %v", err)
	}

	// Type and glob filters.
	var typeGlobs []string
	for _, t := range o.types {
		if pats, ok := fileTypes[t]; ok {
			typeGlobs = append(typeGlobs, pats...)
		}
	}

	var includes, excludes []string
	for _, g := range o.globs {
		if stdstrings.HasPrefix(g, "!") {
			excludes = append(excludes, g[1:])
		} else {
			includes = append(includes, g)
		}
	}

	var files []string
	for _, p := range paths {
		collect(c, p, p, o.hidden, &files)
	}
	sort.Strings(files)

	anyMatch := false
	for _, f := range files {
		base := path.Base(f)
		if len(typeGlobs) > 0 {
			ok := false
			for _, g := range typeGlobs {
				if m, _ := path.Match(g, base); m {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if len(includes) > 0 {
			ok := false
			for _, g := range includes {
				if m, _ := path.Match(g, base); m {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		skip := false
		for _, g := range excludes {
			if m, _ := path.Match(g, base); m {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if searchFile(c, &o, re, f) {
			anyMatch = true
		}
	}
	if anyMatch {
		return command.Result{ExitCode: 0}
	}
	return command.Result{ExitCode: 1}
}

func collect(c *command.Context, abs, display string, hidden bool, out *[]string) {
	abs = builtinutil.ResolvePath(c.Cwd, abs)
	fi, err := c.FS.Stat(abs)
	if err != nil {
		return
	}
	if !fi.IsDir() {
		*out = append(*out, display)
		return
	}
	walk(c, abs, display, hidden, out)
}

func walk(c *command.Context, abs, display string, hidden bool, out *[]string) {
	entries, err := c.FS.ReadDir(abs)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !hidden && stdstrings.HasPrefix(name, ".") {
			continue
		}
		childAbs := path.Join(abs, name)
		childDisp := path.Join(display, name)
		if e.IsDir() {
			walk(c, childAbs, childDisp, hidden, out)
			continue
		}
		*out = append(*out, childDisp)
	}
}

func searchFile(c *command.Context, o *opts, re *regexp.Regexp, name string) bool {
	abs := builtinutil.ResolvePath(c.Cwd, name)
	data, err := c.FS.ReadFile(abs)
	if err != nil {
		return false
	}
	lines := splitLines(data)

	matchLines := make([]int, 0)
	matchSpans := make(map[int][][2]int)
	for idx, line := range lines {
		spans := re.FindAllStringIndex(line, -1)
		has := spans != nil
		if o.invert {
			has = !has
		}
		if has {
			matchLines = append(matchLines, idx)
			conv := make([][2]int, 0, len(spans))
			for _, s := range spans {
				conv = append(conv, [2]int{s[0], s[1]})
			}
			matchSpans[idx] = conv
		}
	}

	if o.asJSON {
		emitJSON(c.Stdout, name, lines, matchLines, matchSpans, o)
		return len(matchLines) > 0
	}

	if o.filesOnly {
		if len(matchLines) > 0 {
			_, _ = fmt.Fprintf(c.Stdout, "%s\n", name)
		}
		return len(matchLines) > 0
	}
	if o.countOnly {
		_, _ = fmt.Fprintf(c.Stdout, "%s:%d\n", name, len(matchLines))
		return len(matchLines) > 0
	}
	if len(matchLines) == 0 {
		return false
	}

	// Header is the filename per ripgrep convention (only when context spans).
	_, _ = fmt.Fprintf(c.Stdout, "%s\n", name)
	// Determine printed-line set with before/after context.
	printed := make(map[int]bool)
	for _, l := range matchLines {
		for k := l - o.before; k <= l+o.after; k++ {
			if k >= 0 && k < len(lines) {
				printed[k] = true
			}
		}
	}
	keys := make([]int, 0, len(printed))
	for k := range printed {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	prev := -2
	for _, k := range keys {
		if prev >= 0 && k > prev+1 && (o.before > 0 || o.after > 0) {
			_, _ = io.WriteString(c.Stdout, "--\n")
		}
		isMatch := matchSpans[k] != nil
		sep := "-"
		if isMatch {
			sep = ":"
		}
		if o.lineNumber && !o.noLineNumber {
			_, _ = fmt.Fprintf(c.Stdout, "%d%s%s\n", k+1, sep, lines[k])
		} else {
			_, _ = fmt.Fprintf(c.Stdout, "%s\n", lines[k])
		}
		prev = k
	}
	// Trailing newline between files (ripgrep convention).
	_, _ = io.WriteString(c.Stdout, "\n")
	return true
}

// emitJSON emits ripgrep-shape JSON Lines: begin, match*, end, summary.
func emitJSON(w io.Writer, name string, lines []string, matchLines []int, spans map[int][][2]int, o *opts) {
	start := time.Now()
	// begin
	encode(w, map[string]any{
		"type": "begin",
		"data": map[string]any{
			"path": map[string]any{"text": name},
		},
	})
	matched := 0
	for _, idx := range matchLines {
		matched++
		ms := spans[idx]
		subs := make([]map[string]any, 0, len(ms))
		for _, s := range ms {
			subs = append(subs, map[string]any{
				"match": map[string]any{"text": lines[idx][s[0]:s[1]]},
				"start": s[0],
				"end":   s[1],
			})
		}
		// Encode lines as text when valid UTF-8; otherwise base64 (rg's shape).
		text := lines[idx] + "\n"
		var linesField map[string]any
		if valid := stdstrings.ToValidUTF8(text, "") == text; valid {
			linesField = map[string]any{"text": text}
		} else {
			linesField = map[string]any{"bytes": base64.StdEncoding.EncodeToString([]byte(text))}
		}
		encode(w, map[string]any{
			"type": "match",
			"data": map[string]any{
				"path":             map[string]any{"text": name},
				"lines":            linesField,
				"line_number":      idx + 1,
				"absolute_offset":  0,
				"submatches":       subs,
			},
		})
	}
	// end
	dur := time.Since(start)
	encode(w, map[string]any{
		"type": "end",
		"data": map[string]any{
			"path": map[string]any{"text": name},
			"stats": map[string]any{
				"elapsed":          map[string]any{"secs": int(dur.Seconds()), "nanos": int(dur.Nanoseconds() % 1e9), "human": dur.String()},
				"searches":         1,
				"searches_with_match": boolToInt(matched > 0),
				"bytes_searched":   sumBytes(lines),
				"bytes_printed":    0,
				"matched_lines":    matched,
				"matches":          matched,
			},
		},
	})
	// summary (one per file in our simplified shape)
	encode(w, map[string]any{
		"type": "summary",
		"data": map[string]any{
			"elapsed_total": map[string]any{"secs": int(dur.Seconds()), "nanos": int(dur.Nanoseconds() % 1e9), "human": dur.String()},
			"stats": map[string]any{
				"matched_lines":       matched,
				"matches":             matched,
				"searches":            1,
				"searches_with_match": boolToInt(matched > 0),
			},
		},
	})
	_ = o
}

func encode(w io.Writer, v any) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	_, _ = w.Write(buf.Bytes())
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sumBytes(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	return n
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
