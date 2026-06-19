// pipeline.go — the transform-plugin orchestrator.
//
// A Pipeline holds an ordered slice of Plugins. Pipeline.Transform
// parses the source, walks the slice (handing each plugin the running
// AST and metadata bag), serializes the post-transform AST back to
// source, and returns a BashTransformResult the caller can re-parse
// and run.
//
// Pipelines are constructed empty; callers append plugins with Use,
// which returns the same pipeline for fluent chaining. Pipelines are
// NOT safe for concurrent mutation; the Bash.Exec call site builds
// (or reuses) a Pipeline under its per-Bash mutex.

package transform

import (
	"fmt"

	"github.com/mark3labs/go-bash/ast"
	"github.com/mark3labs/go-bash/parser"
)

// Pipeline is the ordered plugin orchestrator.
type Pipeline struct {
	plugins []Plugin
}

// New returns an empty Pipeline.
func New() *Pipeline { return &Pipeline{} }

// Use appends a plugin to the pipeline and returns the same pipeline
// for fluent chaining: New().Use(a).Use(b).
func (p *Pipeline) Use(plugin Plugin) *Pipeline {
	if plugin == nil {
		return p
	}
	p.plugins = append(p.plugins, plugin)
	return p
}

// Plugins returns the registered plugin slice in registration order.
// The returned slice is a copy; the caller may mutate it freely.
func (p *Pipeline) Plugins() []Plugin {
	out := make([]Plugin, len(p.plugins))
	copy(out, p.plugins)
	return out
}

// Transform parses the given script, dispatches every registered
// plugin in order, then re-serializes the post-transform AST back to
// bash source. Returns the BashTransformResult bundle: the new
// Script, the post-transform AST, and the plugin-keyed Metadata bag.
//
// When zero plugins are registered, Transform is a thin parse+serialize
// pass-through (the script flows out untouched, modulo printer-driven
// whitespace normalization).
//
// Parse errors are returned as *parser.ParseError (aliased to
// gobash.ParseError at the top-level package).
func (p *Pipeline) Transform(script string) (*BashTransformResult, error) {
	parsed, err := parser.Parse(script)
	if err != nil {
		return nil, err
	}
	return p.TransformAST(parsed)
}

// TransformAST runs the pipeline on an already-parsed AST. The AST
// must have a non-nil Origin (i.e. it must come from parser.Parse);
// plugin-synthesized ASTs without an Origin can still be passed,
// but Serialize will fall back to the limited astToFile path.
func (p *Pipeline) TransformAST(parsed *ast.Script) (*BashTransformResult, error) {
	if parsed == nil {
		return nil, fmt.Errorf("transform: nil AST")
	}
	cur := parsed
	bag := map[string]any{}
	for _, pl := range p.plugins {
		res := pl.Transform(Context{AST: cur, Metadata: bag})
		if res.AST != nil {
			cur = res.AST
		}
		if res.Metadata != nil {
			bag[pl.Name()] = res.Metadata
		}
	}
	out, err := Serialize(cur)
	if err != nil {
		return nil, err
	}
	return &BashTransformResult{Script: out, AST: cur, Metadata: bag}, nil
}
