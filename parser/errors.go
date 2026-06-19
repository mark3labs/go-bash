// Package parser exposes the public Parse / ParseString entry points and
// the typed [ParseError] returned on failure. Implementation lives in
// parser.go and translate.go.
//
// ParseError is re-exported from the top-level gobash package as
// gobash.ParseError via a type alias so consumers can use either spelling
// interchangeably (see errors.go in the root package).
package parser

import "fmt"

// ParseError reports a syntactic parse failure produced by the parser
// front-end. Line and Col are 1-based when known and 0 when the parser
// did not supply a position.
type ParseError struct {
	Msg  string
	Line int
	Col  int
}

func (e *ParseError) Error() string {
	if e.Line == 0 && e.Col == 0 {
		return "parse error: " + e.Msg
	}
	return fmt.Sprintf("parse error at %d:%d: %s", e.Line, e.Col, e.Msg)
}
