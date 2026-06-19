package sleep_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/go-bash/builtins/sleep"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, ctx *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	ctx.Stdout = &o
	ctx.Stderr = &e
	res := sleep.New().Execute(context.Background(), append([]string{"sleep"}, args...), ctx)
	return o.String(), e.String(), res.ExitCode
}

func TestSleepUsesContextSleep(t *testing.T) {
	var seen time.Duration
	c := &command.Context{
		Sleep: func(_ context.Context, d time.Duration) error {
			seen = d
			return nil
		},
	}
	_, _, code := run(t, c, "2.5")
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if seen != 2500*time.Millisecond {
		t.Errorf("dur = %v", seen)
	}
}

func TestSleepSuffixes(t *testing.T) {
	cases := []struct {
		op   string
		want time.Duration
	}{
		{"1", time.Second},
		{"5s", 5 * time.Second},
		{"2m", 2 * time.Minute},
		{"1h", time.Hour},
		{"1d", 24 * time.Hour},
		{"0.25", 250 * time.Millisecond},
	}
	for _, tc := range cases {
		var seen time.Duration
		c := &command.Context{
			Sleep: func(_ context.Context, d time.Duration) error {
				seen = d
				return nil
			},
		}
		_, _, code := run(t, c, tc.op)
		if code != 0 {
			t.Errorf("%s: code=%d", tc.op, code)
		}
		if seen != tc.want {
			t.Errorf("%s: got %v; want %v", tc.op, seen, tc.want)
		}
	}
}

func TestSleepMultipleOperandsSum(t *testing.T) {
	var seen time.Duration
	c := &command.Context{Sleep: func(_ context.Context, d time.Duration) error {
		seen = d
		return nil
	}}
	_, _, code := run(t, c, "1", "2s")
	if code != 0 {
		t.Errorf("code = %d", code)
	}
	if seen != 3*time.Second {
		t.Errorf("dur = %v", seen)
	}
}

func TestSleepInvalid(t *testing.T) {
	c := &command.Context{}
	_, err, code := run(t, c, "abc")
	if code != 1 || !strings.Contains(err, "invalid time interval") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestSleepNoOperand(t *testing.T) {
	c := &command.Context{}
	_, err, code := run(t, c, "--")
	if code != 1 || !strings.Contains(err, "missing operand") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestSleepCancelled(t *testing.T) {
	c := &command.Context{
		Sleep: func(_ context.Context, _ time.Duration) error {
			return context.Canceled
		},
	}
	_, _, code := run(t, c, "1")
	if code != 130 {
		t.Errorf("code = %d", code)
	}
}

func TestSleepHelp(t *testing.T) {
	c := &command.Context{}
	out, _, code := run(t, c, "--help")
	if code != 0 || !strings.Contains(out, "Usage:") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestSleepZeroDurationSkipsHook(t *testing.T) {
	called := false
	c := &command.Context{Sleep: func(_ context.Context, _ time.Duration) error {
		called = true
		return nil
	}}
	_, _, code := run(t, c, "0")
	if code != 0 || called {
		t.Errorf("called=%v code=%d", called, code)
	}
}
