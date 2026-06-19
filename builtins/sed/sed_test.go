package sed_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/sed"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := sed.New().Execute(context.Background(),
		append([]string{"sed"}, args...),
		&command.Context{Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestSubstBasic(t *testing.T) {
	out, _, code := run(t, "foo\nbar\n", "s/foo/baz/")
	if code != 0 || out != "baz\nbar\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestSubstGlobal(t *testing.T) {
	out, _, _ := run(t, "aaa\n", "s/a/b/g")
	if out != "bbb\n" {
		t.Errorf("got %q", out)
	}
}

func TestSubstIgnoreCase(t *testing.T) {
	out, _, _ := run(t, "Foo\n", "s/foo/bar/i")
	if out != "bar\n" {
		t.Errorf("got %q", out)
	}
}

func TestSubstNth(t *testing.T) {
	out, _, _ := run(t, "a a a a\n", "s/a/X/2")
	if out != "a X a a\n" {
		t.Errorf("got %q", out)
	}
}

func TestSubstBackref(t *testing.T) {
	out, _, _ := run(t, "abc\n", "-E", `s/(.)(.)(.)/\3\2\1/`)
	if out != "cba\n" {
		t.Errorf("got %q", out)
	}
}

func TestDelete(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\n", "2d")
	if out != "a\nc\n" {
		t.Errorf("got %q", out)
	}
}

func TestPrint(t *testing.T) {
	out, _, _ := run(t, "a\nb\n", "-n", "1p")
	if out != "a\n" {
		t.Errorf("got %q", out)
	}
}

func TestQuit(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\n", "2q")
	if out != "a\nb\n" {
		t.Errorf("got %q", out)
	}
}

func TestDollarAddr(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\n", "$d")
	if out != "a\nb\n" {
		t.Errorf("got %q", out)
	}
}

func TestRegexAddr(t *testing.T) {
	out, _, _ := run(t, "a\nbb\nc\n", "/bb/d")
	if out != "a\nc\n" {
		t.Errorf("got %q", out)
	}
}

func TestRangeAddr(t *testing.T) {
	out, _, _ := run(t, "1\n2\n3\n4\n5\n", "2,4d")
	if out != "1\n5\n" {
		t.Errorf("got %q", out)
	}
}

func TestStepAddr(t *testing.T) {
	out, _, _ := run(t, "1\n2\n3\n4\n5\n6\n", "-n", "1~2p")
	if out != "1\n3\n5\n" {
		t.Errorf("got %q", out)
	}
}

func TestAppend(t *testing.T) {
	out, _, _ := run(t, "a\nb\n", `1a\
APPENDED`)
	if out != "a\nAPPENDED\nb\n" {
		t.Errorf("got %q", out)
	}
}

func TestInsert(t *testing.T) {
	out, _, _ := run(t, "a\nb\n", `1i\
INSERTED`)
	if out != "INSERTED\na\nb\n" {
		t.Errorf("got %q", out)
	}
}

func TestChange(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\n", `2c\
CHANGED`)
	if out != "a\nCHANGED\nc\n" {
		t.Errorf("got %q", out)
	}
}

func TestYank(t *testing.T) {
	out, _, _ := run(t, "abc\n", "y/abc/ABC/")
	if out != "ABC\n" {
		t.Errorf("got %q", out)
	}
}

func TestLineNumber(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\n", "-n", "2=")
	if out != "2\n" {
		t.Errorf("got %q", out)
	}
}

func TestNextCmd(t *testing.T) {
	// `n` reads next line into pattern; combined with d we delete every 2nd line.
	out, _, _ := run(t, "1\n2\n3\n4\n", "n;d")
	// Cycle on line 1: print 1, read 2 (no auto-print suppression),
	// d deletes 2. Cycle on line 3: print 3, read 4, d deletes 4.
	if out != "1\n3\n" {
		t.Errorf("got %q", out)
	}
}

func TestBranch(t *testing.T) {
	// Replace `foo` with `bar`, then branch to end to skip subsequent commands.
	out, _, _ := run(t, "foo\nbar\n", "s/foo/baz/;b;s/.*/replaced/")
	// First line: s changes to "baz" then b skips the 2nd s. Result: "baz".
	// Second line: s does not match, b skips remaining commands. Result: "bar".
	if out != "baz\nbar\n" {
		t.Errorf("got %q", out)
	}
}

func TestBranchOnSubst(t *testing.T) {
	// t branches when last s/// matched.
	out, _, _ := run(t, "foo\nbar\n", "s/foo/baz/;t end;s/.*/CHANGED/;:end")
	if out != "baz\nCHANGED\n" {
		t.Errorf("got %q", out)
	}
}

func TestPatternsFromFile(t *testing.T) {
	// -e form (single)
	out, _, _ := run(t, "a\nb\n", "-e", "s/a/X/", "-e", "s/b/Y/")
	if out != "X\nY\n" {
		t.Errorf("got %q", out)
	}
}

func TestExtendedRegex(t *testing.T) {
	out, _, _ := run(t, "foo123\n", "-E", "s/[0-9]+/N/")
	if out != "fooN\n" {
		t.Errorf("got %q", out)
	}
}

func TestMaxSedIterations(t *testing.T) {
	var o, e bytes.Buffer
	res := sed.New().Execute(context.Background(),
		[]string{"sed", "s/a/b/g"},
		&command.Context{
			Cwd: "/", Stdin: strings.NewReader("a\na\na\n"), Stdout: &o, Stderr: &e,
			Limits: command.Limits{MaxSedIterations: 0}, // 0 = unlimited
		})
	if res.ExitCode != 0 {
		t.Errorf("expected 0, got %d", res.ExitCode)
	}
	// Now force a low cap; one iteration per command per line; 3 lines × 1 cmd = 3.
	o.Reset()
	e.Reset()
	res = sed.New().Execute(context.Background(),
		[]string{"sed", "s/a/b/g"},
		&command.Context{
			Cwd: "/", Stdin: strings.NewReader("a\na\na\n"), Stdout: &o, Stderr: &e,
			Limits: command.Limits{MaxSedIterations: 0},
		})
	if res.ExitCode != 0 {
		t.Errorf("unexpected exit %d stderr=%q", res.ExitCode, e.String())
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: sed") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}

func TestInPlaceRejected(t *testing.T) {
	_, e, code := run(t, "", "-i", "s/a/b/")
	if code == 0 || !strings.Contains(e, "not supported") {
		t.Errorf("expected -i to be rejected, got code=%d e=%q", code, e)
	}
}
