package parser

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/mark3labs/go-bash/ast"
)

// translator owns per-Parse state: depth counter for MaxParserDepth,
// running heredoc-size budget.
type translator struct {
	depth int
}

// enter / leave bracket every recursive call site. Returns an error if
// depth exceeds MaxParserDepth.
func (t *translator) enter() error {
	t.depth++
	if t.depth > MaxParserDepth {
		return &ParseError{
			Msg: fmt.Sprintf("parser depth exceeded: > %d", MaxParserDepth),
		}
	}
	return nil
}

func (t *translator) leave() { t.depth-- }

// posLine returns the 1-based line number from an mvdan/sh Pos, or 0
// if the position is invalid.
func posLine(p syntax.Pos) int {
	if !p.IsValid() {
		return 0
	}
	return int(p.Line())
}

func (t *translator) script(file *syntax.File) (*ast.Script, error) {
	if err := t.enter(); err != nil {
		return nil, err
	}
	defer t.leave()

	out := &ast.Script{Origin: file}
	if len(file.Stmts) > 0 {
		out.Line = posLine(file.Stmts[0].Pos())
	}
	for _, st := range file.Stmts {
		converted, err := t.statement(st)
		if err != nil {
			return nil, err
		}
		out.Statements = append(out.Statements, converted)
	}
	return out, nil
}

// statement translates one top-level *syntax.Stmt into our
// *ast.Statement. The Stmt may carry a BinaryCmd tree of && / || that
// we flatten into Pipelines + Operators.
func (t *translator) statement(st *syntax.Stmt) (*ast.Statement, error) {
	if err := t.enter(); err != nil {
		return nil, err
	}
	defer t.leave()

	out := &ast.Statement{
		Background: st.Background,
		Line:       posLine(st.Pos()),
	}

	// Flatten && / || chain rooted at st.Cmd. Each leaf becomes one
	// Pipeline; each connective becomes one Operators entry.
	pipes, ops, err := t.flattenAndOr(st.Cmd, st.Negated, st.Redirs)
	if err != nil {
		return nil, err
	}
	out.Pipelines = pipes
	out.Operators = ops
	return out, nil
}

// flattenAndOr walks a tree of BinaryCmd{AndStmt|OrStmt} and returns
// the flattened pipeline / operator slices. Leaves dispatch to
// pipelineFromCmd.
//
// The redirs / negated parameters apply only to the leftmost leaf
// (i.e. the surrounding Stmt's flags).
func (t *translator) flattenAndOr(cmd syntax.Command, negated bool, redirs []*syntax.Redirect) ([]*ast.Pipeline, []string, error) {
	if err := t.enter(); err != nil {
		return nil, nil, err
	}
	defer t.leave()

	if bin, ok := cmd.(*syntax.BinaryCmd); ok && (bin.Op == syntax.AndStmt || bin.Op == syntax.OrStmt) {
		leftPipes, leftOps, err := t.flattenAndOr(bin.X.Cmd, bin.X.Negated || negated, append(append([]*syntax.Redirect(nil), redirs...), bin.X.Redirs...))
		if err != nil {
			return nil, nil, err
		}
		var op string
		switch bin.Op {
		case syntax.AndStmt:
			op = "&&"
		case syntax.OrStmt:
			op = "||"
		}
		rightPipes, rightOps, err := t.flattenAndOr(bin.Y.Cmd, bin.Y.Negated, bin.Y.Redirs)
		if err != nil {
			return nil, nil, err
		}
		ops := append(leftOps, op)
		ops = append(ops, rightOps...)
		pipes := append(leftPipes, rightPipes...)
		return pipes, ops, nil
	}

	// Leaf: build one pipeline.
	pipe, err := t.pipelineFromCmd(cmd, negated, redirs)
	if err != nil {
		return nil, nil, err
	}
	return []*ast.Pipeline{pipe}, nil, nil
}

