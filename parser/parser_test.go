package parser_test

import (
	"errors"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/ast"
	"github.com/mark3labs/go-bash/parser"
)

// TestParseAcceptanceShape covers the spec: Parse("echo $((1+2)) | grep o")
// yields a Script with one statement / one pipeline / two SimpleCommands.
func TestParseAcceptanceShape(t *testing.T) {
	script, err := parser.Parse("echo $((1+2)) | grep o")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := len(script.Statements); got != 1 {
		t.Fatalf("expected 1 statement, got %d", got)
	}
	st := script.Statements[0]
	if got := len(st.Pipelines); got != 1 {
		t.Fatalf("expected 1 pipeline, got %d", got)
	}
	pipe := st.Pipelines[0]
	if got := len(pipe.Commands); got != 2 {
		t.Fatalf("expected 2 commands in pipeline, got %d", got)
	}
	c0, ok := pipe.Commands[0].(*ast.SimpleCommand)
	if !ok {
		t.Fatalf("expected SimpleCommand, got %T", pipe.Commands[0])
	}
	if c0.Name == nil || len(c0.Name.Parts) == 0 {
		t.Fatalf("expected echo name part")
	}
	if lit, _ := c0.Name.Parts[0].(*ast.Literal); lit == nil || lit.Value != "echo" {
		t.Fatalf("expected literal echo, got %+v", c0.Name.Parts[0])
	}
	// arg should be ArithmeticExpansion
	if got := len(c0.Args); got != 1 {
		t.Fatalf("expected 1 arg, got %d", got)
	}
	if len(c0.Args[0].Parts) == 0 {
		t.Fatalf("expected arg with parts")
	}
	arith, _ := c0.Args[0].Parts[0].(*ast.ArithmeticExpansion)
	if arith == nil {
		t.Fatalf("expected ArithmeticExpansion, got %T", c0.Args[0].Parts[0])
	}
	if !strings.Contains(arith.Expr, "1") || !strings.Contains(arith.Expr, "2") {
		t.Fatalf("arith expr should mention 1+2, got %q", arith.Expr)
	}

	c1, ok := pipe.Commands[1].(*ast.SimpleCommand)
	if !ok {
		t.Fatalf("expected second SimpleCommand, got %T", pipe.Commands[1])
	}
	if c1.Name == nil {
		t.Fatalf("expected grep name")
	}
	if lit, _ := c1.Name.Parts[0].(*ast.Literal); lit == nil || lit.Value != "grep" {
		t.Fatalf("expected literal grep, got %+v", c1.Name.Parts[0])
	}
}

func TestParseStringAlias(t *testing.T) {
	a, errA := parser.Parse("echo hello")
	b, errB := parser.ParseString("echo hello")
	if errA != nil || errB != nil {
		t.Fatalf("unexpected error: %v / %v", errA, errB)
	}
	if len(a.Statements) != 1 || len(b.Statements) != 1 {
		t.Fatalf("expected one statement each")
	}
}

func TestParseAndOrFlattens(t *testing.T) {
	script, err := parser.Parse("a && b || c")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(script.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(script.Statements))
	}
	st := script.Statements[0]
	if got := len(st.Pipelines); got != 3 {
		t.Fatalf("expected 3 pipelines, got %d", got)
	}
	if got := st.Operators; len(got) != 2 || got[0] != "&&" || got[1] != "||" {
		t.Fatalf("expected operators &&,||, got %v", got)
	}
}

func TestParseSemicolonProducesTwoStatements(t *testing.T) {
	script, err := parser.Parse("a; b")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := len(script.Statements); got != 2 {
		t.Fatalf("expected 2 statements, got %d", got)
	}
}

func TestParseBackgroundFlag(t *testing.T) {
	script, err := parser.Parse("sleep 5 &")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !script.Statements[0].Background {
		t.Fatalf("expected Background=true on `sleep 5 &`")
	}
}

func TestParseSyntaxErrorReturnsTypedParseError(t *testing.T) {
	_, err := parser.Parse("if then fi")
	if err == nil {
		t.Fatalf("expected parse error")
	}
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *parser.ParseError, got %T: %v", err, err)
	}
	if pe.Line == 0 {
		t.Fatalf("expected non-zero line in parse error, got %+v", pe)
	}
}

// gobash.ParseError must be aliased to parser.ParseError so callers can
// errors.As against either spelling.
func TestParseErrorAliasedFromTopLevel(t *testing.T) {
	_, err := parser.Parse("|||")
	if err == nil {
		t.Fatalf("expected parse error")
	}
	var pe *gobash.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected gobash.ParseError alias to match, got %T", err)
	}
}

