package sha256sum_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/sha256sum"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := sha256sum.New().Execute(context.Background(), append([]string{"sha256sum"}, args...), &command.Context{
		Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e,
	})
	return o.String(), e.String(), res.ExitCode
}

func TestSum(t *testing.T) {
	out, _, _ := run(t, "hello")
	if !strings.HasPrefix(out, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824") {
		t.Errorf("got %q", out)
	}
}
