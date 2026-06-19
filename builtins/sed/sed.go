// Package sed implements the `sed` built-in.
//
// Implements GNU-ish sed with address forms (line numbers, $, /regex/,
// ranges N,M, step N~step), commands s/g/i/p/N, d, p, q, n, N, a\, i\,
// c\, y///, =, labels :LBL, b LBL, t LBL, T LBL. Extended regex via
// -E/-r. Programs may come from a positional argument, -e SCRIPT
// (repeatable), or -f FILE (repeatable).
//
// Honors Context.Limits.MaxSedIterations as a per-cycle counter:
// every iteration through the program (including label branches) ticks
// once; exceeding the cap aborts with exit 2.
package sed

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	stdstrings "strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "sed [-nE] [-e SCRIPT | -f FILE]... [SCRIPT] [FILE...]"
const helpText = `Usage: sed [OPTION]... {script-only-if-no-other-script} [input-file]...

  -n, --quiet, --silent  suppress automatic printing
  -e SCRIPT              add script to commands (may repeat)
  -f FILE                add file contents to commands (may repeat)
  -E, -r, --regexp-extended  use extended regular expressions
  -i (in-place)          NOT SUPPORTED in sandbox
  --help                 show this help`

// New returns the sed command.
func New() command.Command { return command.Define("sed", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	noAutoPrint := false
	extended := false
	var scripts []string
	scriptSet := false
	var files []string

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-n", a == "--quiet", a == "--silent":
			noAutoPrint = true
		case a == "-E", a == "-r", a == "--regexp-extended":
			extended = true
		case a == "-e":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			scripts = append(scripts, args[i])
			scriptSet = true
		case stdstrings.HasPrefix(a, "-e") && len(a) > 2:
			scripts = append(scripts, a[2:])
			scriptSet = true
		case a == "-f":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			data, err := readVFS(c, args[i])
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "sed", 2, "%s: %v", args[i], err)
			}
			scripts = append(scripts, string(data))
			scriptSet = true
		case a == "-i", stdstrings.HasPrefix(a, "-i"):
			return builtinutil.Errorf(c.Stderr, "sed", 2, "-i not supported in sandbox")
		case a == "--":
			i++
			if !scriptSet && i < len(args) {
				scripts = append(scripts, args[i])
				scriptSet = true
				i++
			}
			files = append(files, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			// Bundled short flags.
			ok := true
			for _, ch := range a[1:] {
				switch ch {
				case 'n':
					noAutoPrint = true
				case 'E', 'r':
					extended = true
				default:
					ok = false
				}
			}
			if !ok {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			if !scriptSet {
				scripts = append(scripts, a)
				scriptSet = true
			} else {
				files = append(files, a)
			}
		}
	}
run:
	if !scriptSet {
		return builtinutil.UsageError(c.Stderr, usage)
	}

	prog, err := compile(stdstrings.Join(scripts, "\n"), extended)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "sed", 2, "%v", err)
	}

	// Read all input (sed needs '$' last-line address).
	if len(files) == 0 {
		files = []string{"-"}
	}
	var inBuf bytes.Buffer
	for _, f := range files {
		data, err := readVFS(c, f)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "sed", 2, "%s: %v", f, err)
		}
		inBuf.Write(data)
	}
	lines := splitLines(inBuf.Bytes())

	st := &state{
		prog:        prog,
		lines:       lines,
		out:         c.Stdout,
		noAutoPrint: noAutoPrint,
		maxIter:     c.Limits.MaxSedIterations,
	}
	if err := st.run(); err != nil {
		return builtinutil.Errorf(c.Stderr, "sed", 2, "%v", err)
	}
	if err := st.flushAppend(); err != nil {
		return builtinutil.Errorf(c.Stderr, "sed", 2, "%v", err)
	}
	return command.Result{ExitCode: 0}
}

// ---- Program ----

type program struct {
	cmds   []*cmd
	labels map[string]int // index into cmds
}

type cmdKind int

const (
	cSubst cmdKind = iota
	cDelete
	cPrint
	cQuit
	cNext
	cBigNext
	cAppend
	cInsert
	cChange
	cYank // y///
	cLine // =
	cLabel
	cBranch
	cBranchTrue
	cBranchFalse
	cBlockStart
	cBlockEnd
)

type address struct {
	// kind: lineNum / dollar / regex / none
	kind addrKind
	num  int64
	re   *regexp.Regexp
}

type addrKind int

