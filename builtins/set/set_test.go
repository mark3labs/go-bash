package set

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/runtimestate"
)

func TestSetListsEnv(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"set"}, &command.Context{
		Env:    map[string]string{"A": "1", "B": "2"},
		Stdout: &o,
	})
	if r.ExitCode != 0 || !strings.Contains(o.String(), "A=1") {
		t.Errorf("out=%q", o.String())
	}
}

func TestSetFlag(t *testing.T) {
	st := runtimestate.NewShoptTable()
	r := New().Execute(context.Background(), []string{"set", "-e"}, &command.Context{Shopt: st})
	if r.ExitCode != 0 || !st.IsSet("set:e") {
		t.Errorf("exit=%d set:e=%v", r.ExitCode, st.IsSet("set:e"))
	}
}

func TestSetOPipefail(t *testing.T) {
	st := runtimestate.NewShoptTable()
	r := New().Execute(context.Background(), []string{"set", "-o", "pipefail"}, &command.Context{Shopt: st})
	if r.ExitCode != 0 || !st.IsSet("set:o:pipefail") {
		t.Errorf("not set")
	}
}
