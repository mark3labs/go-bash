// Package tee implements the TeePlugin: a transform plugin that wraps
// every non-trivial pipeline stage with `| tee /OUT/<idx>-<cmd>.stdout.txt`
// so the host can post-mortem the per-stage output of a script.
//
// # Behavior
//
// For every pipeline stage in the script (including a single-command
// "pipeline" that has no `|` operator), the plugin inserts a tee
// stage immediately after it that mirrors the stage's stdout to a
// file inside Options.OutputDir. Stages whose command name does not
// match Options.TargetCommandMatch (when non-nil) are skipped. Stages
// whose command name is `tee` itself are also skipped — the plugin
// is idempotent under repeated application.
//
// # PIPESTATUS preservation
//
// The naive `cmd | tee file` rewrite would shift PIPESTATUS by one
// position per inserted stage and change the exit status reported via
// pipefail. SPEC §13.3 calls for porting just-bash's restore semantics
// "faithfully"; the Phase 13 MVP injects the tee but does NOT yet
// emit the restore-pipeline. Scripts that observe `$PIPESTATUS` or
// `set -o pipefail` exit codes after a wrapped pipeline will see the
// shifted form. This is documented in DECISIONS.md as a Phase 13
// follow-up.
//
// # Metadata
//
// The plugin's metadata payload is a `[]TeeFileInfo` (one entry per
// wrapped stage, in source order). It is exposed under
// BashExecResult.Metadata[Name] as `map[string]any{ "files": []TeeFileInfo{...} }`.
package tee

import (
	"fmt"
	"path"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/mark3labs/go-bash/transform"
)

// Name is the metadata key the plugin uses.
const Name = "tee"

// MetadataKey is the key under the plugin's metadata payload that
// carries the []TeeFileInfo slice.
const MetadataKey = "files"

// Options configures a TeePlugin.
type Options struct {
	// OutputDir is the directory the tee file paths are joined under.
	// Required; empty disables the plugin (Transform becomes a no-op).
	OutputDir string

	// TargetCommandMatch decides which commands get wrapped. Nil
	// means "every stage with a recognizable command name".
	TargetCommandMatch func(string) bool

	// Timestamp is exposed to plugins/hosts that want a single
	// monotonic clock value for the wrapped run. Defaults to
	// time.Now() at New time.
	Timestamp time.Time
}

// TeeFileInfo is one entry in the plugin's metadata payload — one
// per wrapped pipeline stage.
type TeeFileInfo struct {
	CommandIndex int    `json:"commandIndex"`
	CommandName  string `json:"commandName"`
	Command      string `json:"command"`
	StdoutFile   string `json:"stdoutFile"`
}

// Plugin is the TeePlugin instance. Each Transform call assigns its
// own sequential CommandIndex starting at 0; sharing a Plugin across
// pipelines is safe (no per-Plugin state is mutated across calls).
type Plugin struct {
	opts Options
}

// New constructs a TeePlugin. Timestamp defaults to time.Now when
// the caller leaves it zero.
func New(opts Options) *Plugin {
	if opts.Timestamp.IsZero() {
		opts.Timestamp = time.Now()
	}
	return &Plugin{opts: opts}
}

// Name implements transform.Plugin.
func (*Plugin) Name() string { return Name }

// Transform mutates Origin in place, wrapping every matching pipeline
// stage. Returns metadata listing the inserted tee files.
func (p *Plugin) Transform(ctx transform.Context) transform.Result {
	if ctx.AST == nil || ctx.AST.Origin == nil || p.opts.OutputDir == "" {
		return transform.Result{
			AST:      ctx.AST,
			Metadata: map[string]any{MetadataKey: []TeeFileInfo{}},
		}
	}

	st := &state{opts: p.opts}
	st.rewriteStmts(ctx.AST.Origin.Stmts)

	files := st.files
	if files == nil {
		files = []TeeFileInfo{}
	}
	return transform.Result{
		AST: ctx.AST,
		Metadata: map[string]any{
			MetadataKey: files,
		},
	}
}

