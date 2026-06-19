package tar_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"strings"
	"testing"

	gbtar "github.com/mark3labs/go-bash/builtins/tar"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func newCtx(t *testing.T) (*command.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	fs := memfs.New()
	var stdout, stderr bytes.Buffer
	c := &command.Context{
		FS:     fs,
		Cwd:    "/",
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	return c, &stdout, &stderr
}

func runArgs(t *testing.T, c *command.Context, args ...string) int {
	t.Helper()
	res := gbtar.New().Execute(context.Background(), append([]string{"tar"}, args...), c)
	return res.ExitCode
}

func TestCreateAndList(t *testing.T) {
	c, out, _ := newCtx(t)
	_ = c.FS.MkdirAll("/src", 0o755)
	_ = c.FS.WriteFile("/src/a.txt", []byte("AAA\n"), 0o644)
	_ = c.FS.WriteFile("/src/b.txt", []byte("BBB\n"), 0o644)
	if code := runArgs(t, c, "-cf", "out.tar", "src"); code != 0 {
		t.Fatalf("create code=%d", code)
	}
	c2, out2, _ := newCtx(t)
	data, err := c.FS.ReadFile("/out.tar")
	if err != nil {
		t.Fatal(err)
	}
	_ = c2.FS.WriteFile("/out.tar", data, 0o644)
	if code := runArgs(t, c2, "-tf", "out.tar"); code != 0 {
		t.Fatalf("list code=%d stderr=%s", code, out.String())
	}
	got := out2.String()
	if !strings.Contains(got, "src/a.txt") || !strings.Contains(got, "src/b.txt") {
		t.Errorf("list missing entries: %q", got)
	}
}

func TestCreateExtractRoundTrip(t *testing.T) {
	c, _, _ := newCtx(t)
	_ = c.FS.MkdirAll("/src", 0o755)
	_ = c.FS.WriteFile("/src/a.txt", []byte("AAA\n"), 0o644)
	if code := runArgs(t, c, "-cf", "out.tar", "src"); code != 0 {
		t.Fatalf("create code=%d", code)
	}
	c2, _, _ := newCtx(t)
	data, _ := c.FS.ReadFile("/out.tar")
	_ = c2.FS.WriteFile("/out.tar", data, 0o644)
	if code := runArgs(t, c2, "-xf", "out.tar"); code != 0 {
		t.Fatalf("extract code=%d", code)
	}
	got, err := c2.FS.ReadFile("/src/a.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "AAA\n" {
		t.Errorf("got=%q", got)
	}
}

func TestGzipMode(t *testing.T) {
	c, _, _ := newCtx(t)
	_ = c.FS.WriteFile("/a.txt", []byte("payload\n"), 0o644)
	if code := runArgs(t, c, "-czf", "out.tar.gz", "a.txt"); code != 0 {
		t.Fatalf("create code=%d", code)
	}
	data, _ := c.FS.ReadFile("/out.tar.gz")
	// First two bytes are gzip magic.
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		t.Errorf("not gzip-compressed: %x", data[:min(2, len(data))])
	}
	c2, _, _ := newCtx(t)
	_ = c2.FS.WriteFile("/out.tar.gz", data, 0o644)
	if code := runArgs(t, c2, "-xzf", "out.tar.gz"); code != 0 {
		t.Fatalf("extract code=%d", code)
	}
	got, _ := c2.FS.ReadFile("/a.txt")
	if string(got) != "payload\n" {
		t.Errorf("got=%q", got)
	}
}

func TestStripComponents(t *testing.T) {
	// Build a tar by hand containing a/b/file.txt
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "a/b/file.txt", Typeflag: tar.TypeReg, Size: 5, Mode: 0o644})
	_, _ = tw.Write([]byte("hello"))
	_ = tw.Close()

	c, _, _ := newCtx(t)
	_ = c.FS.WriteFile("/in.tar", buf.Bytes(), 0o644)
	if code := runArgs(t, c, "--strip-components", "2", "-xf", "in.tar"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	got, err := c.FS.ReadFile("/file.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got=%q", got)
	}
}

func TestListVerbose(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "x.txt", Typeflag: tar.TypeReg, Size: 3, Mode: 0o644})
	_, _ = tw.Write([]byte("xxx"))
	_ = tw.Close()

	c, out, _ := newCtx(t)
	_ = c.FS.WriteFile("/a.tar", buf.Bytes(), 0o644)
	if code := runArgs(t, c, "-tvf", "a.tar"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "x.txt") || !strings.Contains(out.String(), "rw-") {
		t.Errorf("verbose missing details: %q", out.String())
	}
}

func TestMaxStringLength(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "big.txt", Typeflag: tar.TypeReg, Size: 1024, Mode: 0o644})
	_, _ = tw.Write(bytes.Repeat([]byte{'A'}, 1024))
	_ = tw.Close()

	c, _, e := newCtx(t)
	c.Limits.MaxStringLength = 128
	_ = c.FS.WriteFile("/big.tar", buf.Bytes(), 0o644)
	if code := runArgs(t, c, "-tf", "big.tar"); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(e.String(), "MaxStringLength") {
		t.Errorf("stderr missing diag: %q", e.String())
	}
}

func TestGzipStream(t *testing.T) {
	// Verify --gzip works as a long flag and reads from stdin.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "y.txt", Typeflag: tar.TypeReg, Size: 2, Mode: 0o644})
	_, _ = tw.Write([]byte("yy"))
	_ = tw.Close()
	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	_, _ = gw.Write(buf.Bytes())
	_ = gw.Close()

	c, out, _ := newCtx(t)
	c.Stdin = bytes.NewReader(gz.Bytes())
	if code := runArgs(t, c, "-t", "--gzip"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "y.txt") {
		t.Errorf("got %q", out.String())
	}
}

func TestNoMode(t *testing.T) {
	c, _, e := newCtx(t)
	if code := runArgs(t, c, "-f", "x.tar"); code != 2 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(e.String(), "one of -c") {
		t.Errorf("got %q", e.String())
	}
}

func TestHelp(t *testing.T) {
	c, out, _ := newCtx(t)
	if code := runArgs(t, c, "--help"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "Usage: tar") {
		t.Errorf("missing usage: %q", out.String())
	}
}

func TestUnknownOption(t *testing.T) {
	c, _, e := newCtx(t)
	if code := runArgs(t, c, "--bogus"); code != 2 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(e.String(), "usage:") {
		t.Errorf("stderr missing usage: %q", e.String())
	}
}

func TestChdir(t *testing.T) {
	c, _, _ := newCtx(t)
	_ = c.FS.MkdirAll("/work", 0o755)
	_ = c.FS.WriteFile("/work/a.txt", []byte("a\n"), 0o644)
	if code := runArgs(t, c, "-C", "work", "-cf", "/out.tar", "a.txt"); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if _, err := c.FS.Stat("/out.tar"); err != nil {
		t.Errorf("expected /out.tar: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
