package printf_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/printf"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := printf.New().Execute(context.Background(), append([]string{"printf"}, args...), &command.Context{Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestPrintfPlainString(t *testing.T) {
	out, _, code := run(t, "hello\n")
	if code != 0 || out != "hello\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestPrintfPercentS(t *testing.T) {
	out, _, _ := run(t, "%s\n", "go")
	if out != "go\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfFormatReuse(t *testing.T) {
	out, _, _ := run(t, "%s\n", "a", "b", "c")
	if out != "a\nb\nc\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfPercentD(t *testing.T) {
	out, _, _ := run(t, "%d\n", "42")
	if out != "42\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfPercentX(t *testing.T) {
	out, _, _ := run(t, "%x\n", "255")
	if out != "ff\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfWidthPrecision(t *testing.T) {
	out, _, _ := run(t, "%5.2f\n", "3.14159")
	if out != " 3.14\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfPercentB(t *testing.T) {
	out, _, _ := run(t, "%b", `a\tb\n`)
	if out != "a\tb\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfPercentQ(t *testing.T) {
	out, _, _ := run(t, "%q\n", "with space")
	if !strings.Contains(out, "'with space'") {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfPercentQSingleQuote(t *testing.T) {
	out, _, _ := run(t, "%q\n", `it's`)
	if !strings.Contains(out, `'it'\''s'`) {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfPercentPercent(t *testing.T) {
	out, _, _ := run(t, "%%\n")
	if out != "%\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfBackslashEscape(t *testing.T) {
	out, _, _ := run(t, `\t\n`)
	if out != "\t\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfSingleCharApostrophe(t *testing.T) {
	out, _, _ := run(t, "%d\n", "'A")
	if out != "65\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfStrftime(t *testing.T) {
	out, _, _ := run(t, "%(%Y)T\n", "0")
	// Unix epoch year is 1970.
	if out != "1970\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfMissingFormat(t *testing.T) {
	_, errOut, code := run(t)
	if code != 2 {
		t.Errorf("code = %d; want 2", code)
	}
	if !strings.Contains(errOut, "usage:") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestPrintfHelp(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: printf") {
		t.Errorf("help wrong: out=%q code=%d", out, code)
	}
}

func TestPrintfFewerArgsThanFormat(t *testing.T) {
	// "%s-%s" with one arg gives "arg-" (missing arg → "").
	out, _, _ := run(t, "%s-%s", "x")
	if out != "x-" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfPercentPercentNoInfiniteLoop(t *testing.T) {
	// Format "%%" consumes no args; the cycling loop must not spin.
	out, _, code := run(t, "%%", "extra", "args")
	if code != 0 || out != "%" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestPrintfStrftimeNow(t *testing.T) {
	// "-1" => time.Now(). Just check the output looks year-shaped.
	out, _, code := run(t, "%(%Y)T\n", "-1")
	if code != 0 || len(out) != 5 {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestPrintfShellQuoteEmpty(t *testing.T) {
	out, _, _ := run(t, "%q\n", "")
	if out != "''\n" {
		t.Errorf("out=%q", out)
	}
}

func TestPrintfShellQuoteControlChars(t *testing.T) {
	out, _, _ := run(t, "%q\n", "tab\there")
	if !strings.HasPrefix(out, "$'") {
		t.Errorf("control-char %%q should use $'...': %q", out)
	}
}