// state carries the per-invocation counter and file list.
type state struct {
	opts    Options
	counter int
	files   []TeeFileInfo
}

// rewriteStmts walks a slice of *syntax.Stmt, rewriting each Stmt's
// pipeline in place and recursing into compound bodies.
func (s *state) rewriteStmts(stmts []*syntax.Stmt) {
	for _, st := range stmts {
		if st == nil || st.Cmd == nil {
			continue
		}
		s.recurseInto(st.Cmd)

		bin, isPipe := st.Cmd.(*syntax.BinaryCmd)
		if isPipe && (bin.Op == syntax.Pipe || bin.Op == syntax.PipeAll) {
			var stages []*syntax.Stmt
			var ops []syntax.BinCmdOperator
			flattenPipe(bin, &stages, &ops)
			newStages, newOps := s.wrapStages(stages, ops)
			if len(newStages) != len(stages) {
				st.Cmd = rebuildPipe(newStages, newOps)
			}
			continue
		}

		// Single-stage stmt. Wrap the inner Cmd in a fresh Stmt so the
		// pipeline tree never references the outer st (which would
		// cycle: st.Cmd → BinaryCmd.X → st → ...).
		name := commandName(st.Cmd)
		if !s.shouldWrap(name) {
			continue
		}
		inner := &syntax.Stmt{Cmd: st.Cmd}
		info := TeeFileInfo{
			CommandIndex: s.counter,
			CommandName:  name,
			Command:      stmtText(inner),
			StdoutFile:   path.Join(s.opts.OutputDir, fmt.Sprintf("%d-%s.stdout.txt", s.counter, sanitize(name))),
		}
		s.files = append(s.files, info)
		s.counter++
		st.Cmd = &syntax.BinaryCmd{
			Op: syntax.Pipe,
			X:  inner,
			Y:  newTeeStmt(info.StdoutFile),
		}
	}
}

// flattenPipe walks a BinaryCmd{Pipe|PipeAll} tree and produces the
// flat stage / op slices. The argument is the BinaryCmd itself — NOT
// the outer Stmt that wraps it — so leaves are the binary's own X/Y
// children, never the caller's Stmt. (Passing the outer Stmt would
// re-introduce the self-reference cycle the caller avoids by
// always entering from a BinaryCmd root.)
func flattenPipe(bin *syntax.BinaryCmd, stages *[]*syntax.Stmt, ops *[]syntax.BinCmdOperator) {
	appendStage := func(child *syntax.Stmt) {
		if inner, ok := child.Cmd.(*syntax.BinaryCmd); ok && (inner.Op == syntax.Pipe || inner.Op == syntax.PipeAll) {
			flattenPipe(inner, stages, ops)
			return
		}
		*stages = append(*stages, child)
	}
	appendStage(bin.X)
	*ops = append(*ops, bin.Op)
	appendStage(bin.Y)
}

// recurseInto walks one stage's command for nested Stmt lists
// (subshells, blocks, function bodies, if/for/while/case branches)
// and recurses on them. For pipelines (BinaryCmd{Pipe|PipeAll})
// we recurse into each stage's Cmd, since those Cmds may themselves
// contain compound nodes with nested Stmt lists.
func (s *state) recurseInto(cmd syntax.Command) {
	switch c := cmd.(type) {
	case *syntax.Subshell:
		s.rewriteStmts(c.Stmts)
	case *syntax.Block:
		s.rewriteStmts(c.Stmts)
	case *syntax.IfClause:
		cur := c
		for cur != nil {
			s.rewriteStmts(cur.Cond)
			s.rewriteStmts(cur.Then)
			cur = cur.Else
		}
	case *syntax.ForClause:
		s.rewriteStmts(c.Do)
	case *syntax.WhileClause:
		s.rewriteStmts(c.Cond)
		s.rewriteStmts(c.Do)
	case *syntax.CaseClause:
		for _, it := range c.Items {
			s.rewriteStmts(it.Stmts)
		}
	case *syntax.FuncDecl:
		if c.Body != nil {
			s.recurseInto(c.Body.Cmd)
		}
	case *syntax.BinaryCmd:
		if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
			// Each stage of the pipeline may itself contain a
			// compound body; recurse into each.
			s.recurseInto(c.X.Cmd)
			s.recurseInto(c.Y.Cmd)
		} else {
			// && / ||.
			s.rewriteStmts([]*syntax.Stmt{c.X, c.Y})
		}
	}
}