const (
	aNone addrKind = iota
	aLine
	aDollar
	aRegex
)

type addrSpec struct {
	a1, a2 *address // a2 == nil → single address
	step   int      // for "N~step" form (a1.kind=aLine, step>0)
	negate bool
}

type cmd struct {
	kind cmdKind
	addr addrSpec
	// substitute
	subRe   *regexp.Regexp
	subRepl string
	subFlagG bool
	subFlagP bool
	subFlagI bool
	subFlagN int // 0 = none; replaces Nth match
	// append/insert/change
	text string
	// yank
	yFrom string
	yTo   string
	// label/branch
	label string
	// block end index (for braces)
	blockEnd int
}

// ---- Compiler ----

type compiler struct {
	src      string
	pos      int
	extended bool
	prog     *program
}

func compile(src string, extended bool) (*program, error) {
	c := &compiler{src: src, extended: extended, prog: &program{labels: map[string]int{}}}
	if err := c.parseProgram(0); err != nil {
		return nil, err
	}
	// Resolve labels referenced by branches.
	for _, cm := range c.prog.cmds {
		if cm.kind == cBranch || cm.kind == cBranchTrue || cm.kind == cBranchFalse {
			if cm.label == "" {
				continue
			}
			if _, ok := c.prog.labels[cm.label]; !ok {
				return nil, fmt.Errorf("undefined label %q", cm.label)
			}
		}
	}
	return c.prog, nil
}

func (c *compiler) parseProgram(blockDepth int) error {
	for c.pos < len(c.src) {
		c.skipWS()
		if c.pos >= len(c.src) {
			return nil
		}
		ch := c.src[c.pos]
		if ch == '}' {
			if blockDepth == 0 {
				return fmt.Errorf("unmatched }")
			}
			return nil
		}
		if ch == '#' {
			c.skipLine()
			continue
		}
		if ch == ';' || ch == '\n' {
			c.pos++
			continue
		}
		// Address(es).
		var addr addrSpec
		var err error
		addr, err = c.parseAddrSpec()
		if err != nil {
			return err
		}
		c.skipWS()
		if c.pos >= len(c.src) {
			return fmt.Errorf("missing command after address")
		}
		op := c.src[c.pos]
		if op == '!' {
			addr.negate = true
			c.pos++
			c.skipWS()
			if c.pos >= len(c.src) {
				return fmt.Errorf("missing command after '!'")
			}
			op = c.src[c.pos]
		}
		switch op {
		case '{':
			c.pos++
			start := &cmd{kind: cBlockStart, addr: addr}
			c.prog.cmds = append(c.prog.cmds, start)
			startIdx := len(c.prog.cmds) - 1
			if err := c.parseProgram(blockDepth + 1); err != nil {
				return err
			}
			if c.pos >= len(c.src) || c.src[c.pos] != '}' {
				return fmt.Errorf("missing }")
			}
			c.pos++
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cBlockEnd})
			endIdx := len(c.prog.cmds) - 1
			c.prog.cmds[startIdx].blockEnd = endIdx
		case 's':
			c.pos++
			cm, err := c.parseSubst()
			if err != nil {
				return err
			}
			cm.addr = addr
			c.prog.cmds = append(c.prog.cmds, cm)
		case 'd':
			c.pos++
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cDelete, addr: addr})
		case 'p':
			c.pos++
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cPrint, addr: addr})
		case 'q':
			c.pos++
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cQuit, addr: addr})
		case 'n':
			c.pos++
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cNext, addr: addr})
		case 'N':
			c.pos++
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cBigNext, addr: addr})
		case 'a':
			c.pos++
			text, err := c.parseATText()
			if err != nil {
				return err
			}
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cAppend, addr: addr, text: text})
		case 'i':
			c.pos++
			text, err := c.parseATText()
			if err != nil {
				return err
			}
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cInsert, addr: addr, text: text})
		case 'c':
			c.pos++
			text, err := c.parseATText()
			if err != nil {
				return err
			}
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cChange, addr: addr, text: text})
		case 'y':
			c.pos++
			cm, err := c.parseYank()
			if err != nil {
				return err
			}
			cm.addr = addr
			c.prog.cmds = append(c.prog.cmds, cm)
		case '=':
			c.pos++
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cLine, addr: addr})
		case ':':
			c.pos++
			name := c.readWord()
			c.prog.labels[name] = len(c.prog.cmds)
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cLabel, label: name})
		case 'b':
			c.pos++
			c.skipWS()
			name := c.readWord()
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cBranch, addr: addr, label: name})
		case 't':
			c.pos++
			c.skipWS()
			name := c.readWord()
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cBranchTrue, addr: addr, label: name})
		case 'T':
			c.pos++
			c.skipWS()
			name := c.readWord()
			c.prog.cmds = append(c.prog.cmds, &cmd{kind: cBranchFalse, addr: addr, label: name})
		default:
			return fmt.Errorf("unknown command %q", string(op))
		}
		// Allow a stmt terminator: ';' or newline or end.
		c.skipWS()
		if c.pos < len(c.src) {
			switch c.src[c.pos] {
			case ';', '\n':
				c.pos++
			case '}':
				// handled by outer loop
			}
		}
	}
	return nil
}

