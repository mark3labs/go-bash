package gzip_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strings"
	"testing"

	gz "github.com/mark3labs/go-bash/builtins/gzip"
	"github.com/mark3labs/go-bash/builtins/gunzip"
	"github.com/mark3labs/go-bash/builtins/zcat"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func newCtx(t *testing.T, stdin string) (*command.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	fs := memfs.New()
	var stdout, stderr bytes.Buffer
	c := &command.Context{
		FS:     fs,
		Cwd:    "/",
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	return c, &stdout, &stderr
}

func runArgs(t *testing.T, c *command.Context, args ...string) int {
	t.Helper()
	res := gz.New().Execute(context.Background(), append([]string{"gzip"}, args...), c)
	return res.ExitCode
}

func TestRoundTripStdin(t *testing.T) {
	c, out, _ := newCtx(t, "hello world\n")
	if code := runArgs(t, c, "-c"); code != 0 {
		t.Fatalf("compress code=%d", code)
	}
	// Decompress what we just wrote.
	c2, out2, _ := newCtx(t, "")
	c2.Stdin = bytes.NewReader(out.Bytes())
	res := gunzip.New().Execute(context.Background(), []string{"gunzip", "-c"}, c2)
	if res.ExitCode != 0 {
		t.Fatalf("gunzip code=%d", res.ExitCode)
	}
	if got := out2.String(); got != "hello world\n" {
		t.Errorf("got %q want %q", got, "hello world\n")
	}
}

func TestCompressFileInPlace(t *testing.T) {
	c, _, _ := newCtx(t, "")
	if err := c.FS.WriteFile("/a.txt", []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runArgs(t, c, "a.txt"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := c.FS.Stat("/a.txt"); err == nil {
		t.Errorf("a.txt should have been removed")
	}
	data, err := c.FS.ReadFile("/a.txt.gz")
	if err != nil {
		t.Fatalf("read a.txt.gz: %v", err)
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := io.ReadAll(gr)
	if string(dec) != "hello\n" {
		t.Errorf("dec=%q", dec)
	}
}

func TestKeepInput(t *testing.T) {
	c, _, _ := newCtx(t, "")
	_ = c.FS.WriteFile("/a.txt", []byte("hi\n"), 0o644)
	if code := runArgs(t, c, "-k", "a.txt"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := c.FS.Stat("/a.txt"); err != nil {
		t.Errorf("a.txt should be preserved: %v", err)
	}
}

func TestDecompressFile(t *testing.T) {
	c, _, _ := newCtx(t, "")
	// Pre-seed an a.txt.gz file.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte("payload\n"))
	_ = gw.Close()
	_ = c.FS.WriteFile("/a.txt.gz", buf.Bytes(), 0o644)
	res := gunzip.New().Execute(context.Background(), []string{"gunzip", "a.txt.gz"}, c)
	if res.ExitCode != 0 {
		t.Fatalf("code=%d", res.ExitCode)
	}
	got, err := c.FS.ReadFile("/a.txt")
	if err != nil {
		t.Fatalf("a.txt: %v", err)
	}
	if string(got) != "payload\n" {
		t.Errorf("got=%q", got)
	}
}

func TestZcat(t *testing.T) {
	c, out, _ := newCtx(t, "")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte("zzz\n"))
	_ = gw.Close()
	_ = c.FS.WriteFile("/a.txt.gz", buf.Bytes(), 0o644)
	res := zcat.New().Execute(context.Background(), []string{"zcat", "a.txt.gz"}, c)
	if res.ExitCode != 0 {
		t.Fatalf("code=%d", res.ExitCode)
	}
	if out.String() != "zzz\n" {
		t.Errorf("got %q", out.String())
	}
	// File should still exist (zcat implies -k).
	if _, err := c.FS.Stat("/a.txt.gz"); err != nil {
		t.Errorf("a.txt.gz should be preserved: %v", err)
	}
}

func TestList(t *testing.T) {
	c, out, _ := newCtx(t, "")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte("hello world\n"))
	_ = gw.Close()
	_ = c.FS.WriteFile("/a.txt.gz", buf.Bytes(), 0o644)
	if code := runArgs(t, c, "-l", "a.txt.gz"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	s := out.String()
	if !strings.Contains(s, "compressed") || !strings.Contains(s, "a.txt") {
		t.Errorf("list output unexpected: %q", s)
	}
}

func TestRecursive(t *testing.T) {
	c, _, _ := newCtx(t, "")
	_ = c.FS.MkdirAll("/d", 0o755)
	_ = c.FS.WriteFile("/d/a.txt", []byte("a\n"), 0o644)
	_ = c.FS.WriteFile("/d/b.txt", []byte("b\n"), 0o644)
	if code := runArgs(t, c, "-r", "d"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := c.FS.Stat("/d/a.txt.gz"); err != nil {
		t.Errorf("a.txt.gz: %v", err)
	}
	if _, err := c.FS.Stat("/d/b.txt.gz"); err != nil {
		t.Errorf("b.txt.gz: %v", err)
	}
}

func TestLevelFlag(t *testing.T) {
	c, out, _ := newCtx(t, "lots of data lots of data lots of data\n")
	if code := runArgs(t, c, "-9", "-c"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out.Len() == 0 {
		t.Error("expected compressed output")
	}
}

func TestUseOriginalName(t *testing.T) {
	c, _, _ := newCtx(t, "")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Name = "orig.txt"
	_, _ = gw.Write([]byte("x\n"))
	_ = gw.Close()
	_ = c.FS.WriteFile("/blob.gz", buf.Bytes(), 0o644)
	res := gunzip.New().Execute(context.Background(), []string{"gunzip", "-N", "blob.gz"}, c)
	if res.ExitCode != 0 {
		t.Fatalf("code=%d", res.ExitCode)
	}
	if _, err := c.FS.Stat("/orig.txt"); err != nil {
		t.Errorf("orig.txt: %v", err)
	}
}

func TestHelp(t *testing.T) {
	c, out, _ := newCtx(t, "")
	if code := runArgs(t, c, "--help"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "Usage: gzip") {
		t.Errorf("help missing usage: %q", out.String())
	}
}

func TestUnknownOption(t *testing.T) {
	c, _, e := newCtx(t, "")
	if code := runArgs(t, c, "--bogus"); code != 2 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(e.String(), "usage:") {
		t.Errorf("stderr missing usage: %q", e.String())
	}
}

func TestBundleFlags(t *testing.T) {
	// gzip -ck < stdin
	c, out, _ := newCtx(t, "hello\n")
	if code := runArgs(t, c, "-ck"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	// Round-trip.
	c2, out2, _ := newCtx(t, "")
	c2.Stdin = bytes.NewReader(out.Bytes())
	res := zcat.New().Execute(context.Background(), []string{"zcat"}, c2)
	if res.ExitCode != 0 {
		t.Fatalf("zcat code=%d", res.ExitCode)
	}
	if out2.String() != "hello\n" {
		t.Errorf("round-trip got %q", out2.String())
	}
}
