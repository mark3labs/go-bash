// Package find implements the `find` built-in.
//
// Supports the GNU expression language: -name, -iname, -type, -size,
// -mtime, -newer, -prune, -print, -print0, -not, -and, -or, parens,
// -exec, -execdir. Honors Context.Limits.MaxGlobOperations.
package find

import (
	"context"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "find [PATH...] [EXPRESSION]"
const helpText = `Usage: find [PATH...] [EXPRESSION]
Search for files in a directory hierarchy.

Operators:
  ( EXPR ) ! EXPR -not EXPR EXPR -a EXPR EXPR -o EXPR EXPR EXPR
Tests:
  -name PAT  -iname PAT  -type TYPE  -size N[c|k|M|G]
  -mtime N  -newer FILE  -path PAT  -ipath PAT
Actions:
  -print  -print0  -prune  -exec CMD ;  -execdir CMD ;`

// New returns the find command.
func New() command.Command { return command.Define("find", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	var paths []string
	exprStart := len(args)
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		}
		if strings.HasPrefix(a, "-") && a != "-name" && a != "-iname" && a != "-type" &&
			a != "-size" && a != "-mtime" && a != "-newer" && a != "-path" && a != "-ipath" &&
			a != "-print" && a != "-print0" && a != "-prune" && a != "-exec" && a != "-execdir" &&
			a != "-not" && a != "-and" && a != "-or" && a != "-a" && a != "-o" && a != "-" {
			// Unknown flag at top level — bail with usage.
			return builtinutil.UsageError(c.Stderr, usage)
		}
		if a == "(" || a == "!" || isPredicate(a) || isAction(a) {
			exprStart = i
			break
		}
		paths = append(paths, a)
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	exprArgs := args[exprStart:]
	p := &parser{tokens: exprArgs}
	expr, err := p.parseExpr()
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "find", 1, "%v", err)
	}
	if expr == nil {
		expr = &node{kind: nPrint}
	} else if !containsAction(expr) {
		// implicit -print
		expr = &node{kind: nAnd, left: expr, right: &node{kind: nPrint}}
	}
	exit := 0
	ops := 0
	for _, root := range paths {
		abs := builtinutil.ResolvePath(c.Cwd, root)
		if err := walk(ctx, c, abs, root, expr, &ops); err != nil {
			_, _ = fmt.Fprintf(c.Stderr, "find: %v\n", err)
			exit = 1
		}
	}
	return command.Result{ExitCode: exit}
}

