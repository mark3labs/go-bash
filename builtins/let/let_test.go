package let

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestLetSimple(t *testing.T) {
	r := New().Execute(context.Background(), []string{"let", "2+2"}, &command.Context{})
	if r.ExitCode != 0 { // 4 != 0, exit 0
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestLetZeroResult(t *testing.T) {
	r := New().Execute(context.Background(), []string{"let", "1-1"}, &command.Context{})
	if r.ExitCode != 1 { // 0, exit 1
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestLetAssign(t *testing.T) {
	env := map[string]string{}
	r := New().Execute(context.Background(), []string{"let", "x=4*3"}, &command.Context{Env: env})
	if env["x"] != "12" {
		t.Errorf("env=%v", env)
	}
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestLetSpacedAssign(t *testing.T) {
	env := map[string]string{}
	r := New().Execute(context.Background(), []string{"let", " x = 5 "}, &command.Context{Env: env})
	if env["x"] != "5" || r.ExitCode != 0 {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestLetParens(t *testing.T) {
	env := map[string]string{}
	r := New().Execute(context.Background(), []string{"let", "y=(2+3)*4"}, &command.Context{Env: env})
	if env["y"] != "20" || r.ExitCode != 0 {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestLetVariable(t *testing.T) {
	env := map[string]string{"a": "10"}
	r := New().Execute(context.Background(), []string{"let", "b=a+5"}, &command.Context{Env: env})
	if env["b"] != "15" || r.ExitCode != 0 {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestLetNoArgs(t *testing.T) {
	r := New().Execute(context.Background(), []string{"let"}, &command.Context{})
	if r.ExitCode != 2 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
