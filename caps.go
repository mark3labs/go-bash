package gobash

import (
	"math"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// enforceExpansionCaps applies the SPEC §6 expansion-side runtime caps
// to the parsed AST before execution begins. These caps cover constructs
// mvdan/sh does not natively bound:
//
//   - MaxBraceExpansionResults (§2.1, §6.4): rejects oversized brace
//     expansions ({a,b,c}, {1..N}) by computing each Word's brace
//     cardinality after syntax.SplitBraces.
//   - MaxSubstitutionDepth (§2.1): rejects nested command/process
//     substitution beyond the configured depth. Computed structurally
//     over the AST.
//   - MaxArrayElements (§2.1): rejects oversized arr=(…) literals,
//     summing each element word's brace cardinality.
//
// MaxStringLength is enforced at the CallHandler in bash.go (per-arg
// length limit on every invoked command). MaxGlobOperations is enforced
// at the ReadDirHandler via interp.Config.ReadDirHook (per-readdir
// counter).
//
// The static-analysis approach mirrors the just-bash TS port's
// pre-expansion bookkeeping: just-bash counts brace cardinality in
// `src/shell/brace-expand.ts` before materializing the expansion,
// rather than letting runtime allocation balloon. (Citation kept
// approximate — the just-bash repo is read-only spec source and we do
// not vendor it; see DECISIONS.md.)
func enforceExpansionCaps(file *syntax.File, limits ResolvedLimits) error {
	if e := checkBraceCap(file, limits.MaxBraceExpansionResults); e != nil {
		return e
	}
	if e := checkSubstDepth(file, limits.MaxSubstitutionDepth); e != nil {
		return e
	}
	if e := checkArrayElems(file, limits.MaxArrayElements); e != nil {
		return e
	}
	return nil
}

// wordBraceCardinality returns the brace-expansion cardinality of w
// WITHOUT mutating it: SplitBraces is destructive and produces
// *syntax.BraceExp nodes that mvdan/sh's own syntax.Walk panics on, so
// we copy the Word, split the copy, and discard. The runtime expand
// path inside mvdan/sh likewise operates on a copy (see expand.go
// FieldsSeq), so static counting here and runtime expansion there
// stay in lock-step without sharing AST state.
func wordBraceCardinality(w *syntax.Word, limit int) int {
	if w == nil || !mayHaveBraces(w) {
		return 1
	}
	cp := *w
	if !syntax.SplitBraces(&cp) {
		return 1
	}
	return braceCardinality(&cp, limit)
}

func mayHaveBraces(w *syntax.Word) bool {
	for _, p := range w.Parts {
		if l, ok := p.(*syntax.Lit); ok && strings.ContainsRune(l.Value, '{') {
			return true
		}
	}
	return false
}

func checkBraceCap(file *syntax.File, limit int) *ExecutionLimitError {
	if limit <= 0 {
		return nil
	}
	var tripped *ExecutionLimitError
	syntax.Walk(file, func(n syntax.Node) bool {
		if tripped != nil {
			return false
		}
		w, ok := n.(*syntax.Word)
		if !ok {
			return true
		}
		if !mayHaveBraces(w) {
			return true
		}
		if card := wordBraceCardinality(w, limit); card > limit {
			tripped = &ExecutionLimitError{Limit: "MaxBraceExpansionResults", Value: limit}
			return false
		}
		return true
	})
	return tripped
}

// braceCardinality returns the number of expanded words SplitBraces +
// expand.Braces would yield for w. limit is an early-exit ceiling: once
// the cardinality is known to exceed limit, we return limit+1 without
// further computation, which both saturates the integer and short-
// circuits absurdly-wide sequences like {1..2000000}.
func braceCardinality(w *syntax.Word, limit int) int {
	card := 1
	for _, part := range w.Parts {
		be, ok := part.(*syntax.BraceExp)
		if !ok {
			continue
		}
		var beCard int
		if be.Sequence {
			beCard = sequenceCardinality(be.Elems, limit)
		} else {
			beCard = 0
			for _, e := range be.Elems {
				if e == nil {
					beCard++
					continue
				}
				beCard += braceCardinality(e, limit)
				if beCard > limit {
					return limit + 1
				}
			}
		}
		if beCard <= 0 {
			beCard = 1
		}
		if card <= 0 {
			card = 1
		}
		// Saturating multiply: if card * beCard would exceed limit+1,
		// short-circuit. Guard against div-by-zero with the card<=0
		// check above.
		if beCard > (limit+1)/card {
			return limit + 1
		}
		card *= beCard
		if card > limit {
			return limit + 1
		}
	}
	return card
}

func sequenceCardinality(elems []*syntax.Word, limit int) int {
	if len(elems) < 2 {
		return 1
	}
	step := 1
	if len(elems) >= 3 {
		s, err := wordToInt(elems[2])
		if err == nil && s != 0 {
			if s < 0 {
				s = -s
			}
			step = s
		}
	}
	if a, errA := wordToInt(elems[0]); errA == nil {
		if b, errB := wordToInt(elems[1]); errB == nil {
			diff := b - a
			if diff < 0 {
				diff = -diff
			}
			if diff > math.MaxInt32 {
				return limit + 1
			}
			return diff/step + 1
		}
	}
	if ca, okA := wordToChar(elems[0]); okA {
		if cb, okB := wordToChar(elems[1]); okB {
			diff := int(cb) - int(ca)
			if diff < 0 {
				diff = -diff
			}
			return diff/step + 1
		}
	}
	return 1
}

func wordToInt(w *syntax.Word) (int, error) {
	s := wordLit(w)
	if s == "" {
		return 0, errNoLit
	}
	return strconv.Atoi(s)
}

func wordToChar(w *syntax.Word) (rune, bool) {
	s := wordLit(w)
	runes := []rune(s)
	if len(runes) != 1 {
		return 0, false
	}
	return runes[0], true
}

func wordLit(w *syntax.Word) string {
	var sb strings.Builder
	for _, p := range w.Parts {
		l, ok := p.(*syntax.Lit)
		if !ok {
			return ""
		}
		sb.WriteString(l.Value)
	}
	return sb.String()
}

var errNoLit = errString("not a literal")

type errString string

func (e errString) Error() string { return string(e) }

// checkSubstDepth bounds the maximum nesting depth of *syntax.CmdSubst
// and *syntax.ProcSubst nodes. Parameter expansion (${VAR/.../...})
// does not count toward depth — only command/process substitution
// recursion does, which matches the spec's "MaxSubstitutionDepth" intent.
// Runtime-only nesting via eval is bounded separately by MaxCallDepth.
func checkSubstDepth(file *syntax.File, limit int) *ExecutionLimitError {
	if limit <= 0 {
		return nil
	}
	var deepest int
	var walk func(n syntax.Node, depth int)
	walk = func(n syntax.Node, depth int) {
		if n == nil {
			return
		}
		if depth > deepest {
			deepest = depth
		}
		syntax.Walk(n, func(child syntax.Node) bool {
			if child == n {
				return true
			}
			switch child.(type) {
			case *syntax.CmdSubst, *syntax.ProcSubst:
				walk(child, depth+1)
				return false
			}
			return true
		})
	}
	walk(file, 0)
	if deepest > limit {
		return &ExecutionLimitError{Limit: "MaxSubstitutionDepth", Value: limit}
	}
	return nil
}

func checkArrayElems(file *syntax.File, limit int) *ExecutionLimitError {
	if limit <= 0 {
		return nil
	}
	var tripped *ExecutionLimitError
	syntax.Walk(file, func(n syntax.Node) bool {
		if tripped != nil {
			return false
		}
		ae, ok := n.(*syntax.ArrayExpr)
		if !ok {
			return true
		}
		total := 0
		for _, el := range ae.Elems {
			if el == nil || el.Value == nil {
				total++
				continue
			}
			c := wordBraceCardinality(el.Value, limit)
			if c < 1 {
				c = 1
			}
			total += c
			if total > limit {
				tripped = &ExecutionLimitError{Limit: "MaxArrayElements", Value: limit}
				return false
			}
		}
		return true
	})
	return tripped
}
