package gunzip_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/gunzip"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin []byte, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := gunzip.New().Execute(context.Background(),
		append([]string{"gunzip"}, args...),
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

func TestDecompressStdin(t *testing.T) {
	out, _, code := run(t, gzipBytes(t, "hello\n"), "-c")
	if code != 0 || out != "hello\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: gunzip") {
		t.Errorf("help: out=%q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, nil, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}
