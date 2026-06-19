// Package transform exposes the Serialize entry point that renders an
// *ast.Script back to bash source. It is the inverse of parser.Parse.
//
// # Phase 4 scope
//
// Serialize prefers the Origin back-reference parsed scripts carry:
// when present, it hands the original *syntax.File to
// mvdan.cc/sh/v3/syntax.Printer for byte-faithful round-trip. This is
// the path the Phase 4 round-trip acceptance test exercises.
//
// For plugin-synthesized scripts (Origin == nil, arriving in Phase 13
// when the transform plugin pipeline lands), we fall back to
// astToFile — a best-effort inverse translator that reconstructs a
// *syntax.File from our typed AST and lets the printer fill in default
// positions. Phase 4 ships only the minimal set of node reconstructors
// needed to keep the surface honest; later phases extend coverage as
// new node types come online.
package transform

import (
	"errors"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/mark3labs/go-bash/ast"
)

// Serialize renders s back to a bash source string. It prefers the
// Origin pointer parsed scripts carry; falls back to astToFile for
// plugin-synthesized scripts.
func Serialize(s *ast.Script) (string, error) {
	if s == nil {
		return "", errors.New("transform: nil script")
	}
	if s.Origin != nil {
		var b strings.Builder
		if err := syntax.NewPrinter().Print(&b, s.Origin); err != nil {
			return "", fmt.Errorf("transform: print: %w", err)
		}
		return b.String(), nil
	}
	file, err := astToFile(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := syntax.NewPrinter().Print(&b, file); err != nil {
		return "", fmt.Errorf("transform: print: %w", err)
	}
	return b.String(), nil
}

// astToFile is the inverse translator from our typed AST back to
// mvdan/sh's *syntax.File. Phase 4 covers Script → Statement →
// Pipeline → SimpleCommand → Word(Literal|SingleQuoted|DoubleQuoted)
// with no positions populated — the printer falls back to defaults.
//
// Unknown node types return an error rather than silently dropping
// source so consumers can detect coverage gaps as Phase 13's transform
// pipeline lands.
func astToFile(s *ast.Script) (*syntax.File, error) {
	out := &syntax.File{}
	for _, st := range s.Statements {
		stmts, err := statementToSh(st)
		if err != nil {
			return nil, err
		}
		out.Stmts = append(out.Stmts, stmts...)
	}
	return out, nil
}

func statementToSh(s *ast.Statement) ([]*syntax.Stmt, error) {
	if len(s.Pipelines) == 0 {
		return nil, nil
	}
	first, err := pipelineToCmd(s.Pipelines[0])
	if err != nil {
		return nil, err
	}
	cur := first
	for i := 1; i < len(s.Pipelines); i++ {
		next, err := pipelineToCmd(s.Pipelines[i])
		if err != nil {
			return nil, err
		}
		var op syntax.BinCmdOperator
		switch s.Operators[i-1] {
		case "&&":
			op = syntax.AndStmt
		case "||":
			op = syntax.OrStmt
		default:
			return nil, fmt.Errorf("transform: unknown statement operator %q", s.Operators[i-1])
		}
		cur = &syntax.BinaryCmd{
			Op: op,
			X:  &syntax.Stmt{Cmd: cur},
			Y:  &syntax.Stmt{Cmd: next},
		}
	}
	stmt := &syntax.Stmt{Cmd: cur, Background: s.Background}
	if len(s.Pipelines) > 0 {
		stmt.Negated = s.Pipelines[0].Negated
	}
	return []*syntax.Stmt{stmt}, nil
}

func pipelineToCmd(p *ast.Pipeline) (syntax.Command, error) {
	if len(p.Commands) == 0 {
		return nil, errors.New("transform: empty pipeline")
	}
	first, err := commandToSh(p.Commands[0])
	if err != nil {
		return nil, err
	}
	if len(p.Commands) == 1 {
		return first, nil
	}
	cur := first
	for i := 1; i < len(p.Commands); i++ {
		next, err := commandToSh(p.Commands[i])
		if err != nil {
			return nil, err
		}
		op := syntax.Pipe
		if i-1 < len(p.PipeStderr) && p.PipeStderr[i-1] {
			op = syntax.PipeAll
		}
		cur = &syntax.BinaryCmd{
			Op: op,
			X:  &syntax.Stmt{Cmd: cur},
			Y:  &syntax.Stmt{Cmd: next},
		}
	}
	return cur, nil
}

func commandToSh(c ast.Command) (syntax.Command, error) {
	switch cmd := c.(type) {
	case *ast.SimpleCommand:
		return simpleToCall(cmd)
	}
	return nil, fmt.Errorf("transform: inverse translator does not yet support %T (lands in Phase 13)", c)
}

func simpleToCall(c *ast.SimpleCommand) (syntax.Command, error) {
	call := &syntax.CallExpr{}
	if c.Name != nil {
		w, err := wordToSh(c.Name)
		if err != nil {
			return nil, err
		}
		call.Args = append(call.Args, w)
	}
	for _, a := range c.Args {
		w, err := wordToSh(a)
		if err != nil {
			return nil, err
		}
		call.Args = append(call.Args, w)
	}
	return call, nil
}

func wordToSh(w *ast.Word) (*syntax.Word, error) {
	out := &syntax.Word{}
	for _, p := range w.Parts {
		converted, err := wordPartToSh(p)
		if err != nil {
			return nil, err
		}
		out.Parts = append(out.Parts, converted)
	}
	if len(out.Parts) == 0 {
		out.Parts = []syntax.WordPart{&syntax.Lit{Value: ""}}
	}
	return out, nil
}

func wordPartToSh(p ast.WordPart) (syntax.WordPart, error) {
	switch v := p.(type) {
	case *ast.Literal:
		return &syntax.Lit{Value: v.Value}, nil
	case *ast.SingleQuoted:
		return &syntax.SglQuoted{Value: v.Value}, nil
	case *ast.DoubleQuoted:
		dq := &syntax.DblQuoted{}
		for _, sub := range v.Parts {
			converted, err := wordPartToSh(sub)
			if err != nil {
				return nil, err
			}
			dq.Parts = append(dq.Parts, converted)
		}
		return dq, nil
	}
	return nil, fmt.Errorf("transform: inverse translator does not yet support %T (lands in Phase 13)", p)
}
