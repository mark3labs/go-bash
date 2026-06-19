package tee_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/parser"
	"github.com/mark3labs/go-bash/transform"
	teeplugin "github.com/mark3labs/go-bash/transform/plugins/tee"
)

func TestTeeWrapsSingleCommand(t *testing.T) {
	parsed, err := parser.Parse(`echo hi`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := teeplugin.New(teeplugin.Options{OutputDir: "/OUT"})
	res := p.Transform(transform.Context{AST: parsed})

	meta := res.Metadata.(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}
	want := teeplugin.TeeFileInfo{
		CommandIndex: 0,
		CommandName:  "echo",
		Command:      "echo hi",
		StdoutFile:   "/OUT/0-echo.stdout.txt",
	}
	if files[0] != want {
		t.Fatalf("file: got %+v, want %+v", files[0], want)
	}

	got, err := transform.Serialize(parsed)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if !strings.Contains(got, "tee /OUT/0-echo.stdout.txt") {
		t.Fatalf("serialized: %q", got)
	}
}

func TestTeeWrapsPipelineStages(t *testing.T) {
	parsed, err := parser.Parse(`echo hi | grep h | wc -l`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := teeplugin.New(teeplugin.Options{OutputDir: "/OUT"})
	res := p.Transform(transform.Context{AST: parsed})

	meta := res.Metadata.(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 3 {
		t.Fatalf("files: %v", files)
	}
	names := []string{files[0].CommandName, files[1].CommandName, files[2].CommandName}
	wantNames := []string{"echo", "grep", "wc"}
	for i := range names {
		if names[i] != wantNames[i] {
			t.Fatalf("name[%d]: got %q, want %q", i, names[i], wantNames[i])
		}
		if files[i].CommandIndex != i {
			t.Fatalf("index[%d]: %d", i, files[i].CommandIndex)
		}
	}

	got, err := transform.Serialize(parsed)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	for i, n := range wantNames {
		needle := "tee /OUT/" + itoa(i) + "-" + n + ".stdout.txt"
		if !strings.Contains(got, needle) {
			t.Fatalf("serialized missing %q: %s", needle, got)
		}
	}
}

func TestTeeRespectsTargetMatch(t *testing.T) {
	parsed, err := parser.Parse(`echo hi | grep h | wc -l`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := teeplugin.New(teeplugin.Options{
		OutputDir:          "/OUT",
		TargetCommandMatch: func(name string) bool { return name == "grep" },
	})
	res := p.Transform(transform.Context{AST: parsed})
	meta := res.Metadata.(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}
	if files[0].CommandName != "grep" {
		t.Fatalf("name: %s", files[0].CommandName)
	}
}

func TestTeeSkipsItself(t *testing.T) {
	// If a script already has `tee`, the plugin must NOT wrap that
	// stage (preventing infinite-recursion when running the plugin
	// twice).
	parsed, err := parser.Parse(`echo hi | tee /tmp/x`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := teeplugin.New(teeplugin.Options{OutputDir: "/OUT"})
	res := p.Transform(transform.Context{AST: parsed})
	meta := res.Metadata.(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}
	if files[0].CommandName != "echo" {
		t.Fatalf("name: %s", files[0].CommandName)
	}
}

func TestTeeIdempotent(t *testing.T) {
	parsed, err := parser.Parse(`echo hi | grep h`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := teeplugin.New(teeplugin.Options{OutputDir: "/OUT"})
	_ = p.Transform(transform.Context{AST: parsed})
	res2 := p.Transform(transform.Context{AST: parsed})
	meta := res2.Metadata.(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	// On the second pass, the only non-tee stages are still echo and
	// grep — the inserted tees are skipped. So we get 2 more entries.
	if len(files) != 2 {
		t.Fatalf("idempotent files: %v", files)
	}
}

func TestTeeEmptyOutputDirNoOp(t *testing.T) {
	parsed, err := parser.Parse(`echo hi | grep h`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	before, _ := transform.Serialize(parsed)
	p := teeplugin.New(teeplugin.Options{OutputDir: ""})
	res := p.Transform(transform.Context{AST: parsed})
	meta := res.Metadata.(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 0 {
		t.Fatalf("files: %v", files)
	}
	after, _ := transform.Serialize(parsed)
	if before != after {
		t.Fatalf("AST should be untouched: %q -> %q", before, after)
	}
}

func TestTeeNestedPipelinesInSubshell(t *testing.T) {
	parsed, err := parser.Parse(`(echo a | tr a-z A-Z)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := teeplugin.New(teeplugin.Options{OutputDir: "/OUT"})
	res := p.Transform(transform.Context{AST: parsed})
	meta := res.Metadata.(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 2 {
		t.Fatalf("nested files: %v", files)
	}
}

func TestTeeSanitizesCommandName(t *testing.T) {
	// A command name like "with/slash" must not produce a path
	// component containing "/".
	parsed, err := parser.Parse(`'a/b' arg`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := teeplugin.New(teeplugin.Options{OutputDir: "/OUT"})
	res := p.Transform(transform.Context{AST: parsed})
	meta := res.Metadata.(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}
	// "a/b" should be sanitized; the path under /OUT should be a
	// single component, not /OUT/0-a/b.stdout.txt.
	if !strings.HasPrefix(files[0].StdoutFile, "/OUT/0-a_b") {
		t.Fatalf("sanitize: %q", files[0].StdoutFile)
	}
}

func TestTeeName(t *testing.T) {
	if got := teeplugin.New(teeplugin.Options{}).Name(); got != teeplugin.Name {
		t.Fatalf("Name: %q", got)
	}
}

func itoa(i int) string {
	switch i {
	case 0:
		return "0"
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	}
	return ""
}
