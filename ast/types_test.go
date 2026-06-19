package ast_test

import (
	"testing"

	"github.com/mark3labs/go-bash/ast"
)

// TestNodeMarkers makes sure every exported AST node satisfies the
// closed Node / Command / WordPart / ParamOp interface.
func TestNodeMarkers(t *testing.T) {
	var _ ast.Node = (*ast.Script)(nil)
	var _ ast.Node = (*ast.Statement)(nil)
	var _ ast.Node = (*ast.Pipeline)(nil)
	var _ ast.Node = (*ast.Assignment)(nil)
	var _ ast.Node = (*ast.Redirection)(nil)

	var _ ast.Command = (*ast.SimpleCommand)(nil)
	var _ ast.Command = (*ast.IfStmt)(nil)
	var _ ast.Command = (*ast.ForStmt)(nil)
	var _ ast.Command = (*ast.CStyleFor)(nil)
	var _ ast.Command = (*ast.WhileStmt)(nil)
	var _ ast.Command = (*ast.CaseStmt)(nil)
	var _ ast.Command = (*ast.Subshell)(nil)
	var _ ast.Command = (*ast.Group)(nil)
	var _ ast.Command = (*ast.FunctionDef)(nil)
	var _ ast.Command = (*ast.ArithCmd)(nil)
	var _ ast.Command = (*ast.CondCmd)(nil)

	var _ ast.WordPart = (*ast.Literal)(nil)
	var _ ast.WordPart = (*ast.SingleQuoted)(nil)
	var _ ast.WordPart = (*ast.DoubleQuoted)(nil)
	var _ ast.WordPart = (*ast.AnsiCQuoted)(nil)
	var _ ast.WordPart = (*ast.ParameterExpansion)(nil)
	var _ ast.WordPart = (*ast.CommandSubstitution)(nil)
	var _ ast.WordPart = (*ast.ArithmeticExpansion)(nil)
	var _ ast.WordPart = (*ast.ProcessSubstitution)(nil)
	var _ ast.WordPart = (*ast.ExtGlob)(nil)

	var _ ast.ParamOp = (*ast.DefaultValue)(nil)
	var _ ast.ParamOp = (*ast.Assign)(nil)
	var _ ast.ParamOp = (*ast.ErrorOp)(nil)
	var _ ast.ParamOp = (*ast.Alternative)(nil)
	var _ ast.ParamOp = (*ast.SubstringRange)(nil)
	var _ ast.ParamOp = (*ast.Replace)(nil)
	var _ ast.ParamOp = (*ast.PatternRemove)(nil)
	var _ ast.ParamOp = (*ast.CaseModify)(nil)
	var _ ast.ParamOp = (*ast.Transform)(nil)
	var _ ast.ParamOp = (*ast.Names)(nil)
	var _ ast.ParamOp = (*ast.Keys)(nil)
}

func TestDBracketAlias(t *testing.T) {
	// Compile-time check: DBracket is an alias of CondCmd, so a
	// *CondCmd is assignable to *DBracket without an explicit
	// conversion.
	c := &ast.CondCmd{Expr: "-n $x"}
	d := alias(c)
	if d.Expr != "-n $x" {
		t.Fatalf("DBracket alias broken: got %q", d.Expr)
	}
}

// alias forces the assignability via an explicit *ast.DBracket
// parameter so the test exercises the alias rather than relying on
// type inference.
func alias(d *ast.DBracket) *ast.DBracket { return d }
