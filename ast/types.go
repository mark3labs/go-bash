// Package ast defines the typed Go AST that mirrors the just-bash
// TypeScript shape in src/ast/types.ts. It is the public AST surface
// produced by parser.Parse / parser.ParseString and consumed by the
// (future, Phase 13) transform plugin pipeline.
//
// # Stability
//
// Add new node types freely; renaming or removing exported fields is a
// breaking change. Each compound-command type leaves room for parity-only
// fields that are not yet populated by the translator — Phase 4 ships the
// public shape; later phases (transform plugins in Phase 13, interpreter
// bridge in Phase 5) drive incremental fills.
//
// # Origin pointer
//
// Script carries an Origin field pointing back at the
// mvdan.cc/sh/v3/syntax.File the parser produced. transform.Serialize
// uses Origin to byte-faithfully render the script back to bash source.
// Origin is nil for plugin-synthesized scripts (Phase 13+); the inverse
// translator in the transform package handles that case via best-effort
// reconstruction with default positions.
package ast

import "mvdan.cc/sh/v3/syntax"

// Node is the closed type implemented by every AST node. The nodeMarker
// method is unexported to prevent external implementations.
type Node interface{ nodeMarker() }

// Script is the root of a parsed bash file.
type Script struct {
	Statements []*Statement
	Line       int

	// Origin is the back-reference to the original mvdan.cc/sh/v3
	// *syntax.File the parser walked. transform.Serialize uses it for
	// byte-identical round-trips. Nil for plugin-synthesized scripts.
	Origin *syntax.File
}

func (*Script) nodeMarker() {}

// Statement is one and-or list: a sequence of pipelines joined by &&
// or || (Operators), optionally terminated with & (Background).
//
// Note: the just-bash AST overloads Operators to include ";" for
// in-source separators between pipelines on the same line; the Go port
// reserves Operators for "&&" and "||" and represents ";"-separated
// commands as distinct top-level Statements. This preserves the natural
// 1:1 mapping with mvdan.cc/sh/v3's *syntax.Stmt and is documented as a
// resolved decision in handoffs/phase-4.md.
type Statement struct {
	Pipelines  []*Pipeline
	Operators  []string // each "&&" or "||"; len = len(Pipelines)-1
	Background bool
	Line       int
}

func (*Statement) nodeMarker() {}

// Pipeline is a sequence of Commands connected by | (or |& when the
// corresponding PipeStderr entry is true). Negated is true when the
// leftmost pipeline element carries a leading `!`.
type Pipeline struct {
	Commands   []Command
	PipeStderr []bool // length = len(Commands) - 1
	Negated    bool
	Line       int
}

func (*Pipeline) nodeMarker() {}

// Command is one element of a Pipeline. Every command type also
// implements Node.
type Command interface {
	Node
	commandMarker()
}

// SimpleCommand is a leaf command: zero or more inline assignments
// followed by a command name and its arguments, plus any redirections.
type SimpleCommand struct {
	Assignments  []*Assignment
	Name         *Word // may be nil for assignment-only statements
	Args         []*Word
	Redirections []*Redirection
	Line         int
}

func (*SimpleCommand) nodeMarker()    {}
func (*SimpleCommand) commandMarker() {}

// Assignment is a `NAME=value` or `NAME+=value` (Append) shell
// assignment. Array is non-nil for `NAME=(elem ...)` form.
type Assignment struct {
	Name   string
	Value  *Word
	Append bool
	Array  *ArrayInit
	Line   int
}

func (*Assignment) nodeMarker() {}

// ArrayInit holds an array literal: `(a b c)` or `([k]=v ...)`.
type ArrayInit struct {
	Elements []*ArrayElement
}

// ArrayElement is one element of an array literal. Index is "" for
// positional entries.
type ArrayElement struct {
	Index string // raw expression text inside [...]
	Value *Word
}

// Word is a contiguous shell word: a sequence of parts that the
// expander joins.
type Word struct{ Parts []WordPart }

// WordPart is one piece of a Word. Closed via wordPartMarker().
type WordPart interface {
	Node
	wordPartMarker()
}

// Literal is unquoted raw text inside a word.
type Literal struct{ Value string }

// SingleQuoted holds a single-quoted string: '...'.
type SingleQuoted struct{ Value string }

// DoubleQuoted holds the inner parts of a "..." word part.
type DoubleQuoted struct{ Parts []WordPart }

