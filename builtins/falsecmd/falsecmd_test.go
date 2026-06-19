package falsecmd_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/falsecmd"
	"github.com/mark3labs/go-bash/command"
)

func TestFalseExitsOne(t *testing.T) {
	res := falsecmd.New().Execute(context.Background(), []string{"false"}, &command.Context{Stdout: &bytes.Buffer{}})
	if res.ExitCode != 1 {
		t.Errorf("exit = %d; want 1", res.ExitCode)
	}
}

func TestFalseIgnoresArgs(t *testing.T) {
	res := falsecmd.New().Execute(context.Background(), []string{"false", "junk"}, &command.Context{Stdout: &bytes.Buffer{}})
	if res.ExitCode != 1 {
		t.Errorf("exit = %d", res.ExitCode)
	}
}

func TestFalseHelpExitsZero(t *testing.T) {
	var buf bytes.Buffer
	res := falsecmd.New().Execute(context.Background(), []string{"false", "--help"}, &command.Context{Stdout: &buf})
	if res.ExitCode != 0 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if !strings.Contains(buf.String(), "Usage: false") {
		t.Errorf("help missing: %q", buf.String())
	}
}
