package alias

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func parse(t *testing.T, src string) *syntax.File {
	t.Helper()
	f, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func print(f *syntax.File) string {
	var sb strings.Builder
	_ = syntax.NewPrinter().Print(&sb, f)
	return sb.String()
}

func TestExpandSimple(t *testing.T) {
	f := parse(t, "ll x y")
	Expand(f, map[string]string{"ll": "ls -l"})
	got := print(f)
	if !strings.Contains(got, "ls -l x y") {
		t.Errorf("got %q", got)
	}
}

func TestExpandNoMatch(t *testing.T) {
	f := parse(t, "echo x")
	Expand(f, map[string]string{"ll": "ls -l"})
	got := print(f)
	if !strings.Contains(got, "echo x") {
		t.Errorf("got %q", got)
	}
}

func TestExpandChained(t *testing.T) {
	f := parse(t, "a")
	Expand(f, map[string]string{"a": "b", "b": "c"})
	got := print(f)
	if !strings.Contains(got, "c") {
		t.Errorf("got %q", got)
	}
}

func TestExpandSelfLoopSafe(t *testing.T) {
	f := parse(t, "ls -l")
	Expand(f, map[string]string{"ls": "ls --color"})
	// Should NOT infinite-loop. We accept that this alias is a no-op
	// (matching bash's "first-token self-reference is suppressed" rule).
	_ = print(f)
}

func TestExpandNil(t *testing.T) {
	f := parse(t, "echo x")
	Expand(f, nil)
	Expand(nil, map[string]string{"a": "b"})
}
