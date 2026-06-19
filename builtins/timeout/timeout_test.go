package timeout_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/go-bash/builtins/timeout"
	"github.com/mark3labs/go-bash/command"
)

// stubExec runs each "script" by interpreting it as a single token
// name and dispatching it through the supplied dispatch map. Just
// enough to exercise the timeout code paths without spinning up the
// full runtime.
func stubExec(dispatch map[string]func(ctx context.Context, opts command.SubExecOptions) (command.Result, error)) command.SubExecFunc {
	return func(ctx context.Context, script string, opts command.SubExecOptions) (command.Result, error) {
		// Strip single-quotes; we only test single-word commands.
		name := strings.Trim(strings.SplitN(script, " ", 2)[0], "'")
		fn, ok := dispatch[name]
		if !ok {
			return command.Result{ExitCode: 127}, nil
		}
		return fn(ctx, opts)
	}
}

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	c.Stdout = &o
	c.Stderr = &e
	res := timeout.New().Execute(context.Background(), append([]string{"timeout"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestTimeoutFastCommand(t *testing.T) {
	c := &command.Context{
		Exec: stubExec(map[string]func(context.Context, command.SubExecOptions) (command.Result, error){
			"quick": func(_ context.Context, opts command.SubExecOptions) (command.Result, error) {
				if opts.Stdout != nil {
					_, _ = io.WriteString(opts.Stdout, "done\n")
				}
				return command.Result{ExitCode: 0}, nil
			},
		}),
	}
	out, _, code := runCmd(t, c, "1", "quick")
	if code != 0 || out != "done\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestTimeoutFires(t *testing.T) {
	c := &command.Context{
		Exec: stubExec(map[string]func(context.Context, command.SubExecOptions) (command.Result, error){
			"slow": func(ctx context.Context, _ command.SubExecOptions) (command.Result, error) {
				select {
				case <-time.After(2 * time.Second):
					return command.Result{ExitCode: 0}, nil
				case <-ctx.Done():
					return command.Result{ExitCode: 0}, ctx.Err()
				}
			},
		}),
	}
	start := time.Now()
	_, _, code := runCmd(t, c, "0.05", "slow")
	elapsed := time.Since(start)
	if code != 124 {
		t.Errorf("code=%d want=124", code)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("took too long: %v", elapsed)
	}
}

func TestTimeoutPreserveStatus(t *testing.T) {
	c := &command.Context{
		Exec: stubExec(map[string]func(context.Context, command.SubExecOptions) (command.Result, error){
			"slow": func(ctx context.Context, _ command.SubExecOptions) (command.Result, error) {
				select {
				case <-time.After(1 * time.Second):
					return command.Result{ExitCode: 7}, nil
				case <-ctx.Done():
					return command.Result{ExitCode: 7}, ctx.Err()
				}
			},
		}),
	}
	_, _, code := runCmd(t, c, "--preserve-status", "0.05", "slow")
	if code != 7 {
		t.Errorf("code=%d want=7", code)
	}
}

func TestTimeoutInvalidDuration(t *testing.T) {
	c := &command.Context{
		Exec: stubExec(nil),
	}
	_, e, code := runCmd(t, c, "abc", "cmd")
	if code != 125 || !strings.Contains(e, "invalid time interval") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestTimeoutNoExecHook(t *testing.T) {
	c := &command.Context{}
	_, e, code := runCmd(t, c, "1", "cmd")
	if code != 125 || !strings.Contains(e, "sub-shell exec") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestTimeoutSignalFlagAccepted(t *testing.T) {
	c := &command.Context{
		Exec: stubExec(map[string]func(context.Context, command.SubExecOptions) (command.Result, error){
			"ok": func(_ context.Context, _ command.SubExecOptions) (command.Result, error) {
				return command.Result{ExitCode: 0}, nil
			},
		}),
	}
	_, _, code := runCmd(t, c, "-s", "TERM", "1", "ok")
	if code != 0 {
		t.Errorf("code=%d", code)
	}
	_, _, code = runCmd(t, c, "-k", "1", "1", "ok")
	if code != 0 {
		t.Errorf("code=%d (with -k)", code)
	}
}

func TestTimeoutHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: timeout") {
		t.Errorf("help out=%q", out)
	}
}

func TestTimeoutMissingArgs(t *testing.T) {
	_, e, code := runCmd(t, nil)
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d", code)
	}
}

func TestTimeoutExecError(t *testing.T) {
	c := &command.Context{
		Exec: func(_ context.Context, _ string, _ command.SubExecOptions) (command.Result, error) {
			return command.Result{}, errors.New("boom")
		},
	}
	_, e, code := runCmd(t, c, "1", "cmd")
	if code != 125 || !strings.Contains(e, "boom") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}