func (c *compiler) skipWS() {
	for c.pos < len(c.src) {
		switch c.src[c.pos] {
		case ' ', '\t':
			c.pos++
		default:
			return
		}
	}
}

func (c *compiler) skipLine() {
	for c.pos < len(c.src) && c.src[c.pos] != '\n' {
		c.pos++
	}
}

func (c *compiler) readWord() string {
	var b stdstrings.Builder
	for c.pos < len(c.src) {
		ch := c.src[c.pos]
		if ch == ';' || ch == '\n' || ch == ' ' || ch == '\t' || ch == '}' {
			break
		}
		b.WriteByte(ch)
		c.pos++
	}
	return b.String()
}

func (c *compiler) parseAddrSpec() (addrSpec, error) {
	var spec addrSpec
	a1, ok, err := c.parseAddress()
	if err != nil {
		return spec, err
	}
	if !ok {
		return spec, nil
	}
	spec.a1 = a1
	if c.pos < len(c.src) && c.src[c.pos] == ',' {
		c.pos++
		a2, ok2, err := c.parseAddress()
		if err != nil {
			return spec, err
		}
		if !ok2 {
			return spec, fmt.Errorf("missing second address")
		}
		spec.a2 = a2
	} else if c.pos < len(c.src) && c.src[c.pos] == '~' && a1.kind == aLine {
		c.pos++
		n := c.readNumber()
		spec.step = int(n)
	}
	return spec, nil
}

func (c *compiler) parseAddress() (*address, bool, error) {
	if c.pos >= len(c.src) {
		return nil, false, nil
	}
	ch := c.src[c.pos]
	switch {
	case ch >= '0' && ch <= '9':
		n := c.readNumber()
		return &address{kind: aLine, num: n}, true, nil
	case ch == '$':
		c.pos++
		return &address{kind: aDollar}, true, nil
	case ch == '/':
		c.pos++
		pat, err := c.readUntil('/')
		if err != nil {
			return nil, false, err
		}
		re, err := c.compileRegex(pat)
		if err != nil {
			return nil, false, err
		}
		return &address{kind: aRegex, re: re}, true, nil
	}
	return nil, false, nil
}

func (c *compiler) readNumber() int64 {
	start := c.pos
	for c.pos < len(c.src) && c.src[c.pos] >= '0' && c.src[c.pos] <= '9' {
		c.pos++
	}
	n, _ := strconv.ParseInt(c.src[start:c.pos], 10, 64)
	return n
}

func (c *compiler) readUntil(delim byte) (string, error) {
	var b stdstrings.Builder
	for c.pos < len(c.src) {
		ch := c.src[c.pos]
		if ch == '\\' && c.pos+1 < len(c.src) {
			next := c.src[c.pos+1]
			if next == delim {
				b.WriteByte(delim)
				c.pos += 2
				continue
			}
			b.WriteByte(ch)
			b.WriteByte(next)
			c.pos += 2
			continue
		}
		if ch == delim {
			c.pos++
			return b.String(), nil
		}
		if ch == '\n' {
			return "", fmt.Errorf("unterminated regex")
		}
		b.WriteByte(ch)
		c.pos++
	}
	return "", fmt.Errorf("unterminated regex")
}

func (c *compiler) compileRegex(pat string) (*regexp.Regexp, error) {
	if !c.extended {
		// Convert BRE-ish to RE2: just compile as-is. BRE differences
		// (`\(`/`\)` vs `(`/`)`, etc.) are not strictly emulated.
		pat = breToErb(pat)
	}
	return regexp.Compile(pat)
}