// AnsiCQuoted is the $'...' bash-specific quote form with C-style
// escapes. We keep the raw inner text and let the runtime decode.
type AnsiCQuoted struct{ Value string }

// ParameterExpansion captures $var, ${var}, ${var-default}, etc.
// Parameter is the variable name (or special parameter like "?", "0",
// "@"). Operation is one of the concrete *ParamOp* types below, or nil
// for a bare ${var} reference.
type ParameterExpansion struct {
	Parameter string
	Operation ParamOp
	Indirect  bool   // ${!var}
	Length    bool   // ${#var}
	Index     string // raw text inside subscript [...]; "" if none
}

// ParamOp is the closed interface implemented by every parameter
// expansion modifier.
type ParamOp interface {
	Node
	paramOpMarker()
}

// DefaultValue: ${var-word} (or ${var:-word} when ColonNull).
type DefaultValue struct {
	Word     *Word
	ColonNull bool
}

// Assign: ${var=word} (or ${var:=word} when ColonNull).
type Assign struct {
	Word      *Word
	ColonNull bool
}

// ErrorOp: ${var?word} (or ${var:?word} when ColonNull).
type ErrorOp struct {
	Word      *Word
	ColonNull bool
}

// Alternative: ${var+word} (or ${var:+word} when ColonNull).
type Alternative struct {
	Word      *Word
	ColonNull bool
}

// SubstringRange: ${var:offset:length}. Length is "" when omitted.
type SubstringRange struct {
	Offset string
	Length string
}

// Replace: ${var/pattern/replacement} (All for //, First for /).
type Replace struct {
	Pattern     *Word
	Replacement *Word
	All         bool
}

// PatternRemove: ${var#pat} ${var##pat} ${var%pat} ${var%%pat}.
type PatternRemove struct {
	Pattern *Word
	Op      string // "#", "##", "%", "%%"
}

// CaseModify: ${var^}, ${var^^}, ${var,}, ${var,,}.
type CaseModify struct {
	Pattern *Word
	Op      string // "^", "^^", ",", ",,"
}

// Transform: ${var@op}.
type Transform struct {
	Op string // single char after '@'
}

// Names: ${!prefix*} / ${!prefix@}.
type Names struct {
	SplitWords bool // true for @, false for *
}

// Keys: ${!array[@]} or ${!array[*]}.
type Keys struct {
	SplitWords bool
}

func (*DefaultValue) nodeMarker()   {}
func (*DefaultValue) paramOpMarker() {}
func (*Assign) nodeMarker()         {}
func (*Assign) paramOpMarker()      {}
func (*ErrorOp) nodeMarker()        {}
func (*ErrorOp) paramOpMarker()     {}
func (*Alternative) nodeMarker()    {}
func (*Alternative) paramOpMarker() {}
func (*SubstringRange) nodeMarker() {}
func (*SubstringRange) paramOpMarker() {}
func (*Replace) nodeMarker()        {}
func (*Replace) paramOpMarker()     {}
func (*PatternRemove) nodeMarker()  {}
func (*PatternRemove) paramOpMarker() {}
func (*CaseModify) nodeMarker()     {}
func (*CaseModify) paramOpMarker()  {}
func (*Transform) nodeMarker()      {}
func (*Transform) paramOpMarker()   {}
func (*Names) nodeMarker()          {}
func (*Names) paramOpMarker()       {}
func (*Keys) nodeMarker()           {}
func (*Keys) paramOpMarker()        {}

// CommandSubstitution is $(...) or `...` (Backtick).
type CommandSubstitution struct {
	Body     []*Statement
	Backtick bool
}

// ArithmeticExpansion captures $((expr)). We keep the expression
// source verbatim and let the interpreter (or a runtime transform)
// evaluate it.
type ArithmeticExpansion struct {
	Expr string
}

// ProcessSubstitution captures <(cmds) and >(cmds).
type ProcessSubstitution struct {
	Direction string // "<" or ">"
	Body      []*Statement
}

// ExtGlob captures bash extended glob patterns: ?(pat), *(pat), +(pat),
// @(pat), !(pat). Op is one of "?", "*", "+", "@", "!".
type ExtGlob struct {
	Op      string
	Pattern string
}

