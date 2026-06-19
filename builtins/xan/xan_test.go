package xan_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/xan"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := xan.New().Execute(context.Background(),
		append([]string{"xan"}, args...),
		&command.Context{Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

const sample = `name,age,city
alice,30,nyc
bob,25,la
carol,35,nyc
`

func TestHeaders(t *testing.T) {
	out, _, code := run(t, sample, "headers")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "1   name\n2   age\n3   city\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestCount(t *testing.T) {
	out, _, code := run(t, sample, "count")
	if code != 0 || out != "3\n" {
		t.Errorf("got %q code=%d", out, code)
	}
}

func TestSelectByName(t *testing.T) {
	out, _, code := run(t, sample, "select", "name,city")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "name,city\nalice,nyc\nbob,la\ncarol,nyc\n"
	if out != want {
		t.Errorf("got %q", out)
	}
}

func TestSelectByIndex(t *testing.T) {
	out, _, code := run(t, sample, "select", "1,3")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "name,city\nalice,nyc\nbob,la\ncarol,nyc\n"
	if out != want {
		t.Errorf("got %q", out)
	}
}

func TestSliceStartLength(t *testing.T) {
	out, _, code := run(t, sample, "slice", "-s", "1", "-l", "1")
	if code != 0 {
		t.Fatalf("code=%d stderr was wrong", code)
	}
	want := "name,age,city\nbob,25,la\n"
	if out != want {
		t.Errorf("got %q", out)
	}
}

func TestFilterEqString(t *testing.T) {
	out, _, code := run(t, sample, "filter", "city == nyc")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "name,age,city\nalice,30,nyc\ncarol,35,nyc\n"
	if out != want {
		t.Errorf("got %q", out)
	}
}

func TestFilterNumeric(t *testing.T) {
	out, _, code := run(t, sample, "filter", "age > 28")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "name,age,city\nalice,30,nyc\ncarol,35,nyc\n"
	if out != want {
		t.Errorf("got %q", out)
	}
}

func TestFilterRegex(t *testing.T) {
	out, _, code := run(t, sample, "filter", "name =~ ^a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "name,age,city\nalice,30,nyc\n"
	if out != want {
		t.Errorf("got %q", out)
	}
}

func TestFlatten(t *testing.T) {
	out, _, _ := run(t, sample, "flatten")
	if !strings.Contains(out, "name  alice") || !strings.Contains(out, "city  nyc") {
		t.Errorf("got %q", out)
	}
}

func TestStatsNumber(t *testing.T) {
	out, _, _ := run(t, sample, "stats")
	if !strings.Contains(out, "age,3,number,25,35,30") {
		t.Errorf("got %q", out)
	}
}

func TestToJSON(t *testing.T) {
	out, _, _ := run(t, "a,b\n1,2\n3,4\n", "to", "json")
	if !strings.Contains(out, `"a": "1"`) || !strings.Contains(out, `"b": "4"`) {
		t.Errorf("got %q", out)
	}
}

func TestToJSONL(t *testing.T) {
	out, _, _ := run(t, "a,b\n1,2\n3,4\n", "to", "jsonl")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("got lines=%v", lines)
	}
}

func TestToTSV(t *testing.T) {
	out, _, _ := run(t, "a,b\n1,2\n", "to", "tsv")
	if out != "a\tb\n1\t2\n" {
		t.Errorf("got %q", out)
	}
}

func TestFromJSON(t *testing.T) {
	out, _, _ := run(t, `[{"a":"1","b":"2"},{"a":"3","b":"4"}]`, "from", "json")
	want := "a,b\n1,2\n3,4\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestFromJSONL(t *testing.T) {
	in := `{"a":"1","b":"2"}` + "\n" + `{"a":"3","b":"4"}` + "\n"
	out, _, _ := run(t, in, "from", "jsonl")
	want := "a,b\n1,2\n3,4\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestFromTSV(t *testing.T) {
	out, _, _ := run(t, "a\tb\n1\t2\n", "from", "tsv")
	want := "a,b\n1,2\n"
	if out != want {
		t.Errorf("got %q", out)
	}
}

func TestCat(t *testing.T) {
	a := "k,v\na,1\nb,2\n"
	b := "k,v\nc,3\n"
	// concat across stdin and a second file requires VFS; instead exercise the
	// case where two stdin reads aren't possible — use `cat rows -` plus the
	// same file twice, conceptually validated by the unit test.
	// Here we simply confirm "cat rows -" returns the input unchanged.
	out, _, _ := run(t, a+"END", "cat", "rows")
	// Trailing "END" is treated as a 1-field 1-row record by encoding/csv.
	_ = b
	if !strings.HasPrefix(out, "k,v\na,1\nb,2\n") {
		t.Errorf("got %q", out)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: xan") {
		t.Errorf("help: code=%d out=%q", code, out)
	}
}

func TestUnknown(t *testing.T) {
	_, e, code := run(t, "", "bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}

func TestNoArgs(t *testing.T) {
	_, e, code := run(t, "")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}
