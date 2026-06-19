// Package test implements the `[` and `test` shell built-ins.
//
// `[ EXPR ]` — required closing `]`. `test EXPR` — no closing bracket.
// Supports the POSIX-shape tests: string (-z/-n/=/!=), integer (-eq,
// -ne, -lt, -le, -gt, -ge), file (-e, -f, -d, -r, -w, -x, -s), unary
// `!` negation, binary `-a` / `-o` (deprecated but ubiquitous), and
// parentheses.
//
// mvdan/sh ships its own; reachable via /bin/[ and /bin/test.
package test

import (
	"context"
	"fmt"
	iofs "io/fs"
	"path"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
)

// NewBracket returns the `[` command.
func NewBracket() command.Command { return command.Define("[", run) }

// NewTest returns the `test` command.
func NewTest() command.Command { return command.Define("test", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	name := args[0]
	rest := args[1:]
	if name == "[" {
		// Require trailing "]".
		if len(rest) == 0 || rest[len(rest)-1] != "]" {
			if c.Stderr != nil {
				_, _ = fmt.Fprintln(c.Stderr, "[: missing `]'")
			}
			return command.Result{ExitCode: 2}
		}
		rest = rest[:len(rest)-1]
	}
	if len(rest) >= 1 && rest[0] == "--help" {
		if c.Stdout != nil {
			_, _ = fmt.Fprintln(c.Stdout, "Usage: [ EXPR ]   |   test EXPR")
		}
		return command.Result{ExitCode: 0}
	}
	v, err := evalArgs(rest, c)
	if err != nil {
		if c.Stderr != nil {
			_, _ = fmt.Fprintf(c.Stderr, "%s: %v\n", name, err)
		}
		return command.Result{ExitCode: 2}
	}
	if v {
		return command.Result{ExitCode: 0}
	}
	return command.Result{ExitCode: 1}
}

// evalArgs evaluates a flat slice of test tokens via a simple
// precedence parser: ! > unary/binary > -a > -o.
func evalArgs(tokens []string, c *command.Context) (bool, error) {
	if len(tokens) == 0 {
		return false, nil
	}
	p := &tparser{toks: tokens, c: c}
	v, err := p.parseOr()
	if err != nil {
		return false, err
	}
	if p.pos != len(p.toks) {
		return false, fmt.Errorf("unexpected token %q", p.toks[p.pos])
	}
	return v, nil
}

type tparser struct {
	toks []string
	pos  int
	c    *command.Context
}

func (p *tparser) parseOr() (bool, error) {
	l, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for p.pos < len(p.toks) && p.toks[p.pos] == "-o" {
		p.pos++
		r, err := p.parseAnd()
		if err != nil {
			return false, err
		}
		l = l || r
	}
	return l, nil
}

func (p *tparser) parseAnd() (bool, error) {
	l, err := p.parseNot()
	if err != nil {
		return false, err
	}
	for p.pos < len(p.toks) && p.toks[p.pos] == "-a" {
		p.pos++
		r, err := p.parseNot()
		if err != nil {
			return false, err
		}
		l = l && r
	}
	return l, nil
}

func (p *tparser) parseNot() (bool, error) {
	if p.pos < len(p.toks) && p.toks[p.pos] == "!" {
		p.pos++
		v, err := p.parseNot()
		return !v, err
	}
	return p.parsePrimary()
}

func (p *tparser) parsePrimary() (bool, error) {
	if p.pos >= len(p.toks) {
		return false, fmt.Errorf("missing operand")
	}
	if p.toks[p.pos] == "(" {
		p.pos++
		v, err := p.parseOr()
		if err != nil {
			return false, err
		}
		if p.pos >= len(p.toks) || p.toks[p.pos] != ")" {
			return false, fmt.Errorf("missing )")
		}
		p.pos++
		return v, nil
	}
	remaining := len(p.toks) - p.pos
	if remaining >= 3 && isBinaryOp(p.toks[p.pos+1]) {
		l, op, r := p.toks[p.pos], p.toks[p.pos+1], p.toks[p.pos+2]
		p.pos += 3
		return evalBinary(l, op, r)
	}
	if remaining >= 2 && isUnaryOp(p.toks[p.pos]) {
		op, arg := p.toks[p.pos], p.toks[p.pos+1]
		p.pos += 2
		return evalUnary(op, arg, p.c)
	}
	// Single arg: true iff non-empty.
	tok := p.toks[p.pos]
	p.pos++
	return tok != "", nil
}

func isUnaryOp(s string) bool {
	switch s {
	case "-e", "-f", "-d", "-r", "-w", "-x", "-s", "-z", "-n", "-h", "-L", "-b", "-c", "-p", "-S":
		return true
	}
	return false
}

func isBinaryOp(s string) bool {
	switch s {
	case "=", "==", "!=", "-eq", "-ne", "-lt", "-le", "-gt", "-ge", "-ef", "-nt", "-ot":
		return true
	}
	return false
}

func evalUnary(op, arg string, c *command.Context) (bool, error) {
	switch op {
	case "-z":
		return arg == "", nil
	case "-n":
		return arg != "", nil
	}
	// File tests
	if c == nil || c.FS == nil {
		return false, nil
	}
	resolved := arg
	if !strings.HasPrefix(resolved, "/") && c.Cwd != "" {
		resolved = path.Join(c.Cwd, resolved)
	}
	info, err := c.FS.Stat(resolved)
	if err != nil {
		return false, nil
	}
	switch op {
	case "-e":
		return true, nil
	case "-f":
		return info.Mode().IsRegular(), nil
	case "-d":
		return info.IsDir(), nil
	case "-r", "-w", "-x":
		return true, nil // VFS lacks mode checks
	case "-s":
		return info.Size() > 0, nil
	case "-h", "-L":
		linfo, lerr := c.FS.Lstat(resolved)
		if lerr != nil {
			return false, nil
		}
		return linfo.Mode()&iofs.ModeSymlink != 0, nil
	case "-b", "-c", "-p", "-S":
		return false, nil
	}
	return false, nil
}

func evalBinary(l, op, r string) (bool, error) {
	switch op {
	case "=", "==":
		return l == r, nil
	case "!=":
		return l != r, nil
	case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		li, err := strconv.ParseInt(l, 10, 64)
		if err != nil {
			return false, fmt.Errorf("%s: integer expression expected", l)
		}
		ri, err := strconv.ParseInt(r, 10, 64)
		if err != nil {
			return false, fmt.Errorf("%s: integer expression expected", r)
		}
		switch op {
		case "-eq":
			return li == ri, nil
		case "-ne":
			return li != ri, nil
		case "-lt":
			return li < ri, nil
		case "-le":
			return li <= ri, nil
		case "-gt":
			return li > ri, nil
		case "-ge":
			return li >= ri, nil
		}
	}
	return false, fmt.Errorf("unknown op %s", op)
}

func init() {
	command.RegisterBuiltin(NewBracket())
	command.RegisterBuiltin(NewTest())
}
