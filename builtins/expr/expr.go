// Package expr implements the `expr` built-in (SPEC §10 Wave A).
//
// expr evaluates a single expression made of operators and operands.
// Operator precedence (lowest → highest):
//
//	|                logical or
//	&                logical and
//	= != < > <= >=   comparison
//	+ -              addition / subtraction
//	* / %            multiplication / division / modulo
//	: STRING         regex anchor-match (BRE), returns matched substring or length
//	( EXPR )         grouping
//
// String comparisons fall through to numeric when both sides parse as
// integers. expr exits with:
//
//	0 if the result is non-zero AND non-empty
//	1 if the result is 0 or empty
//	2 on syntax error (and writes "expr: ..." to stderr)
package expr

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const helpText = `Usage: expr EXPRESSION
Evaluate EXPRESSION and write the result to standard output.

Operators (lowest → highest precedence):
  ARG1 | ARG2     ARG1 if non-empty/non-zero, else ARG2
  ARG1 & ARG2     ARG1 if both non-empty/non-zero, else 0
  = != < <= > >=  comparison (numeric when both sides parse)
  + -             addition, subtraction
  * / %           multiplication, division, modulo
  STRING : REGEX  BRE anchored at start; returns match length or
                  first captured group
  ( EXPR )        grouping

Exit status:
  0  expression is non-empty and non-zero
  1  expression is empty or zero
  2  syntax error or invalid expression`

// New returns the expr command.
func New() command.Command {
	return command.Define("expr", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	if len(args) >= 2 && args[1] == "--help" {
		builtinutil.PrintHelp(c.Stdout, helpText)
		return command.Result{ExitCode: 0}
	}
	if len(args) < 2 {
		return builtinutil.Errorf(c.Stderr, "expr", 2, "missing operand")
	}
	tokens := args[1:]
	p := &parser{tokens: tokens}
	v, err := p.parseOr()
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "expr", 2, "%s", err.Error())
	}
	if !p.atEnd() {
		return builtinutil.Errorf(c.Stderr, "expr", 2, "syntax error: unexpected %q", p.peek())
	}
	str := v.String()
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, str)
		_, _ = fmt.Fprintln(c.Stdout)
	}
	if str == "" || str == "0" {
		return command.Result{ExitCode: 1}
	}
	return command.Result{ExitCode: 0}
}

// value carries an int (when both sides of an op are numeric) or a
// string. Most ops convert lazily — Int() returns (n, true) when the
// value parses as an integer.
type value struct {
	s string
}

