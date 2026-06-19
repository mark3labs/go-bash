package yq_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/yq"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := yq.New().Execute(context.Background(),
		append([]string{"yq"}, args...),
		&command.Context{Cwd: "/", Stdin: strings.NewReader(stdin), Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestYAMLToJSON(t *testing.T) {
	in := "a: 1\nb: two\n"
	out, _, code := run(t, in, "-o", "json", "-c", ".")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := `{"a":1,"b":"two"}` + "\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestJSONToYAML(t *testing.T) {
	out, _, code := run(t, `{"a":1,"b":"two"}`, "-i", "json", "-o", "yaml", ".")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "a: 1") || !strings.Contains(out, "b: two") {
		t.Errorf("got %q", out)
	}
}

func TestYAMLDefaultRoundTrip(t *testing.T) {
	out, _, code := run(t, "a: 1\n", ".a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	// YAML output of a bare integer.
	if !strings.Contains(out, "1") {
		t.Errorf("got %q", out)
	}
}

func TestRawOutput(t *testing.T) {
	out, _, code := run(t, `{"a":"hello"}`, "-i", "json", "-o", "json", "-r", ".a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "hello\n" {
		t.Errorf("got %q", out)
	}
}

func TestTOMLToJSON(t *testing.T) {
	in := `title = "yq"
[owner]
name = "bob"
`
	out, _, code := run(t, in, "-i", "toml", "-o", "json", "-c", ".")
	if code != 0 {
		t.Fatalf("code=%d stderr-side: ?", code)
	}
	if !strings.Contains(out, `"title":"yq"`) || !strings.Contains(out, `"owner":{"name":"bob"}`) {
		t.Errorf("got %q", out)
	}
}

func TestJSONToTOML(t *testing.T) {
	out, _, code := run(t, `{"title":"yq"}`, "-i", "json", "-o", "toml", ".")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, `title = "yq"`) {
		t.Errorf("got %q", out)
	}
}

func TestCSVToJSON(t *testing.T) {
	in := "a,b\n1,2\n3,4\n"
	out, _, code := run(t, in, "-i", "csv", "-o", "json", "-c", ".")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	// CSV decodes to a single value: an array of 2 maps.
	want := `[{"a":"1","b":"2"},{"a":"3","b":"4"}]` + "\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestJSONToCSV(t *testing.T) {
	in := `[{"a":"1","b":"2"},{"a":"3","b":"4"}]`
	out, _, code := run(t, in, "-i", "json", "-o", "csv", ".")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "a,b\n1,2\n3,4\n"
	if out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestXMLToJSON(t *testing.T) {
	in := `<root><a>1</a><a>2</a><b name="x">3</b></root>`
	out, _, code := run(t, in, "-i", "xml", "-o", "json", "-c", ".")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, `"root":{`) || !strings.Contains(out, `"a":["1","2"]`) {
		t.Errorf("got %q", out)
	}
	if !strings.Contains(out, `"@name":"x"`) || !strings.Contains(out, `"#text":"3"`) {
		t.Errorf("got %q", out)
	}
}

func TestJSONToXML(t *testing.T) {
	in := `{"root":{"a":"1","b":"two"}}`
	out, _, code := run(t, in, "-i", "json", "-o", "xml", ".")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "<root>") || !strings.Contains(out, "<a>1</a>") {
		t.Errorf("got %q", out)
	}
}

func TestFilter(t *testing.T) {
	in := `{"items":[{"n":1},{"n":2},{"n":3}]}`
	out, _, code := run(t, in, "-i", "json", "-o", "json", "-c", "[.items[].n] | add")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "6\n" {
		t.Errorf("got %q", out)
	}
}

func TestCompact(t *testing.T) {
	in := `{"a":1}`
	out, _, _ := run(t, in, "-i", "json", "-o", "json", "-c", ".")
	if out != "{\"a\":1}\n" {
		t.Errorf("got %q", out)
	}
}

func TestPFlagSetsBoth(t *testing.T) {
	in := `{"a":1}`
	out, _, code := run(t, in, "-p", "json", "-c", ".")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "{\"a\":1}\n" {
		t.Errorf("got %q", out)
	}
}

func TestHelp(t *testing.T) {
	out, _, code := run(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage: yq") {
		t.Errorf("help: code=%d out=%q", code, out)
	}
}

func TestUnknownOption(t *testing.T) {
	_, e, code := run(t, "", "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d e=%q", code, e)
	}
}

func TestUnknownFormat(t *testing.T) {
	_, e, code := run(t, "{}", "-i", "fnord", ".")
	if code != 2 || !strings.Contains(e, "unknown input format") {
		t.Errorf("code=%d e=%q", code, e)
	}
}

func TestMultiDocYAML(t *testing.T) {
	in := "a: 1\n---\na: 2\n"
	out, _, code := run(t, in, "-i", "yaml", "-o", "json", "-c", ".a")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "1\n2\n" {
		t.Errorf("got %q", out)
	}
}
