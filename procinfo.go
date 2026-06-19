package gobash

import (
	"strconv"

	"mvdan.cc/sh/v3/syntax"
)

// rewriteProcInfo rewrites `$$`, `$PPID`, and `$BASHPID` parameter
// expansions in the parsed AST so that they expand to the virtualized
// values from procInfo instead of the host process's real PID/PPID.
//
// # Why a pre-Run AST pass?
//
// mvdan/sh hardcodes these three names in interp/vars.go's lookupVar:
//
//   - `$$` returns strconv.Itoa(os.Getpid())
//   - `$PPID` returns strconv.Itoa(os.Getppid())
//
// The switch in lookupVar fires BEFORE env consultation, so setting
// `$` or `PPID` in the env environ passed to interp.Env(...) has no
// effect. `BASHPID` is not in the hardcoded set, so it would fall
// through to env lookup, but the env value would be the same in every
// subshell — SPEC §12 says BASHPID increments per subshell. An AST
// rewrite is therefore the cleanest hook: we replace each occurrence
// with a *syntax.Lit holding the desired literal value, leaving every
// other expansion semantic intact.
//
// # SPEC §12 BASHPID counter
//
// "BASHPID starts at virtual pid; each subshell increments a counter."
// We walk the AST in DFS pre-order, assigning each *syntax.Subshell
// node a unique counter value (procInfo.PID+1 for the first subshell,
// +2 for the next, etc.). References to `$BASHPID` inside a subshell
// expand to the innermost enclosing subshell's counter value;
// references outside any subshell expand to procInfo.PID.
//
// # What is NOT rewritten
//
// Only "simple" parameter expansions in the form `$X` or `${X}` (no
// modifiers, no subscript, no length / width / replace / expansion /
// indirection) are eligible. Complex forms like `${BASHPID:-fallback}`
// or `${#PPID}` are left alone — those would require generating a Lit
// in a context that expects a ParamExp child, which the surrounding
// syntax does not allow. Such forms are vanishingly rare in real
// scripts; documented in DECISIONS.md.
func rewriteProcInfo(file *syntax.File, pid, ppid int) {
	if file == nil {
		return
	}
	pidLit := strconv.Itoa(pid)
	ppidLit := strconv.Itoa(ppid)

	// First pass: assign each Subshell a counter value in DFS pre-order.
	// We track [start, end) offsets so a second pass can resolve any
	// $BASHPID position to its innermost enclosing subshell.
	type ssRange struct {
		start, end uint
		lit        string
	}
	var ranges []ssRange
	counter := pid
	syntax.Walk(file, func(n syntax.Node) bool {
		if ss, ok := n.(*syntax.Subshell); ok {
			counter++
			ranges = append(ranges, ssRange{
				start: ss.Pos().Offset(),
				end:   ss.End().Offset(),
				lit:   strconv.Itoa(counter),
			})
		}
		return true
	})

	bashpidFor := func(p syntax.Pos) string {
		off := p.Offset()
		best := pidLit
		// ranges is in DFS pre-order: a later entry that still contains
		// off is necessarily more deeply nested. Linear scan keeps the
		// implementation obvious; scripts with thousands of subshells
		// are vanishingly rare.
		for _, r := range ranges {
			if off >= r.start && off < r.end {
				best = r.lit
			}
		}
		return best
	}

	// Second pass: replace eligible ParamExp WordParts with Lit nodes
	// inside Word and DblQuoted parents (the only WordPart-bearing
	// containers in bash mode).
	syntax.Walk(file, func(n syntax.Node) bool {
		switch w := n.(type) {
		case *syntax.Word:
			rewriteProcInfoParts(w.Parts, pidLit, ppidLit, bashpidFor)
		case *syntax.DblQuoted:
			rewriteProcInfoParts(w.Parts, pidLit, ppidLit, bashpidFor)
		}
		return true
	})
}

func rewriteProcInfoParts(parts []syntax.WordPart, pidLit, ppidLit string, bashpidFor func(syntax.Pos) string) {
	for i, p := range parts {
		pe, ok := p.(*syntax.ParamExp)
		if !ok {
			continue
		}
		if !isSimpleParamExp(pe) {
			continue
		}
		var lit string
		switch pe.Param.Value {
		case "$":
			lit = pidLit
		case "PPID":
			lit = ppidLit
		case "BASHPID":
			lit = bashpidFor(pe.Pos())
		default:
			continue
		}
		parts[i] = &syntax.Lit{
			ValuePos: pe.Pos(),
			ValueEnd: pe.End(),
			Value:    lit,
		}
	}
}

// isSimpleParamExp returns true when pe is the bare `$X` / `${X}`
// shape — i.e. the same predicate mvdan/sh's syntax.ParamExp.simple()
// uses internally (unexported there, replicated here). We restrict
// rewriting to this shape so complex modifiers (length, slice,
// substring, alternate-value, etc.) are left to the runtime expander.
func isSimpleParamExp(pe *syntax.ParamExp) bool {
	return pe != nil &&
		pe.Param != nil &&
		pe.Flags == nil &&
		!pe.Excl && !pe.Length && !pe.Width && !pe.IsSet &&
		pe.NestedParam == nil && pe.Index == nil &&
		len(pe.Modifiers) == 0 && pe.Slice == nil &&
		pe.Repl == nil && pe.Names == 0 && pe.Exp == nil
}