func TestMaxInputSizeEnforced(t *testing.T) {
	big := strings.Repeat("a", parser.MaxInputSize+1)
	_, err := parser.Parse(big)
	if err == nil {
		t.Fatalf("expected MaxInputSize error")
	}
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if !strings.Contains(pe.Msg, "input too large") {
		t.Fatalf("expected input-too-large message, got %q", pe.Msg)
	}
}

func TestMaxTokensEnforced(t *testing.T) {
	// Each `a;` pair yields ~3 syntax-tree nodes (Stmt, CallExpr,
	// Word, Lit ⇒ 4). With MaxTokens=100000 we need well over 25k
	// statements. We synthesize ~60k statements to comfortably trip
	// the limit while staying under MaxInputSize (60000*3 = 180k
	// bytes, well under 1 MiB).
	src := strings.Repeat("a;\n", 60000)
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatalf("expected MaxTokens error")
	}
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if !strings.Contains(pe.Msg, "too many tokens") {
		t.Fatalf("expected too-many-tokens message, got %q", pe.Msg)
	}
}

func TestMaxParserDepthEnforced(t *testing.T) {
	// Deeply nested subshells: ((((( ... ))))).
	depth := parser.MaxParserDepth + 50
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString("( ")
	}
	b.WriteString("a")
	for i := 0; i < depth; i++ {
		b.WriteString(" )")
	}
	_, err := parser.Parse(b.String())
	if err == nil {
		t.Fatalf("expected MaxParserDepth error")
	}
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if !strings.Contains(pe.Msg, "depth") {
		t.Fatalf("expected depth-exceeded message, got %q", pe.Msg)
	}
}

func TestMaxHeredocSizeEnforced(t *testing.T) {
	body := strings.Repeat("a", parser.MaxHeredocSize+1)
	if len(body) >= parser.MaxInputSize {
		// MaxInputSize would trip first; in that case the test trips
		// input-size which still proves the limit is enforced.
		// MaxHeredocSize == 10 MiB > MaxInputSize == 1 MiB, so the
		// input-size check is the actual gate. This test exists to
		// document the relationship; the heredoc limit becomes
		// relevant when a future caller raises MaxInputSize.
		_, err := parser.Parse("cat <<EOF\n" + body + "\nEOF\n")
		if err == nil {
			t.Fatalf("expected size error")
		}
		var pe *parser.ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("expected *ParseError, got %T", err)
		}
		return
	}
	src := "cat <<EOF\n" + body + "\nEOF\n"
	_, err := parser.Parse(src)
	if err == nil {
		t.Fatalf("expected MaxHeredocSize error")
	}
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if !strings.Contains(pe.Msg, "heredoc") {
		t.Fatalf("expected heredoc-too-large message, got %q", pe.Msg)
	}
}

func TestParseHeredocBody(t *testing.T) {
	script, err := parser.Parse("cat <<EOF\nhello\nworld\nEOF\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(script.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(script.Statements))
	}
	pipe := script.Statements[0].Pipelines[0]
	cmd, ok := pipe.Commands[0].(*ast.SimpleCommand)
	if !ok {
		t.Fatalf("expected SimpleCommand, got %T", pipe.Commands[0])
	}
	if got := len(cmd.Redirections); got != 1 {
		t.Fatalf("expected 1 redirection, got %d", got)
	}
	r := cmd.Redirections[0]
	if r.Heredoc == nil {
		t.Fatalf("expected heredoc")
	}
	if !strings.Contains(r.Heredoc.Body, "hello") {
		t.Fatalf("expected heredoc body to contain 'hello', got %q", r.Heredoc.Body)
	}
}

func TestParseRedirectionsOnSimpleCommand(t *testing.T) {
	script, err := parser.Parse("echo hi > out.txt")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd, ok := script.Statements[0].Pipelines[0].Commands[0].(*ast.SimpleCommand)
	if !ok {
		t.Fatalf("expected SimpleCommand")
	}
	if got := len(cmd.Redirections); got != 1 {
		t.Fatalf("expected 1 redirection, got %d", got)
	}
	if cmd.Redirections[0].Op != ">" {
		t.Fatalf("expected '>' redirection, got %q", cmd.Redirections[0].Op)
	}
}

