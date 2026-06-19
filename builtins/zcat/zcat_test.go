package zcat_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/zcat"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin []byte, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := zcat.New().Execute(context.Background(),
		append([]string{"zcat"}, args...),
		&command.Context{Cwd: "/", Stdin: bytes.NewReader(stdin), Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(s))
	_ = gw.Close()
	return buf.Bytes()
}

func TestZcatStdin(t *testing.T) {
	out, _, code := run(t, gzipBytes(t, "abc\n"))
	if code != 0 || out != "abc\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: zcat") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, nil, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}
