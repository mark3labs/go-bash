package transform

import "mvdan.cc/sh/v3/syntax"

// WordLit returns the literal text of w when every part of the word is
// statically known (plain literals, single-quoted, or double-quoted
// runs of literals). It returns "" for a nil word or as soon as it
// encounters any dynamic part (parameter/command/arithmetic
// expansion), since the literal value is then unknowable at
// transform time.
//
// Plugins use this to recover a command name or argument from a
// *syntax.Word without evaluating the script.
func WordLit(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	if lit := w.Lit(); lit != "" {
		return lit
	}
	var b []byte
	for _, p := range w.Parts {
		switch v := p.(type) {
		case *syntax.Lit:
			b = append(b, v.Value...)
		case *syntax.SglQuoted:
			b = append(b, v.Value...)
		case *syntax.DblQuoted:
			for _, sub := range v.Parts {
				if lit, ok := sub.(*syntax.Lit); ok {
					b = append(b, lit.Value...)
				}
			}
		default:
			// Dynamic word part — bail; we recover only literal text.
			return ""
		}
	}
	return string(b)
}