func (v value) String() string { return v.s }
func (v value) Int() (int64, bool) {
	n, err := strconv.ParseInt(v.s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
func strVal(s string) value         { return value{s: s} }
func intVal(n int64) value          { return value{s: strconv.FormatInt(n, 10)} }
func (v value) IsTrue() bool       { return v.s != "" && v.s != "0" }

type parser struct {
	tokens []string
	pos    int
}

func (p *parser) atEnd() bool  { return p.pos >= len(p.tokens) }
func (p *parser) peek() string { if p.atEnd() { return "" }; return p.tokens[p.pos] }
func (p *parser) consume() string {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) parseOr() (value, error) {
	v, err := p.parseAnd()
	if err != nil {
		return v, err
	}
	for !p.atEnd() && p.peek() == "|" {
		p.consume()
		rhs, err := p.parseAnd()
		if err != nil {
			return v, err
		}
		if v.IsTrue() {
			// already true; ignore rhs
		} else {
			v = rhs
		}
	}
	return v, nil
}

func (p *parser) parseAnd() (value, error) {
	v, err := p.parseCompare()
	if err != nil {
		return v, err
	}
	for !p.atEnd() && p.peek() == "&" {
		p.consume()
		rhs, err := p.parseCompare()
		if err != nil {
			return v, err
		}
		if v.IsTrue() && rhs.IsTrue() {
			// keep v
		} else {
			v = intVal(0)
		}
	}
	return v, nil
}

func (p *parser) parseCompare() (value, error) {
	v, err := p.parseAdd()
	if err != nil {
		return v, err
	}
	for !p.atEnd() {
		op := p.peek()
		if op != "=" && op != "!=" && op != "<" && op != "<=" && op != ">" && op != ">=" {
			break
		}
		p.consume()
		rhs, err := p.parseAdd()
		if err != nil {
			return v, err
		}
		v = compare(op, v, rhs)
	}
	return v, nil
}

func (p *parser) parseAdd() (value, error) {
	v, err := p.parseMul()
	if err != nil {
		return v, err
	}
	for !p.atEnd() {
		op := p.peek()
		if op != "+" && op != "-" {
			break
		}
		p.consume()
		rhs, err := p.parseMul()
		if err != nil {
			return v, err
		}
		a, ok1 := v.Int()
		b, ok2 := rhs.Int()
		if !ok1 || !ok2 {
			return v, fmt.Errorf("non-integer argument")
		}
		if op == "+" {
			v = intVal(a + b)
		} else {
			v = intVal(a - b)
		}
	}
	return v, nil
}

func (p *parser) parseMul() (value, error) {
	v, err := p.parseMatch()
	if err != nil {
		return v, err
	}
	for !p.atEnd() {
		op := p.peek()
		if op != "*" && op != "/" && op != "%" {
			break
		}
		p.consume()
		rhs, err := p.parseMatch()
		if err != nil {
			return v, err
		}
		a, ok1 := v.Int()
		b, ok2 := rhs.Int()
		if !ok1 || !ok2 {
			return v, fmt.Errorf("non-integer argument")
		}
		switch op {
		case "*":
			v = intVal(a * b)
		case "/":
			if b == 0 {
				return v, fmt.Errorf("division by zero")
			}
			v = intVal(a / b)
		case "%":
			if b == 0 {
				return v, fmt.Errorf("division by zero")
			}
			v = intVal(a % b)
		}
	}
	return v, nil
}

func (p *parser) parseMatch() (value, error) {
	v, err := p.parsePrimary()
	if err != nil {
		return v, err
	}
	for !p.atEnd() && p.peek() == ":" {
		p.consume()
		rhs, err := p.parsePrimary()
		if err != nil {
			return v, err
		}
		v = match(v.String(), rhs.String())
	}
	return v, nil
}

func (p *parser) parsePrimary() (value, error) {
	if p.atEnd() {
		return value{}, fmt.Errorf("syntax error: missing argument")
	}
	t := p.consume()
	if t == "(" {
		v, err := p.parseOr()
		if err != nil {
			return v, err
		}
		if p.atEnd() || p.consume() != ")" {
			return v, fmt.Errorf("syntax error: missing ')'")
		}
		return v, nil
	}
	return strVal(t), nil
}

func compare(op string, a, b value) value {
	// Numeric when both parse.
	an, aOK := a.Int()
	bn, bOK := b.Int()
	var result bool
	if aOK && bOK {
		switch op {
		case "=":
			result = an == bn
		case "!=":
			result = an != bn
		case "<":
			result = an < bn
		case "<=":
			result = an <= bn
		case ">":
			result = an > bn
		case ">=":
			result = an >= bn
		}
	} else {
		switch op {
		case "=":
			result = a.s == b.s
		case "!=":
			result = a.s != b.s
		case "<":
			result = a.s < b.s
		case "<=":
			result = a.s <= b.s
		case ">":
			result = a.s > b.s
		case ">=":
			result = a.s >= b.s
		}
	}
	if result {
		return intVal(1)
	}
	return intVal(0)
}

// match implements the `:` BRE-anchor-match operator. Returns the
// first captured group when the pattern has one, else the length of
// the matched substring (0 if no match).
func match(s, pat string) value {
	// Translate BRE-ish patterns to Go RE2 syntax. expr's regex is
	// always anchored at the start, so prepend ^.
	re, err := regexp.Compile("^" + bre2re2(pat))
	if err != nil {
		return strVal("")
	}
	m := re.FindStringSubmatch(s)
	if m == nil {
		// no group ⇒ length 0; group ⇒ empty string
		if re.NumSubexp() > 0 {
			return strVal("")
		}
		return intVal(0)
	}
	if re.NumSubexp() > 0 {
		return strVal(m[1])
	}
	return intVal(int64(len(m[0])))
}

// bre2re2 does a minimal BRE → RE2 translation: `\(`/`\)` → `(`/`)`,
// `\{n,m\}` → `{n,m}`, `\|` → `|`. expr's regex grammar is BRE so
// these are the cases that actually differ. Plain RE2 metacharacters
// pass through.
func bre2re2(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' && i+1 < len(p) {
			switch p[i+1] {
			case '(', ')', '{', '}', '|', '+', '?':
				b.WriteByte(p[i+1])
				i++
				continue
			}
		}
		b.WriteByte(p[i])
	}
	return b.String()
}

func init() {
	command.RegisterBuiltin(New())
}
