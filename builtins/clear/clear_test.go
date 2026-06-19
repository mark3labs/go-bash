package clear_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/builtins/clear"
	"github.com/mark3labs/go-bash/command"
)

func TestClearWritesAnsi(t *testing.T) {
	var buf bytes.Buffer
	res := clear.New().Execute(context.Background(), []string{"clear"}, &command.Context{Stdout: &buf})
	if res.ExitCode != 0 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if buf.String() != clear.ANSIClear {
		t.Errorf("stdout = %q; want %q", buf.String(), clear.ANSIClear)
	}
}

func TestClearHelp(t *testing.T) {
	var buf bytes.Buffer
	res := clear.New().Execute(context.Background(), []string{"clear", "--help"}, &command.Context{Stdout: &buf})
	if res.ExitCode != 0 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if buf.String() == clear.ANSIClear {
		t.Error("help mode should not emit ANSI clear")
	}
}
