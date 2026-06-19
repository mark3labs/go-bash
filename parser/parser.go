package parser

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/mark3labs/go-bash/ast"
)

// Parser-side hard limits enforced by Parse / ParseString. The values
// are law; do not adjust without updating
// The spec.
const (
	// MaxInputSize caps the byte length of a single Parse call.
	MaxInputSize = 1 << 20 // 1 MiB

	// MaxTokens caps the number of syntax-tree nodes the parser walks
	// after a successful parse. We count syntax.Walk visits.
	MaxTokens = 100000

	// MaxParserDepth caps how deep the translator may recurse while
	// walking nested commands / words / heredocs.
	MaxParserDepth = 200

	// MaxHeredocSize caps the body of any single here-document.
	// Mirrors the DefaultLimits().MaxHeredocSize used at runtime.
	MaxHeredocSize = 10 * 1024 * 1024
)

// Parse parses src into a typed *ast.Script, enforcing the parser-side
// hard limits in this package. Returns *ParseError on failure.
func Parse(src string) (*ast.Script, error) {
	if len(src) > MaxInputSize {
		return nil, &ParseError{
			Msg: fmt.Sprintf("input too large: %d bytes (max %d)", len(src), MaxInputSize),
		}
	}

	p := syntax.NewParser(syntax.Variant(syntax.LangBash), syntax.KeepComments(false))
	file, err := p.Parse(strings.NewReader(src), "")
	if err != nil {
		return nil, wrapSyntaxError(err)
	}

	tokens := 0
	overflow := false
	syntax.Walk(file, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		tokens++
		if tokens > MaxTokens {
			overflow = true
			return false
		}
		return true
	})
	if overflow {
		return nil, &ParseError{
			Msg: fmt.Sprintf("too many tokens: > %d", MaxTokens),
		}
	}

	t := &translator{}
	script, err := t.script(file)
	if err != nil {
		return nil, err
	}
	return script, nil
}

// ParseString is an alias for Parse, mirroring src/parser/parser.ts.
func ParseString(src string) (*ast.Script, error) { return Parse(src) }

// wrapSyntaxError converts an mvdan/sh syntax.ParseError into our
// typed ParseError, preserving line/column when present.
func wrapSyntaxError(err error) *ParseError {
	if pe, ok := err.(syntax.ParseError); ok {
		return &ParseError{
			Msg:  pe.Text,
			Line: int(pe.Pos.Line()),
			Col:  int(pe.Pos.Col()),
		}
	}
	if pe, ok := err.(*syntax.ParseError); ok {
		return &ParseError{
			Msg:  pe.Text,
			Line: int(pe.Pos.Line()),
			Col:  int(pe.Pos.Col()),
		}
	}
	if le, ok := err.(syntax.LangError); ok {
		return &ParseError{
			Msg:  le.Error(),
			Line: int(le.Pos.Line()),
			Col:  int(le.Pos.Col()),
		}
	}
	return &ParseError{Msg: err.Error()}
}