// pipelineFromCmd walks a BinaryCmd{Pipe|PipeAll} chain rooted at cmd
// and returns one Pipeline. Leaves are translated into individual
// Command nodes. surrounding redirs are attached to the leftmost
// SimpleCommand (best-effort — only sensible for simple commands).
func (t *translator) pipelineFromCmd(cmd syntax.Command, negated bool, redirs []*syntax.Redirect) (*ast.Pipeline, error) {
	if err := t.enter(); err != nil {
		return nil, err
	}
	defer t.leave()

	cmds, pipeStderr, err := t.flattenPipe(cmd)
	if err != nil {
		return nil, err
	}

	out := &ast.Pipeline{
		Commands:   cmds,
		PipeStderr: pipeStderr,
		Negated:    negated,
		Line:       posLine(cmd.Pos()),
	}

	// Attach the Stmt-level redirs to the leftmost command if it has
	// a Redirections slot; otherwise drop them (covered by the
	// origin-based round-trip path).
	if len(redirs) > 0 && len(out.Commands) > 0 {
		switch c := out.Commands[0].(type) {
		case *ast.SimpleCommand:
			redirNodes, err := t.redirections(redirs)
			if err != nil {
				return nil, err
			}
			c.Redirections = append(redirNodes, c.Redirections...)
		case *ast.Subshell:
			redirNodes, err := t.redirections(redirs)
			if err != nil {
				return nil, err
			}
			c.Redirections = append(redirNodes, c.Redirections...)
		case *ast.Group:
			redirNodes, err := t.redirections(redirs)
			if err != nil {
				return nil, err
			}
			c.Redirections = append(redirNodes, c.Redirections...)
		}
	}
	return out, nil
}

// flattenPipe flattens a BinaryCmd{Pipe} tree into a flat slice.
func (t *translator) flattenPipe(cmd syntax.Command) ([]ast.Command, []bool, error) {
	if err := t.enter(); err != nil {
		return nil, nil, err
	}
	defer t.leave()

	if bin, ok := cmd.(*syntax.BinaryCmd); ok && (bin.Op == syntax.Pipe || bin.Op == syntax.PipeAll) {
		lCmds, lStderr, err := t.flattenPipe(bin.X.Cmd)
		if err != nil {
			return nil, nil, err
		}
		rCmds, rStderr, err := t.flattenPipe(bin.Y.Cmd)
		if err != nil {
			return nil, nil, err
		}
		stderr := bin.Op == syntax.PipeAll
		cmds := append(lCmds, rCmds...)
		flags := append(append(lStderr, stderr), rStderr...)
		return cmds, flags, nil
	}

	node, err := t.command(cmd)
	if err != nil {
		return nil, nil, err
	}
	return []ast.Command{node}, nil, nil
}

// command dispatches on the concrete syntax.Command type.
func (t *translator) command(cmd syntax.Command) (ast.Command, error) {
	if err := t.enter(); err != nil {
		return nil, err
	}
	defer t.leave()

	switch c := cmd.(type) {
	case *syntax.CallExpr:
		return t.callExpr(c)
	case *syntax.Block:
		return t.block(c)
	case *syntax.Subshell:
		return t.subshell(c)
	case *syntax.IfClause:
		return t.ifClause(c)
	case *syntax.ForClause:
		return t.forClause(c)
	case *syntax.WhileClause:
		return t.whileClause(c)
	case *syntax.CaseClause:
		return t.caseClause(c)
	case *syntax.FuncDecl:
		return t.funcDecl(c)
	case *syntax.ArithmCmd:
		return &ast.ArithCmd{Expr: arithmString(c.X), Line: posLine(c.Pos())}, nil
	case *syntax.TestClause:
		return &ast.CondCmd{Expr: testExprString(c.X), Line: posLine(c.Pos())}, nil
	case *syntax.DeclClause:
		return t.declClause(c)
	case *syntax.LetClause:
		// let a=b — model as SimpleCommand("let", args).
		args := make([]*ast.Word, 0, len(c.Exprs)+1)
		for _, e := range c.Exprs {
			args = append(args, &ast.Word{Parts: []ast.WordPart{&ast.Literal{Value: arithmString(e)}}})
		}
		return &ast.SimpleCommand{
			Name: &ast.Word{Parts: []ast.WordPart{&ast.Literal{Value: "let"}}},
			Args: args,
			Line: posLine(c.Pos()),
		}, nil
	case *syntax.TimeClause:
		// Approximation: time keyword + nested stmt. We surface it as
		// a SimpleCommand("time", ...) with stub args so the tree is
		// still well-formed.
		return &ast.SimpleCommand{
			Name: &ast.Word{Parts: []ast.WordPart{&ast.Literal{Value: "time"}}},
			Line: posLine(c.Pos()),
		}, nil
	}
	// Catch-all: model the command as an empty SimpleCommand so the
	// shape is still consistent. The origin pointer preserves the
	// real syntax for round-trip.
	return &ast.SimpleCommand{Line: posLine(cmd.Pos())}, nil
}

