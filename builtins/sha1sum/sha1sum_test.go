package sha1sum_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/sha1sum"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := sha1sum.New().Execute(context.Background(), append([]string{"sha1sum"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestSum(t *testing.T) {
	out, _, _ := run(t, "hello")
	if !strings.HasPrefix(out, "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d") {
		t.Errorf("got %q", out)
	}
}
