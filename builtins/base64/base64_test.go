package base64_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/base64"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := base64.New().Execute(context.Background(), append([]string{"base64"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestEncode(t *testing.T) {
	out, _, _ := run(t, "hello")
	if !strings.HasPrefix(out, "aGVsbG8=") {
		t.Errorf("got %q", out)
	}
}

func TestDecode(t *testing.T) {
	out, _, _ := run(t, "aGVsbG8=", "-d")
	if out != "hello" {
		t.Errorf("got %q", out)
	}
}

func TestRoundtrip(t *testing.T) {
	enc, _, _ := run(t, "the quick brown fox")
	enc = strings.TrimRight(enc, "\n")
	out, _, _ := run(t, enc, "-d")
	if out != "the quick brown fox" {
		t.Errorf("roundtrip got %q", out)
	}
}

func TestWrap(t *testing.T) {
	long := strings.Repeat("A", 100)
	out, _, _ := run(t, long, "-w", "16")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for _, l := range lines[:len(lines)-1] {
		if len(l) != 16 {
			t.Errorf("expected width 16, got %d (%q)", len(l), l)
		}
	}
}

func TestWrapZero(t *testing.T) {
	long := strings.Repeat("A", 100)
	out, _, _ := run(t, long, "-w", "0")
	if strings.Count(out, "\n") != 1 {
		t.Errorf("expected single line: %q", out)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: base64") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
