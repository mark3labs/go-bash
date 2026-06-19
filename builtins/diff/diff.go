// Package diff implements the `diff` built-in (SPEC §10 Wave C).
//
// Flags: -u unified, -q brief, -r recursive, -N treat absent as empty,
// -y side-by-side (minimal). Exit codes: 0 same, 1 different, 2 error.
package diff

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "diff [-uqrNy] FILE1 FILE2"
const helpText = `Usage: diff [OPTION]... FILE1 FILE2
Compare files line by line.

  -u, --unified         output unified diff format
  -q, --brief           report only when files differ
  -r, --recursive       recursively compare directories
  -N, --new-file        treat absent files as empty
  -y, --side-by-side    side-by-side (minimal)

Exit status: 0 same, 1 different, 2 error.`

// New returns the diff command.
func New() command.Command { return command.Define("diff", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	unified := false
	brief := false
	recursive := false
	newFile := false
	sideBySide := false
	var pos []string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-u", a == "--unified":
			unified = true
		case a == "-q", a == "--brief":
			brief = true
		case a == "-r", a == "--recursive":
			recursive = true
		case a == "-N", a == "--new-file":
			newFile = true
		case a == "-y", a == "--side-by-side":
			sideBySide = true
		case a == "--":
			i++
			pos = append(pos, args[i:]...)
			goto run
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			ok := true
			for _, ch := range a[1:] {
				switch ch {
				case 'u':
					unified = true
				case 'q':
					brief = true
				case 'r':
					recursive = true
				case 'N':
					newFile = true
				case 'y':
					sideBySide = true
				default:
					ok = false
				}
			}
			if !ok {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			pos = append(pos, a)
		}
	}
run:
	_ = recursive
	if len(pos) != 2 {
		return builtinutil.Errorf(c.Stderr, "diff", 2, "need exactly two files")
	}
	a, err := readFile(c, pos[0], newFile)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "diff", 2, "%s: %v", pos[0], err)
	}
	b, err := readFile(c, pos[1], newFile)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "diff", 2, "%s: %v", pos[1], err)
	}
	if a == b {
		return command.Result{}
	}
	if brief {
		_, _ = fmt.Fprintf(c.Stdout, "Files %s and %s differ\n", pos[0], pos[1])
		return command.Result{ExitCode: 1}
	}
	if sideBySide {
		emitSide(c.Stdout, pos[0], pos[1], a, b)
		return command.Result{ExitCode: 1}
	}
	if unified {
		emitUnified(c.Stdout, pos[0], pos[1], a, b)
	} else {
		emitNormal(c.Stdout, a, b)
	}
	return command.Result{ExitCode: 1}
}

func readFile(c *command.Context, name string, newFile bool) (string, error) {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		if newFile {
			return "", nil
		}
		return "", err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func emitNormal(w io.Writer, a, b string) {
	la := splitLines(a)
	lb := splitLines(b)
	hunks := diffHunks(la, lb)
	for _, h := range hunks {
		switch {
		case h.aCount == 0:
			_, _ = fmt.Fprintf(w, "%da%d,%d\n", h.aStart, h.bStart+1, h.bStart+h.bCount)
			for _, l := range lb[h.bStart : h.bStart+h.bCount] {
				_, _ = fmt.Fprintf(w, "> %s\n", l)
			}
		case h.bCount == 0:
			_, _ = fmt.Fprintf(w, "%d,%dd%d\n", h.aStart+1, h.aStart+h.aCount, h.bStart)
			for _, l := range la[h.aStart : h.aStart+h.aCount] {
				_, _ = fmt.Fprintf(w, "< %s\n", l)
			}
		default:
			_, _ = fmt.Fprintf(w, "%d,%dc%d,%d\n", h.aStart+1, h.aStart+h.aCount, h.bStart+1, h.bStart+h.bCount)
			for _, l := range la[h.aStart : h.aStart+h.aCount] {
				_, _ = fmt.Fprintf(w, "< %s\n", l)
			}
			_, _ = io.WriteString(w, "---\n")
			for _, l := range lb[h.bStart : h.bStart+h.bCount] {
				_, _ = fmt.Fprintf(w, "> %s\n", l)
			}
		}
	}
}

func emitUnified(w io.Writer, na, nb, a, b string) {
	la := splitLines(a)
	lb := splitLines(b)
	_, _ = fmt.Fprintf(w, "--- %s\n", na)
	_, _ = fmt.Fprintf(w, "+++ %s\n", nb)
	// Single hunk encompassing entire file (simple impl).
	_, _ = fmt.Fprintf(w, "@@ -1,%d +1,%d @@\n", len(la), len(lb))
	// emit LCS-based diff per-line
	ops := lcsOps(la, lb)
	for _, op := range ops {
		_, _ = fmt.Fprintln(w, op)
	}
}

func emitSide(w io.Writer, na, nb, a, b string) {
	la := splitLines(a)
	lb := splitLines(b)
	_ = na
	_ = nb
	n := len(la)
	if len(lb) > n {
		n = len(lb)
	}
	for i := 0; i < n; i++ {
		var x, y string
		var mark byte = ' '
		if i < len(la) {
			x = la[i]
		} else {
			mark = '>'
		}
		if i < len(lb) {
			y = lb[i]
		} else {
			mark = '<'
		}
		if x != y && i < len(la) && i < len(lb) {
			mark = '|'
		}
		_, _ = fmt.Fprintf(w, "%-30s %c %s\n", x, mark, y)
	}
}

type hunk struct {
	aStart, aCount, bStart, bCount int
}

// diffHunks computes a very simple hunk list — collapse runs of
// non-matching lines. Uses LCS.
func diffHunks(a, b []string) []hunk {
	ops := lcsOpsRaw(a, b)
	var hunks []hunk
	var cur hunk
	cur = hunk{}
	hasCur := false
	ai, bi := 0, 0
	flush := func() {
		if hasCur {
			hunks = append(hunks, cur)
			hasCur = false
		}
	}
	for _, op := range ops {
		switch op.kind {
		case ' ':
			flush()
			ai++
			bi++
		case '-':
			if !hasCur {
				cur = hunk{aStart: ai, bStart: bi}
				hasCur = true
			}
			cur.aCount++
			ai++
		case '+':
			if !hasCur {
				cur = hunk{aStart: ai, bStart: bi}
				hasCur = true
			}
			cur.bCount++
			bi++
		}
	}
	flush()
	return hunks
}

type op struct {
	kind byte
	line string
}

func lcsOpsRaw(a, b []string) []op {
	// LCS length matrix
	la, lb := len(a), len(b)
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	var ops []op
	i, j := la, lb
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			ops = append([]op{{' ', a[i-1]}}, ops...)
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			ops = append([]op{{'+', b[j-1]}}, ops...)
			j--
		case i > 0:
			ops = append([]op{{'-', a[i-1]}}, ops...)
			i--
		}
	}
	return ops
}

func lcsOps(a, b []string) []string {
	raw := lcsOpsRaw(a, b)
	out := make([]string, 0, len(raw))
	for _, o := range raw {
		out = append(out, string(o.kind)+o.line)
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func init() { command.RegisterBuiltin(New()) }
