package builtinutil_test

import (
	"os"
	"testing"

	"github.com/mark3labs/go-bash/internal/builtinutil"
)

func TestParseChmodNumeric(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("755", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	if m != 0o755 {
		t.Errorf("got %o", m)
	}
}

func TestParseChmodNumericLeadingZero(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("0700", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	if m != 0o700 {
		t.Errorf("got %o", m)
	}
}

func TestParseChmodSymbolicAdd(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("u+x", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	if m != 0o744 {
		t.Errorf("got %o", m)
	}
}

func TestParseChmodSymbolicRemove(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("g-r", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	if m != 0o604 {
		t.Errorf("got %o", m)
	}
}

func TestParseChmodSymbolicAssign(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("o=r", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	if m != 0o644 {
		t.Errorf("got %o", m)
	}
}

func TestParseChmodSymbolicCommaList(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("u+x,g-w,o=r", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	// 0o644: rw-r--r--
	// u+x => 0o744
	// g-w => 0o744 (g had no w)
	// o=r => 0o744 (already r)
	if m != 0o744 {
		t.Errorf("got %o", m)
	}
}

func TestParseChmodAll(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("a+w", 0o400, false)
	if err != nil {
		t.Fatal(err)
	}
	if m != 0o622 {
		t.Errorf("got %o", m)
	}
}

func TestParseChmodCapitalXOnDir(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("a+X", 0o644, true)
	if err != nil {
		t.Fatal(err)
	}
	if m&0o111 == 0 {
		t.Errorf("expected exec bits for dir: %o", m)
	}
}

func TestParseChmodCapitalXNoExecNoDir(t *testing.T) {
	m, err := builtinutil.ParseChmodMode("a+X", 0o644, false)
	if err != nil {
		t.Fatal(err)
	}
	if m&0o111 != 0 {
		t.Errorf("unexpected exec on non-dir non-exec: %o", m)
	}
}

func TestParseChmodInvalid(t *testing.T) {
	if _, err := builtinutil.ParseChmodMode("bogus", 0o644, false); err == nil {
		t.Error("expected error")
	}
}

func TestParseChmodEmpty(t *testing.T) {
	if _, err := builtinutil.ParseChmodMode("", 0o644, false); err == nil {
		t.Error("expected error")
	}
}

func TestHumanSizeSmall(t *testing.T) {
	if got := builtinutil.HumanSize(500); got != "500" {
		t.Errorf("got %q", got)
	}
}

func TestHumanSizeK(t *testing.T) {
	got := builtinutil.HumanSize(2048)
	if got != "2.0K" {
		t.Errorf("got %q", got)
	}
}

func TestHumanSizeM(t *testing.T) {
	got := builtinutil.HumanSize(2 * 1024 * 1024)
	if got != "2.0M" {
		t.Errorf("got %q", got)
	}
}

func TestHumanSizeRoundsUp(t *testing.T) {
	// 1500 bytes -> 1.5K (ceiling)
	got := builtinutil.HumanSize(1500)
	if got != "1.5K" {
		t.Errorf("got %q", got)
	}
}

func TestHumanSizeNegative(t *testing.T) {
	if got := builtinutil.HumanSize(-2048); got != "-2.0K" {
		t.Errorf("got %q", got)
	}
}

func TestResolvePathAbsolute(t *testing.T) {
	if p := builtinutil.ResolvePath("/home", "/etc"); p != "/etc" {
		t.Errorf("got %q", p)
	}
}

func TestResolvePathRelative(t *testing.T) {
	if p := builtinutil.ResolvePath("/home", "foo"); p != "/home/foo" {
		t.Errorf("got %q", p)
	}
}

func TestResolvePathEmptyCwd(t *testing.T) {
	if p := builtinutil.ResolvePath("", "foo"); p != "/foo" {
		t.Errorf("got %q", p)
	}
}

// Ensure os.FileMode constants haven't drifted (sanity).
func TestChmodModeMatchesOSFileMode(t *testing.T) {
	if os.FileMode(0o755).Perm() != 0o755 {
		t.Error("os.FileMode drift")
	}
}
