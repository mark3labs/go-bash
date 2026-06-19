package transform

import "github.com/mark3labs/go-bash/ast"

// Plugin is the closed interface implemented by every transform-pipeline
// plugin. Plugins observe (and optionally rewrite) the AST of a script
// before it is handed to the runtime, and may attach plugin-scoped
// metadata that surfaces in BashExecResult.Metadata under the plugin's
// Name.
//
// # Mutating the AST
//
// The Context exposes the typed *ast.Script the parser produced. Most
// of go-bash's AST round-trip serialization is driven by Script.Origin
// (the *syntax.File the mvdan/sh parser produced) — see
// transform.Serialize. Plugins that need to rewrite source therefore
// mutate Origin in place (rather than rebuilding the typed AST). This
// keeps the inverse-translator surface small and lets the round-trip
// always go through mvdan/sh's printer for byte-faithful output.
//
// Plugins that only inspect the AST should return Result.AST == ctx.AST
// unchanged.
type Plugin interface {
	Name() string
	Transform(ctx Context) Result
}

// Context is the per-Exec, per-plugin invocation payload handed to
// Plugin.Transform. AST is the typed AST the parser produced. Metadata
// is the running bag accumulated by previous plugins in the pipeline,
// keyed by plugin Name — plugins may read prior plugins' results here.
type Context struct {
	AST      *ast.Script
	Metadata map[string]any
}

// Result is what a Plugin.Transform returns. AST is the (possibly same,
// possibly mutated) AST the pipeline continues with; nil is treated as
// "no change" and the prior AST flows through. Metadata is the plugin's
// own metadata payload — the pipeline stores it under the plugin's
// Name in the shared bag, exposed to subsequent plugins (via Context.Metadata)
// and to the host (via BashExecResult.Metadata).
type Result struct {
	AST      *ast.Script
	Metadata any
}

// BashTransformResult is the return type of Pipeline.Transform. Script
// is the re-serialized source after every plugin has run; AST is the
// final post-transform typed AST; Metadata is the keyed bag.
type BashTransformResult struct {
	Script   string
	AST      *ast.Script
	Metadata map[string]any
}