// breToErb converts a few BRE escapes to RE2 syntax. This is a
// best-effort shim — full BRE→ERE conversion is out of scope.
func breToErb(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case '(', ')', '|', '{', '}', '+', '?':
				out = append(out, next)
				i++
				continue
			}
		}
		switch ch {
		case '(', ')', '{', '}', '+', '?':
			// In BRE these are literals.
			out = append(out, '\\', ch)
		default:
			out = append(out, ch)
		}
	}
	return string(out)
}

func (c *compiler) parseSubst() (*cmd, error) {
	if c.pos >= len(c.src) {
		return nil, fmt.Errorf("missing delimiter for s")
	}
	delim := c.src[c.pos]
	c.pos++
	pat, err := c.readUntil(delim)
	if err != nil {
		return nil, err
	}
	repl, err := c.readUntil(delim)
	if err != nil {
		return nil, err
	}
	cm := &cmd{kind: cSubst, subRepl: repl}
	flagI := false
	// Read flags until newline / ; / } / end.
	for c.pos < len(c.src) {
		ch := c.src[c.pos]
		if ch == ';' || ch == '\n' || ch == '}' {
			break
		}
		switch {
		case ch == 'g':
			cm.subFlagG = true
		case ch == 'p':
			cm.subFlagP = true
		case ch == 'i', ch == 'I':
			flagI = true
		case ch >= '0' && ch <= '9':
			n := c.readNumber()
			cm.subFlagN = int(n)
			continue
		case ch == ' ' || ch == '\t':
			// skip
		default:
			return nil, fmt.Errorf("unknown s flag %q", string(ch))
		}
		c.pos++
	}
	cm.subFlagI = flagI
	cflags := ""
	if flagI {
		cflags = "(?i)"
	}
	if !c.extended {
		pat = breToErb(pat)
	}
	re, err := regexp.Compile(cflags + pat)
	if err != nil {
		return nil, err
	}
	cm.subRe = re
	return cm, nil
}

func (c *compiler) parseYank() (*cmd, error) {
	if c.pos >= len(c.src) {
		return nil, fmt.Errorf("missing delimiter for y")
	}
	delim := c.src[c.pos]
	c.pos++
	from, err := c.readUntil(delim)
	if err != nil {
		return nil, err
	}
	to, err := c.readUntil(delim)
	if err != nil {
		return nil, err
	}
	if len([]rune(from)) != len([]rune(to)) {
		return nil, fmt.Errorf("y source and dest length differ")
	}
	return &cmd{kind: cYank, yFrom: from, yTo: to}, nil
}

// parseATText reads text after a, i, c. Forms:
//   a\<newline>TEXT (terminated by newline-not-preceded-by-backslash)
//   a TEXT (rest of line)
//   a\TEXT (rest of line)
func (c *compiler) parseATText() (string, error) {
	if c.pos < len(c.src) && c.src[c.pos] == '\\' {
		c.pos++
		// Optional newline.
		if c.pos < len(c.src) && c.src[c.pos] == '\n' {
			c.pos++
		}
	} else if c.pos < len(c.src) && c.src[c.pos] == ' ' {
		c.pos++
	}
	var b stdstrings.Builder
	for c.pos < len(c.src) {
		ch := c.src[c.pos]
		if ch == '\\' && c.pos+1 < len(c.src) && c.src[c.pos+1] == '\n' {
			b.WriteByte('\n')
			c.pos += 2
			continue
		}
		if ch == '\n' {
			break
		}
		b.WriteByte(ch)
		c.pos++
	}
	return b.String(), nil
}

// ---- Executor ----

type state struct {
	prog        *program
	lines       []string
	out         io.Writer
	noAutoPrint bool
	maxIter     int

	// per-cycle state
	pat       string
	curLine   int  // 1-based current input line
	lastLine  int  // total line count
	lastSub   bool // most recent s/// matched (for t/T)
	quit      bool
	deleted   bool
	skipPrint bool
	// pending append/insert text appended after the line prints.
	appendBuf []string
	// active range tracker: addrSpec key (index in cmds) → in-range flag
	rangeActive map[int]bool
	// per-cycle counter for MaxSedIterations.
	cycleTicks int
}

func (s *state) run() error {
	s.lastLine = len(s.lines)
	s.rangeActive = map[int]bool{}
	for s.curLine = 1; s.curLine <= s.lastLine && !s.quit; s.curLine++ {
		s.pat = s.lines[s.curLine-1]
		s.deleted = false
		s.skipPrint = false
		s.lastSub = false
		s.cycleTicks = 0
		if err := s.runCycle(0); err != nil {
			return err
		}
		if !s.noAutoPrint && !s.deleted && !s.skipPrint {
			s.writeLine(s.pat)
		}
		for _, t := range s.appendBuf {
			s.writeLine(t)
		}
		s.appendBuf = nil
	}
	return nil
}