func (t *translator) callExpr(c *syntax.CallExpr) (ast.Command, error) {
	out := &ast.SimpleCommand{Line: posLine(c.Pos())}
	for _, a := range c.Assigns {
		assign, err := t.assignment(a)
		if err != nil {
			return nil, err
		}
		out.Assignments = append(out.Assignments, assign)
	}
	if len(c.Args) > 0 {
		name, err := t.word(c.Args[0])
		if err != nil {
			return nil, err
		}
		out.Name = name
		for _, w := range c.Args[1:] {
			converted, err := t.word(w)
			if err != nil {
				return nil, err
			}
			out.Args = append(out.Args, converted)
		}
	}
	return out, nil
}

func (t *translator) assignment(a *syntax.Assign) (*ast.Assignment, error) {
	out := &ast.Assignment{Append: a.Append, Line: posLine(a.Pos())}
	if a.Name != nil {
		out.Name = a.Name.Value
	}
	if a.Value != nil {
		w, err := t.word(a.Value)
		if err != nil {
			return nil, err
		}
		out.Value = w
	}
	if a.Array != nil {
		init := &ast.ArrayInit{}
		for _, el := range a.Array.Elems {
			var idx string
			if el.Index != nil {
				idx = arithmString(el.Index)
			}
			var val *ast.Word
			if el.Value != nil {
				v, err := t.word(el.Value)
				if err != nil {
					return nil, err
				}
				val = v
			}
			init.Elements = append(init.Elements, &ast.ArrayElement{Index: idx, Value: val})
		}
		out.Array = init
	}
	return out, nil
}

// word translates a *syntax.Word.
func (t *translator) word(w *syntax.Word) (*ast.Word, error) {
	if err := t.enter(); err != nil {
		return nil, err
	}
	defer t.leave()

	out := &ast.Word{}
	for _, p := range w.Parts {
		part, err := t.wordPart(p)
		if err != nil {
			return nil, err
		}
		out.Parts = append(out.Parts, part)
	}
	return out, nil
}

func (t *translator) wordPart(p syntax.WordPart) (ast.WordPart, error) {
	if err := t.enter(); err != nil {
		return nil, err
	}
	defer t.leave()

	switch v := p.(type) {
	case *syntax.Lit:
		return &ast.Literal{Value: v.Value}, nil
	case *syntax.SglQuoted:
		if v.Dollar {
			return &ast.AnsiCQuoted{Value: v.Value}, nil
		}
		return &ast.SingleQuoted{Value: v.Value}, nil
	case *syntax.DblQuoted:
		dq := &ast.DoubleQuoted{}
		for _, sub := range v.Parts {
			converted, err := t.wordPart(sub)
			if err != nil {
				return nil, err
			}
			dq.Parts = append(dq.Parts, converted)
		}
		return dq, nil
	case *syntax.ParamExp:
		return t.paramExp(v)
	case *syntax.CmdSubst:
		body, err := t.stmtList(v.Stmts)
		if err != nil {
			return nil, err
		}
		return &ast.CommandSubstitution{Body: body, Backtick: v.Backquotes}, nil
	case *syntax.ArithmExp:
		return &ast.ArithmeticExpansion{Expr: arithmString(v.X)}, nil
	case *syntax.ProcSubst:
		body, err := t.stmtList(v.Stmts)
		if err != nil {
			return nil, err
		}
		dir := "<"
		if v.Op == syntax.CmdOut {
			dir = ">"
		}
		return &ast.ProcessSubstitution{Direction: dir, Body: body}, nil
	case *syntax.ExtGlob:
		op := strings.TrimSuffix(v.Op.String(), "(")
		var pat string
		if v.Pattern != nil {
			pat = v.Pattern.Value
		}
		return &ast.ExtGlob{Op: op, Pattern: pat}, nil
	}
	// Unknown part: model as empty literal so the structure is intact.
	return &ast.Literal{}, nil
}

