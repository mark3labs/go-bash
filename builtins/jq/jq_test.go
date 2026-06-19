package jq_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/jq"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := jq.New().Execute(context.Background(),
		append([]string{"jq"}, args...),
		&command.Context{Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestIdentity(t *testing.T) {
	out, _, code := run(t, `{"a":1}`, ".")
	if code != 0 || !strings.Contains(out, "\"a\": 1") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestCompact(t *testing.T) {
	out, _, _ := run(t, `{"a":1}`, "-c", ".")
	if out != "{\"a\":1}\n" {
		t.Errorf("got %q", out)
	}
}

func TestRawOutput(t *testing.T) {
	out, _, _ := run(t, `"hello"`, "-r", ".")
	if out != "hello\n" {
		t.Errorf("got %q", out)
	}
}

func TestRawInput(t *testing.T) {
	out, _, _ := run(t, "alpha\nbeta\n", "-R", ".")
	if out != `"alpha"
"beta"
` {
		t.Errorf("got %q", out)
	}
}

func TestSlurp(t *testing.T) {
	out, _, _ := run(t, "1 2 3", "-cs", ".")
	if out != "[1,2,3]\n" {
		t.Errorf("got %q", out)
	}
}

func TestNullInput(t *testing.T) {
	out, _, _ := run(t, "", "-n", "-c", "42")
	if out != "42\n" {
		t.Errorf("got %q", out)
	}
}

func TestArg(t *testing.T) {
	out, _, _ := run(t, "", "-n", "-r", "--arg", "name", "world", "$name")
	if out != "world\n" {
		t.Errorf("got %q", out)
	}
}

func TestArgJSON(t *testing.T) {
	out, _, _ := run(t, "", "-n", "-c", "--argjson", "x", "[1,2]", "$x")
	if out != "[1,2]\n" {
		t.Errorf("got %q", out)
	}
}

func TestRawInputSlurp(t *testing.T) {
	out, _, _ := run(t, "abc\ndef\n", "-Rs", "-r", ".")
	if out != "abc\ndef\n\n" {
		t.Errorf("got %q", out)
	}
}

func TestPipeline(t *testing.T) {
	out, _, _ := run(t, `{"a":[1,2,3]}`, "-c", ".a | length")
	if out != "3\n" {
		t.Errorf("got %q", out)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: jq") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}
