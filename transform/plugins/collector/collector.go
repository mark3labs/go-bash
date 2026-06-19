// Package collector implements the CommandCollectorPlugin: a read-only
// transform plugin that walks the parsed AST and reports the command
// name of every leaf SimpleCommand it encounters.
//
// The plugin does NOT mutate the AST. Its metadata payload is a
// `[]string` of command names, in source order, stored under the
// plugin's Name() ("command-collector") in the pipeline metadata bag
// and surfaced from Bash.Exec via BashExecResult.Metadata.
package collector

import (
	"mvdan.cc/sh/v3/syntax"

	"github.com/mark3labs/go-bash/transform"
)

// Name is the metadata key the plugin uses.
const Name = "command-collector"

// MetadataKey is the JSON-style key for the collected list within the
// plugin's metadata payload. The full plugin payload is exposed under
//
//	transform.BashTransformResult.Metadata[Name] = map[string]any{
//	    MetadataKey: []string{...},
//	}.
const MetadataKey = "commands"

// Plugin is the CommandCollectorPlugin instance. Stateless; safe to
// share across pipelines.
type Plugin struct{}

// New returns a fresh CommandCollectorPlugin.
func New() *Plugin { return &Plugin{} }

// Name implements transform.Plugin.
func (*Plugin) Name() string { return Name }

// Transform walks the script's Origin syntax tree and collects the
// first-argument literal of every CallExpr (which corresponds to a
// SimpleCommand in our typed AST). Compound commands (if / while /
// subshell / etc.) are recursed into transparently by mvdan/sh's
// syntax.Walk; their bodies' SimpleCommands are collected too.
func (*Plugin) Transform(ctx transform.Context) transform.Result {
	if ctx.AST == nil || ctx.AST.Origin == nil {
		return transform.Result{AST: ctx.AST, Metadata: map[string]any{MetadataKey: []string{}}}
	}
	var names []string
	syntax.Walk(ctx.AST.Origin, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		if name := transform.WordLit(call.Args[0]); name != "" {
			names = append(names, name)
		}
		return true
	})
	if names == nil {
		names = []string{}
	}
	return transform.Result{
		AST: ctx.AST,
		Metadata: map[string]any{
			MetadataKey: names,
		},
	}
}