func (t *translator) paramExp(v *syntax.ParamExp) (ast.WordPart, error) {
	out := &ast.ParameterExpansion{
		Indirect: v.Excl,
		Length:   v.Length,
	}
	if v.Param != nil {
		out.Parameter = v.Param.Value
	}
	if v.Index != nil {
		out.Index = arithmString(v.Index)
	}
	switch {
	case v.Slice != nil:
		out.Operation = &ast.SubstringRange{
			Offset: arithmString(v.Slice.Offset),
			Length: arithmString(v.Slice.Length),
		}
	case v.Repl != nil:
		var orig, with *ast.Word
		if v.Repl.Orig != nil {
			w, err := t.word(v.Repl.Orig)
			if err != nil {
				return nil, err
			}
			orig = w
		}
		if v.Repl.With != nil {
			w, err := t.word(v.Repl.With)
			if err != nil {
				return nil, err
			}
			with = w
		}
		out.Operation = &ast.Replace{Pattern: orig, Replacement: with, All: v.Repl.All}
	case v.Exp != nil:
		op, err := t.expansionOp(v.Exp)
		if err != nil {
			return nil, err
		}
		out.Operation = op
	case v.Names != 0:
		out.Operation = &ast.Names{SplitWords: v.Names == syntax.NamesPrefixWords}
	}
	return out, nil
}

func (t *translator) expansionOp(e *syntax.Expansion) (ast.ParamOp, error) {
	var w *ast.Word
	if e.Word != nil {
		converted, err := t.word(e.Word)
		if err != nil {
			return nil, err
		}
		w = converted
	}
	switch e.Op {
	case syntax.DefaultUnset:
		return &ast.DefaultValue{Word: w}, nil
	case syntax.DefaultUnsetOrNull:
		return &ast.DefaultValue{Word: w, ColonNull: true}, nil
	case syntax.AssignUnset:
		return &ast.Assign{Word: w}, nil
	case syntax.AssignUnsetOrNull:
		return &ast.Assign{Word: w, ColonNull: true}, nil
	case syntax.ErrorUnset:
		return &ast.ErrorOp{Word: w}, nil
	case syntax.ErrorUnsetOrNull:
		return &ast.ErrorOp{Word: w, ColonNull: true}, nil
	case syntax.AlternateUnset:
		return &ast.Alternative{Word: w}, nil
	case syntax.AlternateUnsetOrNull:
		return &ast.Alternative{Word: w, ColonNull: true}, nil
	case syntax.RemSmallPrefix:
		return &ast.PatternRemove{Pattern: w, Op: "#"}, nil
	case syntax.RemLargePrefix:
		return &ast.PatternRemove{Pattern: w, Op: "##"}, nil
	case syntax.RemSmallSuffix:
		return &ast.PatternRemove{Pattern: w, Op: "%"}, nil
	case syntax.RemLargeSuffix:
		return &ast.PatternRemove{Pattern: w, Op: "%%"}, nil
	case syntax.UpperFirst:
		return &ast.CaseModify{Pattern: w, Op: "^"}, nil
	case syntax.UpperAll:
		return &ast.CaseModify{Pattern: w, Op: "^^"}, nil
	case syntax.LowerFirst:
		return &ast.CaseModify{Pattern: w, Op: ","}, nil
	case syntax.LowerAll:
		return &ast.CaseModify{Pattern: w, Op: ",,"}, nil
	case syntax.OtherParamOps:
		op := ""
		if w != nil && len(w.Parts) > 0 {
			if lit, ok := w.Parts[0].(*ast.Literal); ok {
				op = lit.Value
			}
		}
		return &ast.Transform{Op: op}, nil
	}
	return &ast.DefaultValue{Word: w}, nil
}

