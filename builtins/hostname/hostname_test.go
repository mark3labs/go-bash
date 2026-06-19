package hostname_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/hostname"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func run(t *testing.T, ctx *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	ctx.Stdout = &o
	ctx.Stderr = &e
	res := hostname.New().Execute(context.Background(), append([]string{"hostname"}, args...), ctx)
	return o.String(), e.String(), res.ExitCode
}

func TestHostnameDefault(t *testing.T) {
	out, _, _ := run(t, &command.Context{})
	if out != "localhost\n" {
		t.Errorf("out=%q", out)
	}
}

func TestHostnameFromFile(t *testing.T) {
	mfs := memfs.New()
	if err := mfs.MkdirAll("/etc", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mfs.WriteFile("/etc/hostname", []byte("my-box.local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, _ := run(t, &command.Context{FS: mfs})
	if out != "my-box.local\n" {
		t.Errorf("out=%q", out)
	}
}

func TestHostnameShort(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/etc", 0o755)
	_ = mfs.WriteFile("/etc/hostname", []byte("my-box.local\n"), 0o644)
	out, _, _ := run(t, &command.Context{FS: mfs}, "-s")
	if out != "my-box\n" {
		t.Errorf("out=%q", out)
	}
}

func TestHostnameDomain(t *testing.T) {
	mfs := memfs.New()
	_ = mfs.MkdirAll("/etc", 0o755)
	_ = mfs.WriteFile("/etc/hostname", []byte("my-box.local\n"), 0o644)
	out, _, _ := run(t, &command.Context{FS: mfs}, "-d")
	if out != "local\n" {
		t.Errorf("out=%q", out)
	}
}

func TestHostnameUnknownFlag(t *testing.T) {
	_, err, code := run(t, &command.Context{}, "-Q")
	if code != 2 || !strings.Contains(err, "usage:") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestHostnameHelp(t *testing.T) {
	out, _, code := run(t, &command.Context{}, "--help")
	if code != 0 || !strings.Contains(out, "Usage: hostname") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

// Compile-time interface guard: ensure the test's mfs is a FileSystem.
var _ fs.FileSystem = (*memfs.FS)(nil)