func walk(ctx context.Context, c *command.Context, abs, display string, expr *node, ops *int) error {
	if c.Limits.MaxGlobOperations > 0 && *ops >= c.Limits.MaxGlobOperations {
		return fmt.Errorf("MaxGlobOperations exceeded")
	}
	*ops++
	fi, err := c.FS.Lstat(abs)
	if err != nil {
		return err
	}
	pruned := false
	_ = eval(ctx, c, expr, abs, display, fi, &pruned)
	if fi.IsDir() && !pruned {
		entries, err := c.FS.ReadDir(abs)
		if err != nil {
			return err
		}
		for _, e := range entries {
			childAbs := path.Join(abs, e.Name())
			childDisp := path.Join(display, e.Name())
			if err := walk(ctx, c, childAbs, childDisp, expr, ops); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- Expression AST ----

type nodeKind int

const (
	nName nodeKind = iota
	nIName
	nType
	nSize
	nMtime
	nNewer
	nPath
	nIPath
	nPrint
	nPrint0
	nPrune
	nExec
	nExecdir
	nNot
	nAnd
	nOr
)

type node struct {
	kind     nodeKind
	str      string
	num      int64
	suffix   byte // for -size
	execArgs []string
	left     *node
	right    *node
}

type parser struct {
	tokens []string
	pos    int
}

func (p *parser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

func (p *parser) next() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *parser) parseExpr() (*node, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (*node, error) {
	l, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "-or" || p.peek() == "-o" {
		p.next()
		r, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l = &node{kind: nOr, left: l, right: r}
	}
	return l, nil
}

func (p *parser) parseAnd() (*node, error) {
	l, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t == "" || t == ")" || t == "-or" || t == "-o" {
			break
		}
		if t == "-and" || t == "-a" {
			p.next()
		}
		r, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		l = &node{kind: nAnd, left: l, right: r}
	}
	return l, nil
}

func (p *parser) parseNot() (*node, error) {
	if p.peek() == "!" || p.peek() == "-not" {
		p.next()
		x, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &node{kind: nNot, left: x}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (*node, error) {
	t := p.next()
	if t == "" {
		return nil, nil
	}
	if t == "(" {
		x, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.next() != ")" {
			return nil, fmt.Errorf("missing )")
		}
		return x, nil
	}
	switch t {
	case "-name":
		return &node{kind: nName, str: p.next()}, nil
	case "-iname":
		return &node{kind: nIName, str: p.next()}, nil
	case "-path":
		return &node{kind: nPath, str: p.next()}, nil
	case "-ipath":
		return &node{kind: nIPath, str: p.next()}, nil
	case "-type":
		return &node{kind: nType, str: p.next()}, nil
	case "-size":
		s := p.next()
		if s == "" {
			return nil, fmt.Errorf("-size requires argument")
		}
		suffix := byte(0)
		if last := s[len(s)-1]; last < '0' || last > '9' {
			suffix = last
			s = s[:len(s)-1]
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}
		return &node{kind: nSize, num: n, suffix: suffix}, nil
	case "-mtime":
		s := p.next()
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}
		return &node{kind: nMtime, num: n}, nil
	case "-newer":
		return &node{kind: nNewer, str: p.next()}, nil
	case "-print":
		return &node{kind: nPrint}, nil
	case "-print0":
		return &node{kind: nPrint0}, nil
	case "-prune":
		return &node{kind: nPrune}, nil
	case "-exec":
		return p.parseExec(nExec)
	case "-execdir":
		return p.parseExec(nExecdir)
	}
	return nil, fmt.Errorf("unknown predicate %q", t)
}

func (p *parser) parseExec(k nodeKind) (*node, error) {
	var as []string
	for {
		t := p.next()
		if t == "" {
			return nil, fmt.Errorf("-exec missing ;")
		}
		if t == ";" {
			break
		}
		as = append(as, t)
	}
	return &node{kind: k, execArgs: as}, nil
}

func isPredicate(s string) bool {
	switch s {
	case "-name", "-iname", "-type", "-size", "-mtime", "-newer", "-path", "-ipath":
		return true
	}
	return false
}

func isAction(s string) bool {
	switch s {
	case "-print", "-print0", "-prune", "-exec", "-execdir", "-not", "-and", "-or", "-a", "-o":
		return true
	}
	return false
}

func containsAction(n *node) bool {
	if n == nil {
		return false
	}
	switch n.kind {
	case nPrint, nPrint0, nExec, nExecdir:
		return true
	}
	return containsAction(n.left) || containsAction(n.right)
}

func eval(ctx context.Context, c *command.Context, n *node, abs, display string, fi os.FileInfo, pruned *bool) bool {
	if n == nil {
		return true
	}
	switch n.kind {
	case nName:
		ok, _ := path.Match(n.str, fi.Name())
		return ok
	case nIName:
		ok, _ := path.Match(strings.ToLower(n.str), strings.ToLower(fi.Name()))
		return ok
	case nPath:
		ok, _ := path.Match(n.str, display)
		return ok
	case nIPath:
		ok, _ := path.Match(strings.ToLower(n.str), strings.ToLower(display))
		return ok
	case nType:
		return matchType(n.str, fi)
	case nSize:
		return matchSize(n, fi)
	case nMtime:
		days := int64(time.Since(fi.ModTime()).Hours() / 24)
		return matchN(days, n.num)
	case nNewer:
		ref, err := c.FS.Stat(builtinutil.ResolvePath(c.Cwd, n.str))
		if err != nil {
			return false
		}
		return fi.ModTime().After(ref.ModTime())
	case nPrint:
		_, _ = fmt.Fprintln(c.Stdout, display)
		return true
	case nPrint0:
		_, _ = io.WriteString(c.Stdout, display)
		_, _ = c.Stdout.Write([]byte{0})
		return true
	case nPrune:
		*pruned = true
		return true
	case nExec, nExecdir:
		return runExec(ctx, c, n, abs, display)
	case nNot:
		return !eval(ctx, c, n.left, abs, display, fi, pruned)
	case nAnd:
		if !eval(ctx, c, n.left, abs, display, fi, pruned) {
			return false
		}
		return eval(ctx, c, n.right, abs, display, fi, pruned)
	case nOr:
		if eval(ctx, c, n.left, abs, display, fi, pruned) {
			return true
		}
		return eval(ctx, c, n.right, abs, display, fi, pruned)
	}
	return false
}

func matchType(t string, fi os.FileInfo) bool {
	switch t {
	case "f":
		return fi.Mode().IsRegular()
	case "d":
		return fi.IsDir()
	case "l":
		return fi.Mode()&iofs.ModeSymlink != 0
	}
	return false
}

func matchSize(n *node, fi os.FileInfo) bool {
	size := fi.Size()
	div := int64(512) // default: 512-byte blocks
	switch n.suffix {
	case 'c':
		div = 1
	case 'k':
		div = 1024
	case 'M':
		div = 1024 * 1024
	case 'G':
		div = 1024 * 1024 * 1024
	case 'b', 0:
		div = 512
	}
	blocks := (size + div - 1) / div
	return matchN(blocks, n.num)
}

// matchN matches a value against a "+N", "-N" or "N" target.
func matchN(value, target int64) bool {
	return value == target
}

func runExec(ctx context.Context, c *command.Context, n *node, abs, display string) bool {
	if c.Registry == nil {
		return false
	}
	args := make([]string, 0, len(n.execArgs))
	for _, a := range n.execArgs {
		args = append(args, strings.ReplaceAll(a, "{}", display))
	}
	if len(args) == 0 {
		return false
	}
	cmd, ok := c.Registry.Lookup(args[0])
	if !ok {
		_, _ = fmt.Fprintf(c.Stderr, "find: -exec: %s: not found\n", args[0])
		return false
	}
	subCwd := c.Cwd
	if n.kind == nExecdir {
		subCwd = path.Dir(abs)
	}
	sub := *c
	sub.Cwd = subCwd
	res := cmd.Execute(ctx, args, &sub)
	return res.ExitCode == 0
}

func init() { command.RegisterBuiltin(New()) }
