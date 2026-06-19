package whoami_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/whoami"
	"github.com/mark3labs/go-bash/command"
)

func TestWhoami(t *testing.T) {
	var o bytes.Buffer
	res := whoami.New().Execute(context.Background(), []string{"whoami"}, &command.Context{Stdout: &o})
	if res.ExitCode != 0 || o.String() != "user\n" {
		t.Errorf("out=%q exit=%d", o.String(), res.ExitCode)
	}
}

func TestWhoamiUnknownArg(t *testing.T) {
	var e bytes.Buffer
	res := whoami.New().Execute(context.Background(), []string{"whoami", "-x"}, &command.Context{Stderr: &e})
	if res.ExitCode != 2 || !strings.Contains(e.String(), "usage:") {
		t.Errorf("err=%q exit=%d", e.String(), res.ExitCode)
	}
}

func TestWhoamiHelp(t *testing.T) {
	var o bytes.Buffer
	res := whoami.New().Execute(context.Background(), []string{"whoami", "--help"}, &command.Context{Stdout: &o})
	if res.ExitCode != 0 || !strings.Contains(o.String(), "Usage: whoami") {
		t.Errorf("out=%q exit=%d", o.String(), res.ExitCode)
	}
}
