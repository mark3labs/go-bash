package test

import (
	"context"
	"testing"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runT(t *testing.T, args ...string) int {
	t.Helper()
	r := NewTest().Execute(context.Background(), append([]string{"test"}, args...), &command.Context{})
	return r.ExitCode
}

func TestStringEq(t *testing.T) {
	if runT(t, "a", "=", "a") != 0 {
		t.Errorf("a=a should be true")
	}
	if runT(t, "a", "=", "b") != 1 {
		t.Errorf("a=b should be false")
	}
}

func TestStringNeq(t *testing.T) {
	if runT(t, "a", "!=", "b") != 0 {
		t.Errorf("a != b should be true")
	}
}

func TestZ(t *testing.T) {
	if runT(t, "-z", "") != 0 {
		t.Errorf("-z '' should be true")
	}
	if runT(t, "-z", "x") != 1 {
		t.Errorf("-z 'x' should be false")
	}
}

func TestN(t *testing.T) {
	if runT(t, "-n", "x") != 0 {
		t.Errorf("-n x should be true")
	}
}

func TestIntegerEq(t *testing.T) {
	if runT(t, "5", "-eq", "5") != 0 {
		t.Errorf("5 -eq 5")
	}
	if runT(t, "5", "-lt", "6") != 0 {
		t.Errorf("5 -lt 6")
	}
	if runT(t, "5", "-gt", "10") != 1 {
		t.Errorf("5 -gt 10 should be false")
	}
}

func TestNot(t *testing.T) {
	if runT(t, "!", "-z", "x") != 0 {
		t.Errorf("! -z x")
	}
}

func TestBracket(t *testing.T) {
	r := NewBracket().Execute(context.Background(), []string{"[", "a", "=", "a", "]"}, &command.Context{})
	if r.ExitCode != 0 {
		t.Errorf("[ a = a ] should be 0, got %d", r.ExitCode)
	}
}

func TestBracketMissingClose(t *testing.T) {
	r := NewBracket().Execute(context.Background(), []string{"[", "a"}, &command.Context{})
	if r.ExitCode != 2 {
		t.Errorf("expected 2 got %d", r.ExitCode)
	}
}

func TestFileTest(t *testing.T) {
	fs := memfs.New()
	_ = fs.WriteFile("/x", []byte("hi"), 0o644)
	c := &command.Context{FS: fs, Cwd: "/"}
	r := NewTest().Execute(context.Background(), []string{"test", "-f", "/x"}, c)
	if r.ExitCode != 0 {
		t.Errorf("-f /x")
	}
	r = NewTest().Execute(context.Background(), []string{"test", "-f", "/nope"}, c)
	if r.ExitCode != 1 {
		t.Errorf("-f /nope should be 1")
	}
}

func TestAndOr(t *testing.T) {
	if runT(t, "-z", "", "-a", "-n", "x") != 0 {
		t.Errorf("and")
	}
	if runT(t, "-n", "", "-o", "-z", "") != 0 {
		t.Errorf("or")
	}
}