func (s *state) flushAppend() error {
	for _, t := range s.appendBuf {
		s.writeLine(t)
	}
	s.appendBuf = nil
	return nil
}

func (s *state) writeLine(text string) {
	_, _ = io.WriteString(s.out, text)
	_, _ = io.WriteString(s.out, "\n")
}

func (s *state) runCycle(startPC int) error {
	pc := startPC
	for pc < len(s.prog.cmds) {
		s.cycleTicks++
		if s.maxIter > 0 && s.cycleTicks > s.maxIter {
			return fmt.Errorf("MaxSedIterations exceeded")
		}
		cm := s.prog.cmds[pc]
		match := s.addressMatch(cm, pc)
		if !match {
			if cm.kind == cBlockStart {
				pc = cm.blockEnd + 1
				continue
			}
			pc++
			continue
		}
		switch cm.kind {
		case cBlockStart, cBlockEnd, cLabel:
			pc++
		case cSubst:
			s.doSubst(cm)
			pc++
		case cDelete:
			s.deleted = true
			s.skipPrint = true
			return nil
		case cPrint:
			s.writeLine(s.pat)
			pc++
		case cQuit:
			s.quit = true
			return nil
		case cNext:
			if !s.noAutoPrint {
				s.writeLine(s.pat)
			}
			if s.curLine >= s.lastLine {
				s.quit = true
				s.skipPrint = true
				return nil
			}
			s.curLine++
			s.pat = s.lines[s.curLine-1]
			pc++
		case cBigNext:
			if s.curLine >= s.lastLine {
				s.quit = true
				if !s.noAutoPrint {
					s.writeLine(s.pat)
				}
				s.skipPrint = true
				return nil
			}
			s.curLine++
			s.pat = s.pat + "\n" + s.lines[s.curLine-1]
			pc++
		case cAppend:
			s.appendBuf = append(s.appendBuf, cm.text)
			pc++
		case cInsert:
			s.writeLine(cm.text)
			pc++
		case cChange:
			s.writeLine(cm.text)
			s.deleted = true
			s.skipPrint = true
			return nil
		case cYank:
			s.pat = yankApply(s.pat, cm.yFrom, cm.yTo)
			pc++
		case cLine:
			s.writeLine(strconv.Itoa(s.curLine))
			pc++
		case cBranch:
			if cm.label == "" {
				return nil // branch to end of script for this cycle
			}
			pc = s.prog.labels[cm.label] + 1
		case cBranchTrue:
			if s.lastSub {
				s.lastSub = false
				if cm.label == "" {
					return nil
				}
				pc = s.prog.labels[cm.label] + 1
			} else {
				pc++
			}
		case cBranchFalse:
			if !s.lastSub {
				if cm.label == "" {
					return nil
				}
				pc = s.prog.labels[cm.label] + 1
			} else {
				s.lastSub = false
				pc++
			}
		default:
			pc++
		}
	}
	return nil
}

func (s *state) addressMatch(cm *cmd, pc int) bool {
	spec := cm.addr
	if spec.a1 == nil {
		return true
	}
	matched := false
	if spec.a2 != nil {
		// Range.
		active := s.rangeActive[pc]
		if !active {
			if addrHit(spec.a1, s) {
				active = true
				s.rangeActive[pc] = true
			}
		}
		if active {
			matched = true
			// Check end address.
			if addrEndHit(spec.a2, s, spec.a1) {
				s.rangeActive[pc] = false
			}
		}
	} else if spec.step > 0 {
		// N~step
		first := spec.a1.num
		step := int64(spec.step)
		if s.curLine == 0 {
			matched = false
		} else if int64(s.curLine) >= first {
			if step == 0 {
				matched = int64(s.curLine) == first
			} else {
				matched = (int64(s.curLine)-first)%step == 0
			}
		}
	} else {
		matched = addrHit(spec.a1, s)
	}
	if spec.negate {
		matched = !matched
	}
	return matched
}

func addrHit(a *address, s *state) bool {
	switch a.kind {
	case aLine:
		return int64(s.curLine) == a.num
	case aDollar:
		return s.curLine == s.lastLine
	case aRegex:
		return a.re.MatchString(s.pat)
	}
	return false
}

