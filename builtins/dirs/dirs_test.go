package dirs

import (
	"bytes"
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
)

func TestDirs(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"dirs"}, &command.Context{Cwd: "/home/user", Stdout: &o})
	if r.ExitCode != 0 || o.String() != "/home/user\n" {
		t.Errorf("out=%q exit=%d", o.String(), r.ExitCode)
	}
}

func TestDirsFlags(t *testing.T) {
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"dirs", "-c"}, &command.Context{Cwd: "/", Stdout: &o})
	if r.ExitCode != 0 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
