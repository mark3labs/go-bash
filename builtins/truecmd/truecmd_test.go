package truecmd_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/truecmd"
	"github.com/mark3labs/go-bash/command"
)

func TestTrueExitsZero(t *testing.T) {
	res := truecmd.New().Execute(context.Background(), []string{"true"}, &command.Context{Stdout: &bytes.Buffer{}})
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d; want 0", res.ExitCode)
	}
}

func TestTrueIgnoresArgs(t *testing.T) {
	res := truecmd.New().Execute(context.Background(), []string{"true", "anything", "--unknown"}, &command.Context{Stdout: &bytes.Buffer{}})
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d; want 0", res.ExitCode)
	}
}

func TestTrueHelp(t *testing.T) {
	var buf bytes.Buffer
	res := truecmd.New().Execute(context.Background(), []string{"true", "--help"}, &command.Context{Stdout: &buf})
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d", res.ExitCode)
	}
	if !strings.Contains(buf.String(), "Usage: true") {
		t.Errorf("help missing: %q", buf.String())
	}
}

func TestTrueName(t *testing.T) {
	if truecmd.New().Name() != "true" {
		t.Errorf("name = %q", truecmd.New().Name())
	}
}
