package popd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestPopdEmpty(t *testing.T) {
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"popd"}, &command.Context{Stderr: &e})
	if r.ExitCode != 1 || !strings.Contains(e.String(), "empty") {
		t.Errorf("exit=%d err=%q", r.ExitCode, e.String())
	}
}

func TestPopdHelp(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"popd", "--help"}, &command.Context{Stdout: &o})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
