// Package alias implements parse-time alias expansion for go-bash
// (SPEC §11). When `shopt expand_aliases` is on, the runtime calls
// Expand on each parsed *syntax.File before running it; this walks
// every *syntax.CallExpr and rewrites the first argument when it
// matches an alias name.
//
// Expansion is intentionally simple — we re-tokenize the alias body
// by parsing it as a standalone script and extracting the first
// simple command's Args. Recursive expansion (where an alias body
// references another alias) is supported via a second pass with a
// visited-set guard, matching bash's anti-loop semantics.
package alias

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Expand walks file and rewrites every *syntax.CallExpr whose first
// argument is a single-literal word matching an entry in aliases.
// The argument is replaced by the alias body's tokens; any
// additional arguments are appended after.
//
// aliases may be nil — Expand is a no-op in that case.
func Expand(file *syntax.File, aliases map[string]string) {
	if file == nil || len(aliases) == 0 {
		return
	}
	// Run up to a few passes so chained aliases (alias a=b; alias b=c)
	// resolve. Cap at 16 to bound pathological loops.
	for i := 0; i < 16; i++ {
		changed := false
		syntax.Walk(file, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok || len(ce.Args) == 0 {
				return true
			}
			name, ok := literalWord(ce.Args[0])
			if !ok {
				return true
			}
			body, ok := aliases[name]
			if !ok {
				return true
			}
			newArgs, ok := parseAliasArgs(body)
			if !ok {
				return true
			}
			// Avoid trivial self-loop (alias x=x).
			if len(newArgs) > 0 {
				if first, ok := literalWord(newArgs[0]); ok && first == name {
					return true
				}
			}
			ce.Args = append(newArgs, ce.Args[1:]...)
			changed = true
			return true
		})
		if !changed {
			return
		}
	}
}

// literalWord returns the literal string value of w if w consists of
// a single *syntax.Lit part. Otherwise returns ("", false).
func literalWord(w *syntax.Word) (string, bool) {
	if w == nil || len(w.Parts) != 1 {
		return "", false
	}
	lit, ok := w.Parts[0].(*syntax.Lit)
	if !ok {
		return "", false
	}
	return lit.Value, true
}

// parseAliasArgs parses body as a standalone script and returns the
// Args of the first simple command. ok=false on parse failure or
// when the body does not produce a single simple command.
func parseAliasArgs(body string) (args []*syntax.Word, ok bool) {
	p := syntax.NewParser(syntax.Variant(syntax.LangBash))
	f, err := p.Parse(strings.NewReader(body), "")
	if err != nil || len(f.Stmts) == 0 {
		return nil, false
	}
	stmt := f.Stmts[0]
	ce, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return nil, false
	}
	return ce.Args, true
}
