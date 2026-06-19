package md5sum_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/md5sum"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := md5sum.New().Execute(context.Background(), append([]string{"md5sum"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestSum(t *testing.T) {
	out, _, _ := run(t, "hello")
	if !strings.HasPrefix(out, "5d41402abc4b2a76b9719d911017c592") {
		t.Errorf("got %q", out)
	}
	if !strings.Contains(out, "  -") {
		t.Errorf("expected two spaces before name: %q", out)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: md5sum") {
		t.Errorf("help: %q code=%d", out, code)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("got code=%d e=%q", code, e)
	}
}
