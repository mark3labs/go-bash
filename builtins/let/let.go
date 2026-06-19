// Package let implements the `let` shell built-in (SPEC §11).
//
// `let EXPR...` evaluates each arithmetic expression. The exit code
// is 0 if the last result is non-zero, 1 if it is zero (matching bash).
//
// This is a self-contained mini arithmetic evaluator supporting
// integer arithmetic with +, -, *, /, %, parentheses, unary -, and
// simple assignments `name=expr`. Variables resolve through c.Env and
// assignments mutate c.Env. Expressions richer than this fall back
// to "0" and exit 1.
package let

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "let EXPRESSION..."

// New returns the let command.
func New() command.Command { return command.Define("let", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	if len(args) < 2 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	if args[1] == "--help" {
		builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
		return command.Result{ExitCode: 0}
	}
	var last int64
	for _, expr := range args[1:] {
		// Tolerate spaces around = (mvdan divergence C in DECISIONS.md)
		expr = strings.TrimSpace(expr)
		v, err := evalExpr(expr, c.Env)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "let", 1, "%s: %v", expr, err)
		}
		last = v
	}
	if last == 0 {
		return command.Result{ExitCode: 1}
	}
	return command.Result{ExitCode: 0}
}

// evalExpr evaluates one expression. Supports `name=expr` assignment.
func evalExpr(s string, env map[string]string) (int64, error) {
	// Look for top-level assignment `name=expr` (no other operators
	// containing '=' supported in this slim port).
	if eq := topLevelAssign(s); eq > 0 {
		name := strings.TrimSpace(s[:eq])
		body := strings.TrimSpace(s[eq+1:])
		v, err := parseAndEval(body, env)
		if err != nil {
			return 0, err
		}
		if env != nil {
			env[name] = fmt.Sprintf("%d", v)
		}
		return v, nil
	}
	return parseAndEval(s, env)
}

// topLevelAssign returns the byte index of an `=` that looks like a
// plain assignment, or -1. Disqualified: `==`, `<=`, `>=`, `!=`,
// `*=`, `+=`, `-=`, `/=`, `%=`.
func topLevelAssign(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '=':
			if depth != 0 {
				continue
			}
			if i+1 < len(s) && s[i+1] == '=' {
				return -1
			}
			if i > 0 {
				switch s[i-1] {
				case '<', '>', '!', '*', '+', '-', '/', '%':
					return -1
				}
			}
			return i
		}
	}
	return -1
}

// parseAndEval is a tiny recursive-descent integer expression parser.
type parser struct {
	src string
	pos int
	env map[string]string
}

func parseAndEval(s string, env map[string]string) (int64, error) {
	p := &parser{src: s, env: env}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.pos != len(p.src) {
		return 0, fmt.Errorf("unexpected token at %d", p.pos)
	}
	return v, nil
}

func (p *parser) skipSpaces() {
	for p.pos < len(p.src) && unicode.IsSpace(rune(p.src[p.pos])) {
		p.pos++
	}
}

func (p *parser) parseExpr() (int64, error) { return p.parseAdd() }

func (p *parser) parseAdd() (int64, error) {
	l, err := p.parseMul()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.src) {
			return l, nil
		}
		op := p.src[p.pos]
		if op != '+' && op != '-' {
			return l, nil
		}
		p.pos++
		r, err := p.parseMul()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			l += r
		} else {
			l -= r
		}
	}
}

func (p *parser) parseMul() (int64, error) {
	l, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.src) {
			return l, nil
		}
		op := p.src[p.pos]
		if op != '*' && op != '/' && op != '%' {
			return l, nil
		}
		p.pos++
		r, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			l *= r
		case '/':
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			l /= r
		case '%':
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			l %= r
		}
	}
}

func (p *parser) parseUnary() (int64, error) {
	p.skipSpaces()
	if p.pos < len(p.src) && p.src[p.pos] == '-' {
		p.pos++
		v, err := p.parseUnary()
		return -v, err
	}
	if p.pos < len(p.src) && p.src[p.pos] == '+' {
		p.pos++
		return p.parseUnary()
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (int64, error) {
	p.skipSpaces()
	if p.pos >= len(p.src) {
		return 0, fmt.Errorf("unexpected end")
	}
	if p.src[p.pos] == '(' {
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return 0, fmt.Errorf("missing )")
		}
		p.pos++
		return v, nil
	}
	// Number or identifier
	start := p.pos
	if isDigit(p.src[p.pos]) {
		for p.pos < len(p.src) && isDigit(p.src[p.pos]) {
			p.pos++
		}
		return strconv.ParseInt(p.src[start:p.pos], 10, 64)
	}
	if isIdentStart(p.src[p.pos]) {
		for p.pos < len(p.src) && isIdentPart(p.src[p.pos]) {
			p.pos++
		}
		name := p.src[start:p.pos]
		if p.env == nil {
			return 0, nil
		}
		v := p.env[name]
		if v == "" {
			return 0, nil
		}
		return strconv.ParseInt(v, 10, 64)
	}
	return 0, fmt.Errorf("unexpected char %q", p.src[p.pos])
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) }

func init() { command.RegisterBuiltin(New()) }