func (*Literal) nodeMarker()             {}
func (*Literal) wordPartMarker()         {}
func (*SingleQuoted) nodeMarker()        {}
func (*SingleQuoted) wordPartMarker()    {}
func (*DoubleQuoted) nodeMarker()        {}
func (*DoubleQuoted) wordPartMarker()    {}
func (*AnsiCQuoted) nodeMarker()         {}
func (*AnsiCQuoted) wordPartMarker()     {}
func (*ParameterExpansion) nodeMarker()  {}
func (*ParameterExpansion) wordPartMarker() {}
func (*CommandSubstitution) nodeMarker() {}
func (*CommandSubstitution) wordPartMarker() {}
func (*ArithmeticExpansion) nodeMarker() {}
func (*ArithmeticExpansion) wordPartMarker() {}
func (*ProcessSubstitution) nodeMarker() {}
func (*ProcessSubstitution) wordPartMarker() {}
func (*ExtGlob) nodeMarker()             {}
func (*ExtGlob) wordPartMarker()         {}

// Redirection is a single input/output redirection attached to a
// command.
type Redirection struct {
	FD      int      // -1 when not specified
	Op      string   // ">", ">>", "<", "<<", "<<-", "<<<", "<>", ">|", "&>", "&>>", "<&", ">&"
	Word    *Word    // target word
	Heredoc *Heredoc // non-nil for <<, <<-
}

func (*Redirection) nodeMarker() {}

// Heredoc carries the body of a here-document.
type Heredoc struct {
	Tag       string
	Body      string
	StripTabs bool // true for <<-
	Expand    bool // false when tag was quoted
}

// -----------------------------------------------------------------------------
// Compound commands
// -----------------------------------------------------------------------------

// IfStmt: if/elif/else/fi.
type IfStmt struct {
	Branches []IfBranch
	Else     []*Statement
	Line     int
}

// IfBranch is one (if|elif) condition + body pair.
type IfBranch struct {
	Cond []*Statement
	Body []*Statement
}

func (*IfStmt) nodeMarker()    {}
func (*IfStmt) commandMarker() {}

// ForStmt: for x in words; do body; done.
type ForStmt struct {
	Var   string
	Words []*Word
	Body  []*Statement
	Line  int
}

func (*ForStmt) nodeMarker()    {}
func (*ForStmt) commandMarker() {}

// CStyleFor: for ((init; cond; post)); do body; done.
type CStyleFor struct {
	Init string
	Cond string
	Post string
	Body []*Statement
	Line int
}

func (*CStyleFor) nodeMarker()    {}
func (*CStyleFor) commandMarker() {}

// WhileStmt: while cond; do body; done  (Until=true → until).
type WhileStmt struct {
	Cond  []*Statement
	Body  []*Statement
	Until bool
	Line  int
}

func (*WhileStmt) nodeMarker()    {}
func (*WhileStmt) commandMarker() {}

// CaseStmt: case word in pat) ... ;; esac.
type CaseStmt struct {
	Subject *Word
	Items   []CaseItem
	Line    int
}

// CaseItem is one pattern → body branch.
type CaseItem struct {
	Patterns  []*Word
	Body      []*Statement
	Terminator string // ";;", ";&", ";;&"
}

func (*CaseStmt) nodeMarker()    {}
func (*CaseStmt) commandMarker() {}

// Subshell: (cmds).
type Subshell struct {
	Body         []*Statement
	Redirections []*Redirection
	Line         int
}

func (*Subshell) nodeMarker()    {}
func (*Subshell) commandMarker() {}

// Group: { cmds; }.
type Group struct {
	Body         []*Statement
	Redirections []*Redirection
	Line         int
}

func (*Group) nodeMarker()    {}
func (*Group) commandMarker() {}

// FunctionDef: name() { ... } or function name { ... }.
type FunctionDef struct {
	Name string
	Body Command
	Line int
}

func (*FunctionDef) nodeMarker()    {}
func (*FunctionDef) commandMarker() {}

// ArithCmd: ((expr)).
type ArithCmd struct {
	Expr string
	Line int
}

func (*ArithCmd) nodeMarker()    {}
func (*ArithCmd) commandMarker() {}

// CondCmd: [[ expr ]]. The Expr text is kept verbatim for Phase 4 — a
// proper CondExpr tree lands when we wire conditional evaluation in
// Phase 5/6.
type CondCmd struct {
	Expr string
	Line int
}

func (*CondCmd) nodeMarker()    {}
func (*CondCmd) commandMarker() {}

// DBracket is the spec's alternate spelling for CondCmd. Defined as a
// type alias for API parity with src/ast/types.ts.
type DBracket = CondCmd