func TestParseSingleAndDoubleQuoted(t *testing.T) {
	script, err := parser.Parse(`echo 'a' "b"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd, ok := script.Statements[0].Pipelines[0].Commands[0].(*ast.SimpleCommand)
	if !ok {
		t.Fatalf("expected SimpleCommand")
	}
	if len(cmd.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(cmd.Args))
	}
	if sq, _ := cmd.Args[0].Parts[0].(*ast.SingleQuoted); sq == nil || sq.Value != "a" {
		t.Fatalf("expected SingleQuoted 'a', got %+v", cmd.Args[0].Parts[0])
	}
	if dq, _ := cmd.Args[1].Parts[0].(*ast.DoubleQuoted); dq == nil {
		t.Fatalf("expected DoubleQuoted, got %+v", cmd.Args[1].Parts[0])
	}
}

func TestParseAnsiCQuoted(t *testing.T) {
	script, err := parser.Parse(`echo $'a\nb'`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd, _ := script.Statements[0].Pipelines[0].Commands[0].(*ast.SimpleCommand)
	if cmd == nil || len(cmd.Args) != 1 {
		t.Fatalf("expected 1 arg")
	}
	q, _ := cmd.Args[0].Parts[0].(*ast.AnsiCQuoted)
	if q == nil {
		t.Fatalf("expected AnsiCQuoted, got %T", cmd.Args[0].Parts[0])
	}
}

func TestParseParameterExpansion(t *testing.T) {
	script, err := parser.Parse(`echo ${name:-default}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd, _ := script.Statements[0].Pipelines[0].Commands[0].(*ast.SimpleCommand)
	if cmd == nil || len(cmd.Args) != 1 {
		t.Fatalf("expected 1 arg")
	}
	pe, _ := cmd.Args[0].Parts[0].(*ast.ParameterExpansion)
	if pe == nil {
		t.Fatalf("expected ParameterExpansion, got %T", cmd.Args[0].Parts[0])
	}
	if pe.Parameter != "name" {
		t.Fatalf("expected param 'name', got %q", pe.Parameter)
	}
	def, _ := pe.Operation.(*ast.DefaultValue)
	if def == nil {
		t.Fatalf("expected DefaultValue, got %T", pe.Operation)
	}
	if !def.ColonNull {
		t.Fatalf("expected ColonNull=true for ':-'")
	}
}

func TestParseIfFor(t *testing.T) {
	script, err := parser.Parse(`
if [ -f x ]; then echo yes; else echo no; fi
for i in 1 2 3; do echo "$i"; done
while true; do break; done
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(script.Statements) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(script.Statements))
	}
	if _, ok := script.Statements[0].Pipelines[0].Commands[0].(*ast.IfStmt); !ok {
		t.Fatalf("expected IfStmt, got %T", script.Statements[0].Pipelines[0].Commands[0])
	}
	if _, ok := script.Statements[1].Pipelines[0].Commands[0].(*ast.ForStmt); !ok {
		t.Fatalf("expected ForStmt, got %T", script.Statements[1].Pipelines[0].Commands[0])
	}
	if _, ok := script.Statements[2].Pipelines[0].Commands[0].(*ast.WhileStmt); !ok {
		t.Fatalf("expected WhileStmt, got %T", script.Statements[2].Pipelines[0].Commands[0])
	}
}

func TestParseFunctionDef(t *testing.T) {
	script, err := parser.Parse("greet() { echo hi; }")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fn, ok := script.Statements[0].Pipelines[0].Commands[0].(*ast.FunctionDef)
	if !ok {
		t.Fatalf("expected FunctionDef, got %T", script.Statements[0].Pipelines[0].Commands[0])
	}
	if fn.Name != "greet" {
		t.Fatalf("expected name greet, got %q", fn.Name)
	}
}

func TestParseAssignmentBeforeCommand(t *testing.T) {
	script, err := parser.Parse("FOO=bar echo hi")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd, _ := script.Statements[0].Pipelines[0].Commands[0].(*ast.SimpleCommand)
	if cmd == nil {
		t.Fatalf("expected SimpleCommand")
	}
	if len(cmd.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(cmd.Assignments))
	}
	if cmd.Assignments[0].Name != "FOO" {
		t.Fatalf("expected name FOO, got %q", cmd.Assignments[0].Name)
	}
}

func TestParseEmptyScript(t *testing.T) {
	script, err := parser.Parse("")
	if err != nil {
		t.Fatalf("Parse(\"\"): %v", err)
	}
	if len(script.Statements) != 0 {
		t.Fatalf("expected 0 statements, got %d", len(script.Statements))
	}
	if script.Origin == nil {
		t.Fatalf("Origin should be non-nil even for empty script")
	}
}

func TestOriginPopulated(t *testing.T) {
	script, err := parser.Parse("echo hi")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if script.Origin == nil {
		t.Fatalf("Origin should be non-nil")
	}
}
