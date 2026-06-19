package shopt

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/runtimestate"
)

func TestShoptSet(t *testing.T) {
	st := runtimestate.NewShoptTable()
	r := New().Execute(context.Background(), []string{"shopt", "-s", "expand_aliases"}, &command.Context{Shopt: st})
	if r.ExitCode != 0 || !st.IsSet("expand_aliases") {
		t.Errorf("not set")
	}
}

func TestShoptUnset(t *testing.T) {
	st := runtimestate.NewShoptTable()
	st.Set("foo", true)
	r := New().Execute(context.Background(), []string{"shopt", "-u", "foo"}, &command.Context{Shopt: st})
	if r.ExitCode != 0 || st.IsSet("foo") {
		t.Errorf("still set")
	}
}

func TestShoptQuery(t *testing.T) {
	st := runtimestate.NewShoptTable()
	st.Set("a", true)
	r := New().Execute(context.Background(), []string{"shopt", "-q", "a"}, &command.Context{Shopt: st})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
	r = New().Execute(context.Background(), []string{"shopt", "-q", "missing"}, &command.Context{Shopt: st})
	if r.ExitCode != 1 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestShoptPrint(t *testing.T) {
	st := runtimestate.NewShoptTable()
	st.Set("a", true)
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"shopt", "-p", "a"}, &command.Context{Shopt: st, Stdout: &o})
	if r.ExitCode != 0 || !strings.Contains(o.String(), "a\ton") {
		t.Errorf("out=%q", o.String())
	}
}
