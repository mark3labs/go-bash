package time_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	stdtime "time"

	gbtime "github.com/mark3labs/go-bash/builtins/time"
	"github.com/mark3labs/go-bash/command"
)

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	c.Stdout = &o
	c.Stderr = &e
	res := gbtime.New().Execute(context.Background(), append([]string{"time"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestTimeBasic(t *testing.T) {
	c := &command.Context{
		Exec: func(_ context.Context, _ string, _ command.SubExecOptions) (command.Result, error) {
			stdtime.Sleep(20 * stdtime.Millisecond)
			return command.Result{ExitCode: 0}, nil
		},
	}
	_, e, code := runCmd(t, c, "true")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(e, "real\t") {
		t.Errorf("stderr missing real: %q", e)
	}
}

func TestTimePropagatesExit(t *testing.T) {
	c := &command.Context{
		Exec: func(_ context.Context, _ string, _ command.SubExecOptions) (command.Result, error) {
			return command.Result{ExitCode: 42}, nil
		},
	}
	_, _, code := runCmd(t, c, "fail")
	if code != 42 {
		t.Errorf("code=%d want=42", code)
	}
}

func TestTimeHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: time") {
		t.Errorf("help out=%q", out)
	}
}

func TestTimeUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-z")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestTimeNoArgs(t *testing.T) {
	_, _, code := runCmd(t, nil)
	if code != 0 {
		t.Errorf("code=%d", code)
	}
}

func TestTimeNoExecHook(t *testing.T) {
	c := &command.Context{}
	_, e, code := runCmd(t, c, "cmd")
	if code != 1 || !strings.Contains(e, "sub-shell exec") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}
