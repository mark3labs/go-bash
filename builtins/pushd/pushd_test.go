package pushd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func TestPushd(t *testing.T) {
	fs := memfs.New()
	_ = fs.MkdirAll("/tmp", 0o755)
	var o bytes.Buffer
	r := New().Execute(context.Background(), []string{"pushd", "/tmp"}, &command.Context{FS: fs, Stdout: &o})
	if r.ExitCode != 0 || !strings.Contains(o.String(), "/tmp") {
		t.Errorf("out=%q exit=%d", o.String(), r.ExitCode)
	}
}

func TestPushdNoArgs(t *testing.T) {
	var e bytes.Buffer
	r := New().Execute(context.Background(), []string{"pushd"}, &command.Context{Stderr: &e})
	if r.ExitCode != 1 {
		t.Errorf("exit=%d", r.ExitCode)
	}
}
