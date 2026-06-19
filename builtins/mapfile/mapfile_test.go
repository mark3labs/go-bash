package mapfile

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestMapfileBasic(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("a\nb\nc\n")}
	r := New().Execute(context.Background(), []string{"mapfile", "-t", "ARR"}, c)
	if r.ExitCode != 0 || env["ARR_0"] != "a" || env["ARR_2"] != "c" {
		t.Errorf("env=%v exit=%d", env, r.ExitCode)
	}
}

func TestMapfileN(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("a\nb\nc\n")}
	_ = New().Execute(context.Background(), []string{"mapfile", "-t", "-n", "2", "ARR"}, c)
	if env["ARR_0"] != "a" || env["ARR_1"] != "b" || env["ARR_2"] != "" {
		t.Errorf("env=%v", env)
	}
}

func TestMapfileOrigin(t *testing.T) {
	env := map[string]string{}
	c := &command.Context{Env: env, Stdin: strings.NewReader("x\ny\n")}
	_ = New().Execute(context.Background(), []string{"mapfile", "-t", "-O", "5", "ARR"}, c)
	if env["ARR_5"] != "x" || env["ARR_6"] != "y" {
		t.Errorf("env=%v", env)
	}
}

func TestReadarrayAlias(t *testing.T) {
	if NewReadarray().Name() != "readarray" {
		t.Errorf("name=%v", NewReadarray().Name())
	}
}