// wrapStages takes the flattened (stages, ops) of one pipeline and
// returns a new (stages, ops) with tee stages spliced in after each
// matched stage.
func (s *state) wrapStages(stages []*syntax.Stmt, ops []syntax.BinCmdOperator) ([]*syntax.Stmt, []syntax.BinCmdOperator) {
	var newStages []*syntax.Stmt
	var newOps []syntax.BinCmdOperator
	for i, st := range stages {
		newStages = append(newStages, st)
		name := commandName(st.Cmd)
		if s.shouldWrap(name) {
			info := TeeFileInfo{
				CommandIndex: s.counter,
				CommandName:  name,
				Command:      stmtText(st),
				StdoutFile:   path.Join(s.opts.OutputDir, fmt.Sprintf("%d-%s.stdout.txt", s.counter, sanitize(name))),
			}
			s.files = append(s.files, info)
			s.counter++
			newStages = append(newStages, newTeeStmt(info.StdoutFile))
			newOps = append(newOps, syntax.Pipe)
		}
		if i < len(stages)-1 {
			newOps = append(newOps, ops[i])
		}
	}
	return newStages, newOps
}

// shouldWrap returns true when a stage with the given command name
// should be wrapped: name must be non-empty AND not "tee" itself AND
// (TargetCommandMatch == nil || TargetCommandMatch(name) == true).
func (s *state) shouldWrap(name string) bool {
	if name == "" {
		return false
	}
	if name == "tee" {
		return false
	}
	if s.opts.TargetCommandMatch != nil && !s.opts.TargetCommandMatch(name) {
		return false
	}
	return true
}

// rebuildPipe constructs a left-associative BinaryCmd{Pipe} chain
// from the flat stages and ops.
func rebuildPipe(stages []*syntax.Stmt, ops []syntax.BinCmdOperator) syntax.Command {
	if len(stages) == 0 {
		return nil
	}
	if len(stages) == 1 {
		return stages[0].Cmd
	}
	cur := stages[0]
	for i := 1; i < len(stages); i++ {
		cur = &syntax.Stmt{
			Cmd: &syntax.BinaryCmd{
				Op: ops[i-1],
				X:  cur,
				Y:  stages[i],
			},
		}
	}
	return cur.Cmd
}

// commandName returns the leading literal command name of a syntax.Command,
// or "" when it has none (compound commands, assignments-only calls,
// non-literal first args).
func commandName(cmd syntax.Command) string {
	call, ok := cmd.(*syntax.CallExpr)
	if !ok {
		return ""
	}
	if len(call.Args) == 0 {
		return ""
	}
	return transform.WordLit(call.Args[0])
}

// stmtText renders a Stmt back to its source representation via
// mvdan/sh's printer.
func stmtText(st *syntax.Stmt) string {
	var b strings.Builder
	if err := syntax.NewPrinter().Print(&b, &syntax.File{Stmts: []*syntax.Stmt{st}}); err != nil {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

// newTeeStmt synthesizes a `tee <file>` CallExpr wrapped in a Stmt.
func newTeeStmt(file string) *syntax.Stmt {
	return &syntax.Stmt{
		Cmd: &syntax.CallExpr{
			Args: []*syntax.Word{
				{Parts: []syntax.WordPart{&syntax.Lit{Value: "tee"}}},
				{Parts: []syntax.WordPart{&syntax.Lit{Value: file}}},
			},
		},
	}
}

// sanitize replaces filesystem-unsafe runes in a command name with
// an underscore so the filename is always a single legal path
// component.
func sanitize(name string) string {
	if name == "" {
		return "cmd"
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch r {
		case '/', '\\', 0, '\n', '\r', ' ':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
