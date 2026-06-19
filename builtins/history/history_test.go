package history_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/history"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/runtimestate"
)

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	if c.History == nil {
		c.History = runtimestate.NewHistoryRing(0)
	}
	c.Stdout = &o
	c.Stderr = &e
	res := history.New().Execute(context.Background(), append([]string{"history"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestHistoryList(t *testing.T) {
	c := &command.Context{History: runtimestate.NewHistoryRing(0)}
	c.History.Add("ls")
	c.History.Add("pwd")
	out, _, code := runCmd(t, c)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "    1  ls\n    2  pwd\n"
	if out != want {
		t.Errorf("out=%q want=%q", out, want)
	}
}

func TestHistoryLimit(t *testing.T) {
	c := &command.Context{History: runtimestate.NewHistoryRing(0)}
	c.History.Add("a")
	c.History.Add("b")
	c.History.Add("c")
	out, _, _ := runCmd(t, c, "2")
	want := "    2  b\n    3  c\n"
	if out != want {
		t.Errorf("out=%q want=%q", out, want)
	}
}

func TestHistoryClear(t *testing.T) {
	c := &command.Context{History: runtimestate.NewHistoryRing(0)}
	c.History.Add("ls")
	_, _, code := runCmd(t, c, "-c")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if c.History.Len() != 0 {
		t.Error("history not cleared")
	}
}

func TestHistoryStore(t *testing.T) {
	c := &command.Context{History: runtimestate.NewHistoryRing(0)}
	_, _, code := runCmd(t, c, "-s", "manually", "added")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	_, cmds := c.History.List()
	if len(cmds) != 1 || cmds[0] != "manually added" {
		t.Errorf("cmds=%v", cmds)
	}
}

func TestHistoryDeleteUnsupported(t *testing.T) {
	c := &command.Context{History: runtimestate.NewHistoryRing(0)}
	_, e, code := runCmd(t, c, "-d", "1")
	if code != 1 || !strings.Contains(e, "-d not supported") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestHistoryHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: history") {
		t.Errorf("help out=%q", out)
	}
}

func TestHistoryUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-z")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d", code)
	}
}

func TestHistoryRingEvict(t *testing.T) {
	c := &command.Context{History: runtimestate.NewHistoryRing(2)}
	c.History.Add("a")
	c.History.Add("b")
	c.History.Add("c")
	out, _, _ := runCmd(t, c)
	// expect b, c with sequences 2 and 3
	want := "    2  b\n    3  c\n"
	if out != want {
		t.Errorf("out=%q want=%q", out, want)
	}
}
