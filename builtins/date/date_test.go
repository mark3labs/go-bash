package date_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/go-bash/builtins/date"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
)

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	c.Stdout = &o
	c.Stderr = &e
	res := date.New().Execute(context.Background(), append([]string{"date"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestDateFromUnixD(t *testing.T) {
	// 1700000000 = 2023-11-14 22:13:20 UTC
	out, _, code := runCmd(t, nil, "-d", "@1700000000", "+%Y-%m-%d %H:%M:%S")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if strings.TrimSpace(out) != "2023-11-14 22:13:20" {
		t.Errorf("out=%q", out)
	}
}

func TestDateUTCDefault(t *testing.T) {
	// Without TZ, defaults to UTC.
	c := &command.Context{}
	out, _, _ := runCmd(t, c, "-d", "@0", "+%Z")
	if strings.TrimSpace(out) != "UTC" {
		t.Errorf("out=%q want=UTC", out)
	}
}

func TestDateTZHonored(t *testing.T) {
	c := &command.Context{Env: map[string]string{"TZ": "America/New_York"}}
	out, _, _ := runCmd(t, c, "-d", "@0", "+%Z")
	got := strings.TrimSpace(out)
	if got != "EST" && got != "EDT" {
		t.Errorf("out=%q want=EST/EDT", got)
	}
}

func TestDateForceUTC(t *testing.T) {
	c := &command.Context{Env: map[string]string{"TZ": "America/New_York"}}
	out, _, _ := runCmd(t, c, "-u", "-d", "@0", "+%Z")
	if strings.TrimSpace(out) != "UTC" {
		t.Errorf("out=%q (should force UTC despite TZ)", out)
	}
}

func TestDateRFile(t *testing.T) {
	fsys := memfs.New()
	mtime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	if err := fsys.MkdirAll("/work", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile("/work/f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Chtimes("/work/f", mtime, mtime); err != nil {
		t.Fatal(err)
	}
	c := &command.Context{FS: fsys, Cwd: "/work"}
	out, _, code := runCmd(t, c, "-r", "f", "+%Y-%m-%d")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if strings.TrimSpace(out) != "2024-06-15" {
		t.Errorf("out=%q", out)
	}
}

func TestDateRFileMissing(t *testing.T) {
	fsys := memfs.New()
	c := &command.Context{FS: fsys, Cwd: "/"}
	_, e, code := runCmd(t, c, "-r", "nope")
	if code != 1 || e == "" {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestDateStrftimePercent(t *testing.T) {
	// Literal %% and unknown conversion fall-through.
	out, _, _ := runCmd(t, nil, "-d", "@0", "+%%-%Y-%Q")
	if strings.TrimSpace(out) != "%-1970-%Q" {
		t.Errorf("out=%q", out)
	}
}

func TestDateRFC2822(t *testing.T) {
	out, _, _ := runCmd(t, nil, "-R", "-d", "@0")
	if !strings.Contains(out, "Thu, 01 Jan 1970 00:00:00 +0000") {
		t.Errorf("out=%q", out)
	}
}

func TestDateISO(t *testing.T) {
	out, _, _ := runCmd(t, nil, "-I", "-d", "@0")
	if strings.TrimSpace(out) != "1970-01-01" {
		t.Errorf("out=%q", out)
	}
	out2, _, _ := runCmd(t, nil, "-Iseconds", "-d", "@0")
	if strings.TrimSpace(out2) != "1970-01-01T00:00:00+0000" {
		t.Errorf("out=%q", out2)
	}
}

func TestDateInvalidString(t *testing.T) {
	_, e, code := runCmd(t, nil, "-d", "not a date")
	if code != 1 || !strings.Contains(e, "invalid date") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestDateHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: date") {
		t.Errorf("help out=%q", out)
	}
}

func TestDateUnknownOption(t *testing.T) {
	_, e, code := runCmd(t, nil, "-Q")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d", code)
	}
}

func TestDateStrftimeFull(t *testing.T) {
	// Spot-check many conversions on a known time.
	out, _, _ := runCmd(t, nil, "-d", "@1700000000", "+%a %A %b %B %d %H:%M:%S %Y %Z")
	want := "Tue Tuesday Nov November 14 22:13:20 2023 UTC\n"
	if out != want {
		t.Errorf("out=%q want=%q", out, want)
	}
}