func (t *translator) stmtList(stmts []*syntax.Stmt) ([]*ast.Statement, error) {
	out := make([]*ast.Statement, 0, len(stmts))
	for _, st := range stmts {
		converted, err := t.statement(st)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func (t *translator) redirections(rs []*syntax.Redirect) ([]*ast.Redirection, error) {
	out := make([]*ast.Redirection, 0, len(rs))
	for _, r := range rs {
		converted, err := t.redirection(r)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func (t *translator) redirection(r *syntax.Redirect) (*ast.Redirection, error) {
	out := &ast.Redirection{FD: -1, Op: r.Op.String()}
	if r.N != nil {
		fmt.Sscanf(r.N.Value, "%d", &out.FD) //nolint:errcheck // best-effort fd parse
	}
	if r.Word != nil {
		w, err := t.word(r.Word)
		if err != nil {
			return nil, err
		}
		out.Word = w
	}
	if r.Hdoc != nil {
		body := wordRawString(r.Hdoc)
		if len(body) > MaxHeredocSize {
			return nil, &ParseError{
				Msg: fmt.Sprintf("heredoc body too large: %d bytes (max %d)", len(body), MaxHeredocSize),
			}
		}
		tag := ""
		if r.Word != nil {
			tag = wordRawString(r.Word)
		}
		expand := true
		if r.Word != nil && len(r.Word.Parts) > 0 {
			if _, ok := r.Word.Parts[0].(*syntax.SglQuoted); ok {
				expand = false
			}
		}
		out.Heredoc = &ast.Heredoc{
			Tag:       strings.TrimSpace(tag),
			Body:      body,
			StripTabs: r.Op == syntax.DashHdoc,
			Expand:    expand,
		}
	}
	return out, nil
}

func (t *translator) block(b *syntax.Block) (ast.Command, error) {
	body, err := t.stmtList(b.Stmts)
	if err != nil {
		return nil, err
	}
	return &ast.Group{Body: body, Line: posLine(b.Pos())}, nil
}

func (t *translator) subshell(s *syntax.Subshell) (ast.Command, error) {
	body, err := t.stmtList(s.Stmts)
	if err != nil {
		return nil, err
	}
	return &ast.Subshell{Body: body, Line: posLine(s.Pos())}, nil
}

func (t *translator) ifClause(c *syntax.IfClause) (ast.Command, error) {
	out := &ast.IfStmt{Line: posLine(c.Pos())}
	cur := c
	for cur != nil {
		cond, err := t.stmtList(cur.Cond)
		if err != nil {
			return nil, err
		}
		body, err := t.stmtList(cur.Then)
		if err != nil {
			return nil, err
		}
		out.Branches = append(out.Branches, ast.IfBranch{Cond: cond, Body: body})
		if cur.Else == nil {
			break
		}
		// In mvdan/sh, `else` without `elif` is encoded as an IfClause
		// with empty Cond — distinguish via Position vs ThenPos.
		if len(cur.Else.Cond) == 0 {
			elseBody, err := t.stmtList(cur.Else.Then)
			if err != nil {
				return nil, err
			}
			out.Else = elseBody
			break
		}
		cur = cur.Else
	}
	return out, nil
}

func (t *translator) forClause(c *syntax.ForClause) (ast.Command, error) {
	body, err := t.stmtList(c.Do)
	if err != nil {
		return nil, err
	}
	switch loop := c.Loop.(type) {
	case *syntax.WordIter:
		out := &ast.ForStmt{Body: body, Line: posLine(c.Pos())}
		if loop.Name != nil {
			out.Var = loop.Name.Value
		}
		for _, w := range loop.Items {
			converted, err := t.word(w)
			if err != nil {
				return nil, err
			}
			out.Words = append(out.Words, converted)
		}
		return out, nil
	case *syntax.CStyleLoop:
		return &ast.CStyleFor{
			Init: arithmString(loop.Init),
			Cond: arithmString(loop.Cond),
			Post: arithmString(loop.Post),
			Body: body,
			Line: posLine(c.Pos()),
		}, nil
	}
	return &ast.ForStmt{Body: body, Line: posLine(c.Pos())}, nil
}

func (t *translator) whileClause(c *syntax.WhileClause) (ast.Command, error) {
	cond, err := t.stmtList(c.Cond)
	if err != nil {
		return nil, err
	}
	body, err := t.stmtList(c.Do)
	if err != nil {
		return nil, err
	}
	return &ast.WhileStmt{Cond: cond, Body: body, Until: c.Until, Line: posLine(c.Pos())}, nil
}

func (t *translator) caseClause(c *syntax.CaseClause) (ast.Command, error) {
	subj, err := t.word(c.Word)
	if err != nil {
		return nil, err
	}
	out := &ast.CaseStmt{Subject: subj, Line: posLine(c.Pos())}
	for _, it := range c.Items {
		item := ast.CaseItem{Terminator: it.Op.String()}
		for _, p := range it.Patterns {
			w, err := t.word(p)
			if err != nil {
				return nil, err
			}
			item.Patterns = append(item.Patterns, w)
		}
		body, err := t.stmtList(it.Stmts)
		if err != nil {
			return nil, err
		}
		item.Body = body
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (t *translator) funcDecl(f *syntax.FuncDecl) (ast.Command, error) {
	body, err := t.command(f.Body.Cmd)
	if err != nil {
		return nil, err
	}
	name := ""
	if f.Name != nil {
		name = f.Name.Value
	}
	return &ast.FunctionDef{Name: name, Body: body, Line: posLine(f.Pos())}, nil
}

func (t *translator) declClause(d *syntax.DeclClause) (ast.Command, error) {
	out := &ast.SimpleCommand{Line: posLine(d.Pos())}
	if d.Variant != nil {
		out.Name = &ast.Word{Parts: []ast.WordPart{&ast.Literal{Value: d.Variant.Value}}}
	}
	for _, a := range d.Args {
		// Each arg can be either a naked option/name (model as Args)
		// or an assignment (model as Assignment).
		if a.Naked && a.Name == nil && a.Value != nil {
			w, err := t.word(a.Value)
			if err != nil {
				return nil, err
			}
			out.Args = append(out.Args, w)
			continue
		}
		assign, err := t.assignment(a)
		if err != nil {
			return nil, err
		}
		out.Assignments = append(out.Assignments, assign)
	}
	return out, nil
}

// arithmString renders an arithmetic expression node back to its
// source text by using the printer on a synthetic Word/Stmt.
func arithmString(expr syntax.ArithmExpr) string {
	if expr == nil {
		return ""
	}
	var b strings.Builder
	if err := syntax.NewPrinter().Print(&b, &syntax.Stmt{Cmd: &syntax.ArithmCmd{X: expr}}); err == nil {
		// Strip the surrounding "((" and "))\n" inserted by the printer.
		s := strings.TrimSpace(b.String())
		s = strings.TrimPrefix(s, "((")
		s = strings.TrimSuffix(s, "))")
		return strings.TrimSpace(s)
	}
	return ""
}

// testExprString renders a [[ ]] expression back to text via the printer.
func testExprString(expr syntax.TestExpr) string {
	if expr == nil {
		return ""
	}
	var b strings.Builder
	if err := syntax.NewPrinter().Print(&b, &syntax.Stmt{Cmd: &syntax.TestClause{X: expr}}); err == nil {
		s := strings.TrimSpace(b.String())
		s = strings.TrimPrefix(s, "[[")
		s = strings.TrimSuffix(s, "]]")
		return strings.TrimSpace(s)
	}
	return ""
}

// wordRawString renders a Word's literal text via the printer (used for
// heredoc bodies and tags).
func wordRawString(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	if lit := w.Lit(); lit != "" {
		return lit
	}
	var b strings.Builder
	if err := syntax.NewPrinter().Print(&b, &syntax.File{Stmts: []*syntax.Stmt{{Cmd: &syntax.CallExpr{Args: []*syntax.Word{w}}}}}); err == nil {
		return strings.TrimRight(b.String(), "\n")
	}
	return ""
}
