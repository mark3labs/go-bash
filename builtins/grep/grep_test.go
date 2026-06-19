package grep_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/egrep"
	"github.com/mark3labs/go-bash/builtins/fgrep"
	"github.com/mark3labs/go-bash/builtins/grep"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runWith(t *testing.T, stdin string, files map[string]string, args ...string) (string, string, int) {
	t.Helper()
	fs := memfs.New()
	for p, content := range files {
		if err := fs.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	var o, e bytes.Buffer
	res := grep.New().Execute(context.Background(), append([]string{"grep"}, args...), &command.Context{
		FS: fs, Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	return runWith(t, stdin, nil, args...)
}

func TestBasic(t *testing.T) {
	out, _, code := run(t, "foo\nbar\nfoobar\n", "foo")
	if code != 0 || out != "foo\nfoobar\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestNoMatch(t *testing.T) {
	out, _, code := run(t, "foo\nbar\n", "zzz")
	if code != 1 || out != "" {
		t.Errorf("expected exit 1 empty, got out=%q code=%d", out, code)
	}
}

func TestInvert(t *testing.T) {
	out, _, code := run(t, "foo\nbar\nbaz\n", "-v", "foo")
	if code != 0 || out != "bar\nbaz\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestIgnoreCase(t *testing.T) {
	out, _, code := run(t, "FOO\nbar\n", "-i", "foo")
	if code != 0 || out != "FOO\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestLineNumber(t *testing.T) {
	out, _, _ := run(t, "a\nb\nc\n", "-n", "b")
	if out != "2:b\n" {
		t.Errorf("got %q", out)
	}
}

func TestCount(t *testing.T) {
	out, _, _ := run(t, "a\na\nb\n", "-c", "a")
	if out != "2\n" {
		t.Errorf("got %q", out)
	}
}

func TestExtended(t *testing.T) {
	out, _, _ := run(t, "foo\nbar\nbaz\n", "-E", "ba(r|z)")
	if out != "bar\nbaz\n" {
		t.Errorf("got %q", out)
	}
}

func TestFixed(t *testing.T) {
	out, _, _ := run(t, "a.b\naxb\n", "-F", "a.b")
	if out != "a.b\n" {
		t.Errorf("got %q", out)
	}
}

func TestEgrepDefault(t *testing.T) {
	var o, e bytes.Buffer
	res := egrep.New().Execute(context.Background(),
		[]string{"egrep", "ba(r|z)"},
		&command.Context{Cwd: "/", Stdin: strings.NewReader("foo\nbar\nbaz\n"), Stdout: &o, Stderr: &e})
	if res.ExitCode != 0 || o.String() != "bar\nbaz\n" {
		t.Errorf("egrep: out=%q exit=%d", o.String(), res.ExitCode)
	}
}

func TestFgrepDefault(t *testing.T) {
	var o, e bytes.Buffer
	res := fgrep.New().Execute(context.Background(),
		[]string{"fgrep", "."},
		&command.Context{Cwd: "/", Stdin: strings.NewReader("abc\na.b\n"), Stdout: &o, Stderr: &e})
	if res.ExitCode != 0 || o.String() != "a.b\n" {
		t.Errorf("fgrep: out=%q exit=%d", o.String(), res.ExitCode)
	}
}

func TestOnlyMatching(t *testing.T) {
	out, _, _ := run(t, "foo123 bar456\n", "-oE", "[0-9]+")
	if out != "123\n456\n" {
		t.Errorf("got %q", out)
	}
}

func TestWordRegexp(t *testing.T) {
	out, _, _ := run(t, "foo\nfoobar\nfoo bar\n", "-w", "foo")
	if out != "foo\nfoo bar\n" {
		t.Errorf("got %q", out)
	}
}

func TestLineRegexp(t *testing.T) {
	out, _, _ := run(t, "foo\nfoobar\n", "-x", "foo")
	if out != "foo\n" {
		t.Errorf("got %q", out)
	}
}

func TestQuiet(t *testing.T) {
	out, _, code := run(t, "foo\nbar\n", "-q", "foo")
	if code != 0 || out != "" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestMultipleFiles(t *testing.T) {
	files := map[string]string{"/a.txt": "alpha\n", "/b.txt": "beta\nalpha\n"}
	out, _, _ := runWith(t, "", files, "alpha", "/b.txt", "/a.txt")
	// Filename-sorted output.
	want := "/a.txt:alpha\n/b.txt:alpha\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestRecursive(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("/d/sub", 0o755)
	_ = fs.WriteFile("/d/a.txt", []byte("alpha\n"), 0o644)
	_ = fs.WriteFile("/d/sub/b.txt", []byte("beta alpha\n"), 0o644)
	var o, e bytes.Buffer
	res := grep.New().Execute(context.Background(),
		[]string{"grep", "-r", "alpha", "/d"},
		&command.Context{FS: fs, Cwd: "/", Stdin: strings.NewReader(""), Stdout: &o, Stderr: &e})
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, e.String())
	}
	out := o.String()
	if !strings.Contains(out, "/d/a.txt:alpha") || !strings.Contains(out, "/d/sub/b.txt:beta alpha") {
		t.Errorf("got %q", out)
	}
}

func TestContext(t *testing.T) {
	out, _, _ := run(t, "1\n2\n3\nMATCH\n5\n6\n7\n", "-C", "1", "MATCH")
	want := "3\nMATCH\n5\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestPatternFile(t *testing.T) {
	files := map[string]string{"/pats": "foo\nbar\n"}
	out, _, _ := runWith(t, "alpha\nfoo\nbar\nbaz\n", files, "-f", "/pats")
	if out != "foo\nbar\n" {
		t.Errorf("got %q", out)
	}
}

func TestFilesWithMatch(t *testing.T) {
	files := map[string]string{"/a.txt": "alpha\n", "/b.txt": "beta\n"}
	out, _, _ := runWith(t, "", files, "-l", "alpha", "/a.txt", "/b.txt")
	if out != "/a.txt\n" {
		t.Errorf("got %q", out)
	}
}

func TestFilesWithoutMatch(t *testing.T) {
	files := map[string]string{"/a.txt": "alpha\n", "/b.txt": "beta\n"}
	out, _, _ := runWith(t, "", files, "-L", "alpha", "/a.txt", "/b.txt")
	if out != "/b.txt\n" {
		t.Errorf("got %q", out)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: grep") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
