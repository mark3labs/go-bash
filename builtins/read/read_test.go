package read

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestReadBasic(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("hello\n")}
	r := New().Execute(context.Background(), []string{"read", "X"}, c)
	if r.ExitCode != 0 || env["X"] != "hello" {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestReadReply(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("greeting\n")}
	r := New().Execute(context.Background(), []string{"read"}, c)
	if r.ExitCode != 0 || env["REPLY"] != "greeting" {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestReadMultiple(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("a b c d\n")}
	_ = New().Execute(context.Background(), []string{"read", "A", "B", "C"}, c)
	if env["A"] != "a" || env["B"] != "b" || env["C"] != "c d" {
		t.Errorf("env=%v", env)
	}
}

func TestReadDelim(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("ab,cd")}
	_ = New().Execute(context.Background(), []string{"read", "-d", ",", "X"}, c)
	if env["X"] != "ab" {
		t.Errorf("env=%v", env)
	}
}

func TestReadN(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("abcdef")}
	_ = New().Execute(context.Background(), []string{"read", "-n", "3", "X"}, c)
	if env["X"] != "abc" {
		t.Errorf("env=%v", env)
	}
}

func TestReadPrompt(t *testing.T) {
	var e bytes.Buffer
	env := map[string]string{}
	c := &command.Context{Env: env, Stderr: &e, Stdin: strings.NewReader("v\n")}
	_ = New().Execute(context.Background(), []string{"read", "-p", "PROMPT> ", "X"}, c)
	if !strings.Contains(e.String(), "PROMPT") {
		t.Errorf("prompt=%q", e.String())
	}
}

func TestReadEOF(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("")}
	r := New().Execute(context.Background(), []string{"read", "X"}, c)
	if r.ExitCode != 1 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
