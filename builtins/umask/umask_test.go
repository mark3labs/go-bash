package umask

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestUmaskDefault(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"umask"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || o.String() != "0022\n" {
		t.Errorf("out=%q", o.String())
	}
}

func TestUmaskSet(t *testing.T) {
	r := New().Execute(context.Background(), []string{"umask", "077"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}

func TestUmaskSymbolic(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"umask", "-S"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 || !strings.Contains(o.String(), "u=rwx") {
		t.Errorf("out=%q", o.String())
	}
}

func TestUmaskInvalid(t *testing.T) {
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"umask", "9xx"}, &command.Context{Stderr: &e})
	if r.ExitCode == 0 || !strings.Contains(e.String(), "umask") {
		t.Errorf("exit=%d err=%q", r.ExitCode, e.String())
	}
}
