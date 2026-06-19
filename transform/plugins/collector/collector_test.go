package collector_test

import (
	"testing"

	"github.com/mark3labs/go-bash/parser"
	"github.com/mark3labs/go-bash/transform"
	"github.com/mark3labs/go-bash/transform/plugins/collector"
)

func TestCollectorBasic(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		want    []string
	}{
		{
			name:   "single",
			script: `echo hi`,
			want:   []string{"echo"},
		},
		{
			name:   "pipeline",
			script: `echo hi | grep h | tr a-z A-Z`,
			want:   []string{"echo", "grep", "tr"},
		},
		{
			name:   "and-or",
			script: `true && echo ok || echo bad`,
			want:   []string{"true", "echo", "echo"},
		},
		{
			name:   "if",
			script: `if true; then echo yes; else echo no; fi`,
			want:   []string{"true", "echo", "echo"},
		},
		{
			name:   "for",
			script: `for i in 1 2 3; do echo "$i"; done`,
			want:   []string{"echo"},
		},
		{
			name:   "subshell",
			script: `(cd /tmp && ls)`,
			want:   []string{"cd", "ls"},
		},
		{
			name:   "command-subst",
			script: `echo "$(date)"`,
			want:   []string{"echo", "date"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := parser.Parse(tc.script)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			res := collector.New().Transform(transform.Context{AST: parsed})
			meta, ok := res.Metadata.(map[string]any)
			if !ok {
				t.Fatalf("metadata: %T", res.Metadata)
			}
			got, ok := meta[collector.MetadataKey].([]string)
			if !ok {
				t.Fatalf("commands: %T", meta[collector.MetadataKey])
			}
			if !equal(got, tc.want) {
				t.Fatalf("commands: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCollectorEmptyScript(t *testing.T) {
	parsed, err := parser.Parse("")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := collector.New().Transform(transform.Context{AST: parsed})
	meta := res.Metadata.(map[string]any)
	got, ok := meta[collector.MetadataKey].([]string)
	if !ok {
		t.Fatalf("commands: %T", meta[collector.MetadataKey])
	}
	if len(got) != 0 {
		t.Fatalf("commands: got %v, want []", got)
	}
}

func TestCollectorDoesNotMutateAST(t *testing.T) {
	parsed, err := parser.Parse("echo hi | wc -l")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	before := len(parsed.Origin.Stmts)
	res := collector.New().Transform(transform.Context{AST: parsed})
	if res.AST != parsed {
		t.Fatalf("AST should be the same pointer; got %p vs %p", res.AST, parsed)
	}
	if got := len(parsed.Origin.Stmts); got != before {
		t.Fatalf("Origin.Stmts changed: %d -> %d", before, got)
	}
}

func TestCollectorName(t *testing.T) {
	if got := collector.New().Name(); got != collector.Name {
		t.Fatalf("Name: got %q, want %q", got, collector.Name)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
