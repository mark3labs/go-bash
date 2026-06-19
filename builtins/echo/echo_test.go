package echo_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/echo"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	res := echo.New().Execute(context.Background(), append([]string{"echo"}, args...), &command.Context{Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestEchoBasic(t *testing.T) {
	out, _, code := run(t, "hello", "world")
	if code != 0 || out != "hello world\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestEchoNoArgs(t *testing.T) {
	out, _, code := run(t)
	if code != 0 || out != "\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestEchoMinusN(t *testing.T) {
	out, _, code := run(t, "-n", "no-newline")
	if code != 0 || out != "no-newline" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestEchoMinusE(t *testing.T) {
	out, _, _ := run(t, "-e", `a\tb\nc`)
	if out != "a\tb\nc\n" {
		t.Errorf("out=%q", out)
	}
}

func TestEchoMinusBigE(t *testing.T) {
	out, _, _ := run(t, "-E", `a\tb`)
	if out != `a\tb`+"\n" {
		t.Errorf("out=%q", out)
	}
}

func TestEchoCombinedFlags(t *testing.T) {
	out, _, _ := run(t, "-ne", `a\tb`)
	if out != "a\tb" {
		t.Errorf("out=%q", out)
	}
}

func TestEchoEscapeC(t *testing.T) {
	// -e with \c terminates output and suppresses newline.
	out, _, _ := run(t, "-e", `before\cafter`)
	if out != "before" {
		t.Errorf("out=%q", out)
	}
}

func TestEchoEscapeHex(t *testing.T) {
	out, _, _ := run(t, "-e", `\x41\x42`)
	if out != "AB\n" {
		t.Errorf("out=%q", out)
	}
}

func TestEchoEscapeOctal(t *testing.T) {
	out, _, _ := run(t, "-e", `\0101`)
	if out != "A\n" {
		t.Errorf("out=%q", out)
	}
}

func TestEchoNonFlagLooksLikeFlag(t *testing.T) {
	// -x is not a recognized flag; bash treats it as a literal.
	out, _, _ := run(t, "-x", "y")
	if out != "-x y\n" {
		t.Errorf("out=%q", out)
	}
}

func TestEchoHelp(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: echo") {
		t.Errorf("help missing or wrong code: out=%q code=%d", out, code)
	}
}
