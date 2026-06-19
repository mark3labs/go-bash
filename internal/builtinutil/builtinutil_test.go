package builtinutil_test

import (
	"bytes"
	"testing"

	"github.com/mark3labs/go-bash/internal/builtinutil"
)

func TestPrintHelpAddsNewline(t *testing.T) {
	var b bytes.Buffer
	builtinutil.PrintHelp(&b, "usage line")
	if b.String() != "usage line\n" {
		t.Errorf("got %q", b.String())
	}
	b.Reset()
	builtinutil.PrintHelp(&b, "with trailing\n")
	if b.String() != "with trailing\n" {
		t.Errorf("got %q", b.String())
	}
}

func TestUsageError(t *testing.T) {
	var b bytes.Buffer
	res := builtinutil.UsageError(&b, "cmd [opts]")
	if res.ExitCode != 2 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if b.String() != "usage: cmd [opts]\n" {
		t.Errorf("got %q", b.String())
	}
}

func TestUsageErrorNilWriter(t *testing.T) {
	res := builtinutil.UsageError(nil, "x")
	if res.ExitCode != 2 {
		t.Errorf("exit = %d", res.ExitCode)
	}
}

func TestErrorf(t *testing.T) {
	var b bytes.Buffer
	res := builtinutil.Errorf(&b, "mycmd", 7, "bad %s", "thing")
	if res.ExitCode != 7 {
		t.Errorf("exit = %d", res.ExitCode)
	}
	if b.String() != "mycmd: bad thing\n" {
		t.Errorf("got %q", b.String())
	}
}

func TestErrorfNoCmd(t *testing.T) {
	var b bytes.Buffer
	_ = builtinutil.Errorf(&b, "", 1, "just a message")
	if b.String() != "just a message\n" {
		t.Errorf("got %q", b.String())
	}
}