func addrEndHit(a2 *address, s *state, a1 *address) bool {
	switch a2.kind {
	case aLine:
		if int64(s.curLine) >= a2.num {
			return true
		}
	case aDollar:
		if s.curLine == s.lastLine {
			return true
		}
	case aRegex:
		// (Range end on regex matches when a2.re matches the current
		// pattern. Note: GNU sed allows the same-line case when a1==a2;
		// here we accept that, since rangeActive logic ends the range
		// only after we've matched in addressMatch().)
		_ = a1
		if a2.re.MatchString(s.pat) {
			return true
		}
	}
	return false
}

func (s *state) doSubst(cm *cmd) {
	re := cm.subRe
	repl := cm.subRepl
	if cm.subFlagG && cm.subFlagN == 0 {
		newStr, n := substAll(s.pat, re, repl)
		if n > 0 {
			s.pat = newStr
			s.lastSub = true
			if cm.subFlagP {
				s.writeLine(s.pat)
			}
		}
		return
	}
	if cm.subFlagN > 0 {
		newStr, ok := substNth(s.pat, re, repl, cm.subFlagN, cm.subFlagG)
		if ok {
			s.pat = newStr
			s.lastSub = true
			if cm.subFlagP {
				s.writeLine(s.pat)
			}
		}
		return
	}
	// Single replacement.
	loc := re.FindStringSubmatchIndex(s.pat)
	if loc == nil {
		return
	}
	newPat := s.pat[:loc[0]] + expandRepl(repl, s.pat, loc) + s.pat[loc[1]:]
	s.pat = newPat
	s.lastSub = true
	if cm.subFlagP {
		s.writeLine(s.pat)
	}
}

func substAll(src string, re *regexp.Regexp, repl string) (string, int) {
	matches := re.FindAllStringSubmatchIndex(src, -1)
	if matches == nil {
		return src, 0
	}
	var b stdstrings.Builder
	prev := 0
	for _, m := range matches {
		b.WriteString(src[prev:m[0]])
		b.WriteString(expandRepl(repl, src, m))
		prev = m[1]
	}
	b.WriteString(src[prev:])
	return b.String(), len(matches)
}

func substNth(src string, re *regexp.Regexp, repl string, nth int, alsoG bool) (string, bool) {
	matches := re.FindAllStringSubmatchIndex(src, -1)
	if len(matches) < nth {
		return src, false
	}
	var b stdstrings.Builder
	prev := 0
	hit := false
	for i, m := range matches {
		if i+1 < nth {
			b.WriteString(src[prev:m[1]])
			prev = m[1]
			continue
		}
		// i+1 >= nth → replace
		if !alsoG && i+1 > nth {
			break
		}
		b.WriteString(src[prev:m[0]])
		b.WriteString(expandRepl(repl, src, m))
		prev = m[1]
		hit = true
	}
	b.WriteString(src[prev:])
	return b.String(), hit
}

// expandRepl honors sed replacement syntax: \1..\9 (capture groups),
// & (whole match), \& (literal &), \\ (literal backslash), \n (newline).
func expandRepl(repl, src string, loc []int) string {
	var b stdstrings.Builder
	for i := 0; i < len(repl); i++ {
		ch := repl[i]
		switch ch {
		case '\\':
			if i+1 < len(repl) {
				next := repl[i+1]
				if next >= '0' && next <= '9' {
					idx := int(next-'0') * 2
					if idx+1 < len(loc) && loc[idx] >= 0 {
						b.WriteString(src[loc[idx]:loc[idx+1]])
					}
					i++
					continue
				}
				switch next {
				case '&':
					b.WriteByte('&')
				case '\\':
					b.WriteByte('\\')
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(next)
				}
				i++
				continue
			}
		case '&':
			b.WriteString(src[loc[0]:loc[1]])
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func yankApply(s, from, to string) string {
	fr := []rune(from)
	tr := []rune(to)
	m := make(map[rune]rune, len(fr))
	for i, r := range fr {
		m[r] = tr[i]
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if rep, ok := m[r]; ok {
			out = append(out, rep)
		} else {
			out = append(out, r)
		}
	}
	return string(out)
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

func readVFS(c *command.Context, name string) ([]byte, error) {
	r, closer, err := builtinutil.OpenInput(c, name)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}
	return io.ReadAll(r)
}

// Used to silence linter on label-table ordering during debug.
var _ = sort.Strings

func init() { command.RegisterBuiltin(New()) }
