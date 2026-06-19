package htmltomarkdown_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	htmd "github.com/mark3labs/go-bash/builtins/htmltomarkdown"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	if c.FS == nil {
		c.FS = memfs.New()
	}
	c.Stdout = &o
	c.Stderr = &e
	res := htmd.New().Execute(context.Background(), append([]string{"html-to-markdown"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestStdinBasic(t *testing.T) {
	c := &command.Context{
		Stdin: strings.NewReader("<h1>Hello</h1><p>world</p>"),
	}
	out, _, code := runCmd(t, c)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "# Hello") {
		t.Errorf("missing heading: %q", out)
	}
	if !strings.Contains(out, "world") {
		t.Errorf("missing body: %q", out)
	}
}

func TestFileBasic(t *testing.T) {
	fsx := memfs.New()
	_ = fsx.MkdirAll("/tmp", 0o755)
	if err := fsx.WriteFile("/tmp/page.html", []byte("<h2>Title</h2><p>body <b>bold</b></p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &command.Context{FS: fsx, Cwd: "/tmp"}
	out, _, code := runCmd(t, c, "page.html")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "## Title") {
		t.Errorf("missing heading: %q", out)
	}
	if !strings.Contains(out, "**bold**") {
		t.Errorf("missing bold: %q", out)
	}
}

func TestDashFile(t *testing.T) {
	c := &command.Context{
		Stdin: strings.NewReader("<p>via dash</p>"),
	}
	out, _, code := runCmd(t, c, "-")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "via dash") {
		t.Errorf("out=%q", out)
	}
}

func TestMissingFile(t *testing.T) {
	c := &command.Context{FS: memfs.New(), Cwd: "/"}
	_, e, code := runCmd(t, c, "/nope.html")
	if code != 1 {
		t.Errorf("code=%d", code)
	}
	if !strings.Contains(e, "html-to-markdown:") {
		t.Errorf("stderr=%q", e)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: html-to-markdown") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestDashDash(t *testing.T) {
	fsx := memfs.New()
	_ = fsx.MkdirAll("/", 0o755)
	_ = fsx.WriteFile("/page.html", []byte("<em>x</em>"), 0o644)
	c := &command.Context{FS: fsx, Cwd: "/"}
	out, _, code := runCmd(t, c, "--", "page.html")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "_x_") && !strings.Contains(out, "*x*") {
		t.Errorf("out=%q", out)
	}
}

func TestEmptyStdin(t *testing.T) {
	c := &command.Context{Stdin: strings.NewReader("")}
	out, _, code := runCmd(t, c)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "" {
		t.Errorf("out=%q want empty", out)
	}
}
