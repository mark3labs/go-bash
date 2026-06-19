package transform_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/transform"
)

// orderRecorder is a tiny in-memory plugin that appends to a shared
// trace slice on each Transform call, then echoes back the AST
// unchanged.
type orderRecorder struct {
	name  string
	trace *[]string
}

func (p *orderRecorder) Name() string { return p.name }

func (p *orderRecorder) Transform(ctx transform.Context) transform.Result {
	*p.trace = append(*p.trace, p.name)
	return transform.Result{
		AST: ctx.AST,
		Metadata: map[string]any{
			"called": true,
		},
	}
}

func TestPipelineNoPlugins(t *testing.T) {
	p := transform.New()
	got, err := p.Transform("echo hi\n")
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	// The spec expects parse → serialize is a no-op modulo printer
	// normalization. We pin the exact output the printer emits for
	// `echo hi\n` so any later printer-config change is loud.
	if got.Script != "echo hi\n" {
		t.Fatalf("script: got %q, want %q", got.Script, "echo hi\n")
	}
	if got.AST == nil {
		t.Fatal("AST should be non-nil")
	}
	if got.Metadata == nil {
		t.Fatal("Metadata should be non-nil (empty map)")
	}
	if len(got.Metadata) != 0 {
		t.Fatalf("Metadata should be empty: %v", got.Metadata)
	}
}

func TestPipelineUseChain(t *testing.T) {
	var trace []string
	p := transform.New().
		Use(&orderRecorder{name: "first", trace: &trace}).
		Use(&orderRecorder{name: "second", trace: &trace}).
		Use(&orderRecorder{name: "third", trace: &trace})

	got, err := p.Transform("true")
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if want := []string{"first", "second", "third"}; !equal(trace, want) {
		t.Fatalf("plugin order: got %v, want %v", trace, want)
	}
	for _, name := range []string{"first", "second", "third"} {
		m, ok := got.Metadata[name].(map[string]any)
		if !ok {
			t.Fatalf("Metadata[%q] missing or wrong type: %v", name, got.Metadata[name])
		}
		if m["called"] != true {
			t.Fatalf("Metadata[%q].called: %v", name, m["called"])
		}
	}
}

func TestPipelineUseNilIsSkipped(t *testing.T) {
	var trace []string
	p := transform.New().
		Use(nil).
		Use(&orderRecorder{name: "only", trace: &trace}).
		Use(nil)
	if _, err := p.Transform("true"); err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if len(trace) != 1 || trace[0] != "only" {
		t.Fatalf("trace: got %v, want [only]", trace)
	}
}

func TestPipelineParseError(t *testing.T) {
	p := transform.New()
	_, err := p.Transform("if then fi") // syntactically invalid
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "expected") && !strings.Contains(err.Error(), "if") {
		// the exact mvdan/sh phrasing is "expected command"; we
		// don't pin the exact text but want something error-ish.
		t.Logf("non-empty parse error: %v (allowed)", err)
	}
}

func TestPipelineMetadataAccumulates(t *testing.T) {
	// Verify that each plugin observes earlier plugins' metadata via
	// Context.Metadata.
	var observed []map[string]any
	first := pluginFunc{
		name: "first",
		fn: func(ctx transform.Context) transform.Result {
			return transform.Result{AST: ctx.AST, Metadata: map[string]any{"value": 1}}
		},
	}
	second := pluginFunc{
		name: "second",
		fn: func(ctx transform.Context) transform.Result {
			snap := map[string]any{}
			for k, v := range ctx.Metadata {
				snap[k] = v
			}
			observed = append(observed, snap)
			return transform.Result{AST: ctx.AST}
		},
	}
	if _, err := transform.New().Use(first).Use(second).Transform("true"); err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if len(observed) != 1 {
		t.Fatalf("observed runs: %d", len(observed))
	}
	got, ok := observed[0]["first"].(map[string]any)
	if !ok || got["value"] != 1 {
		t.Fatalf("observed metadata: %v", observed[0])
	}
}

func TestPipelinePluginsList(t *testing.T) {
	a := &orderRecorder{name: "a"}
	b := &orderRecorder{name: "b"}
	p := transform.New().Use(a).Use(b)
	list := p.Plugins()
	if len(list) != 2 {
		t.Fatalf("Plugins(): %v", list)
	}
	if list[0].Name() != "a" || list[1].Name() != "b" {
		t.Fatalf("order: %v", list)
	}
	// Mutating the returned slice must not affect internal state.
	list[0] = nil
	list2 := p.Plugins()
	if list2[0] == nil {
		t.Fatal("Plugins() should return a defensive copy")
	}
}

// equal compares two string slices for ordered equality.
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

// pluginFunc is an adapter that lets a test express a plugin inline
// as a closure.
type pluginFunc struct {
	name string
	fn   func(ctx transform.Context) transform.Result
}

func (p pluginFunc) Name() string                                  { return p.name }
func (p pluginFunc) Transform(ctx transform.Context) transform.Result { return p.fn(ctx) }
