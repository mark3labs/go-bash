package expr_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/expr"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := expr.New().Execute(context.Background(), append([]string{"expr"}, args...), &command.Context{Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestExprAdd(t *testing.T) {
	out, _, code := run(t, "2", "+", "3")
	if code != 0 || out != "5\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestExprSub(t *testing.T) {
	out, _, _ := run(t, "10", "-", "3")
	if out != "7\n" {
		t.Errorf("out=%q", out)
	}
}

func TestExprMul(t *testing.T) {
	out, _, _ := run(t, "4", "*", "3")
	if out != "12\n" {
		t.Errorf("out=%q", out)
	}
}

func TestExprDivAndMod(t *testing.T) {
	if out, _, _ := run(t, "10", "/", "3"); out != "3\n" {
		t.Errorf("div: %q", out)
	}
	if out, _, _ := run(t, "10", "%", "3"); out != "1\n" {
		t.Errorf("mod: %q", out)
	}
}

func TestExprDivZero(t *testing.T) {
	_, err, code := run(t, "1", "/", "0")
	if code != 2 || !strings.Contains(err, "division") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestExprPrecedence(t *testing.T) {
	out, _, _ := run(t, "2", "+", "3", "*", "4")
	if out != "14\n" {
		t.Errorf("out=%q", out)
	}
}

func TestExprParens(t *testing.T) {
	out, _, _ := run(t, "(", "2", "+", "3", ")", "*", "4")
	if out != "20\n" {
		t.Errorf("out=%q", out)
	}
}

func TestExprComparisonNumeric(t *testing.T) {
	out, _, code := run(t, "5", "=", "5")
	if out != "1\n" || code != 0 {
		t.Errorf("out=%q code=%d", out, code)
	}
	out, _, code = run(t, "4", "<", "5")
	if out != "1\n" || code != 0 {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestExprComparisonString(t *testing.T) {
	out, _, _ := run(t, "abc", "=", "abc")
	if out != "1\n" {
		t.Errorf("out=%q", out)
	}
}

func TestExprOr(t *testing.T) {
	out, _, code := run(t, "", "|", "fallback")
	if out != "fallback\n" || code != 0 {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestExprAnd(t *testing.T) {
	out, _, _ := run(t, "x", "&", "y")
	if out != "x\n" {
		t.Errorf("out=%q", out)
	}
	out, _, _ = run(t, "", "&", "y")
	if out != "0\n" {
		t.Errorf("out=%q", out)
	}
}

func TestExprMatchLength(t *testing.T) {
	out, _, code := run(t, "abcdef", ":", "abc")
	if out != "3\n" || code != 0 {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestExprMatchGroup(t *testing.T) {
	out, _, _ := run(t, "hello world", ":", `\(hello\)`)
	if out != "hello\n" {
		t.Errorf("out=%q", out)
	}
}

func TestExprMatchNoMatch(t *testing.T) {
	out, _, code := run(t, "abc", ":", "xyz")
	if out != "0\n" || code != 1 {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestExprResultZeroExits1(t *testing.T) {
	out, _, code := run(t, "1", "-", "1")
	if out != "0\n" || code != 1 {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestExprEmptyExits1(t *testing.T) {
	out, _, code := run(t, "")
	if out != "\n" || code != 1 {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestExprSyntaxError(t *testing.T) {
	_, err, code := run(t, "1", "+")
	if code != 2 || !strings.Contains(err, "expr:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestExprHelp(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: expr") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestExprMissingOperand(t *testing.T) {
	_, err, code := run(t)
	if code != 2 || !strings.Contains(err, "missing") {
		t.Errorf("err=%q code=%d", err, code)
	}
}
