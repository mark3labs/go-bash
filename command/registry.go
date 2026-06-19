package command

import "sort"

// Registry maps Name to Command. It is safe to read after
// construction; concurrent Register/Lookup is NOT supported (the
// runtime registers everything at New time and the registry is
// effectively frozen after that).
//
// Cited surface: SPEC §8.2. Reference (read-only):
// vercel-labs/just-bash, src/commands/registry.ts (CommandRegistry).
type Registry struct {
	cmds map[Name]Command
}

// NewRegistry returns an empty Registry. Phase 8 callers (gobash.New)
// build a registry, register CustomCommands first (so they win over
// later built-in registrations), and apply the BashOptions.Commands
// filter to whatever built-ins Phase 10 will add. Until Phase 10
// lands, only CustomCommands populate the registry.
//
// Named NewRegistry rather than New to avoid shadowing the
// gobash.New constructor for callers that dot-import this package.
func NewRegistry() *Registry {
	return &Registry{cmds: make(map[Name]Command)}
}

// Register adds c to the registry, overwriting any existing entry
// with the same Name. Caller order is therefore significant when
// CustomCommands need to win over built-ins: register customs LAST,
// or equivalently, register them first AND skip already-present
// names when adding built-ins.
//
// The Phase 8 runtime registers CustomCommands BEFORE built-ins (per
// SPEC §1.2 "override built-ins") and the built-in registration loop
// skips names already present — see gobash.New.
func (r *Registry) Register(c Command) {
	if c == nil {
		return
	}
	r.cmds[c.Name()] = c
}

// Lookup returns the Command registered under name, or (nil, false)
// if no such command exists. Name lookup is case-sensitive and
// matches the script's literal command word.
func (r *Registry) Lookup(name string) (Command, bool) {
	if r == nil {
		return nil, false
	}
	c, ok := r.cmds[Name(name)]
	return c, ok
}

// Names returns every registered command name, sorted for
// determinism. The sort matters: SPEC §7 materializes one /bin/X
// stub per registered name, and a stable order keeps the FS layout
// reproducible across runs.
func (r *Registry) Names() []Name {
	if r == nil {
		return nil
	}
	out := make([]Name, 0, len(r.cmds))
	for n := range r.cmds {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Has reports whether name is registered. Equivalent to discarding
// the Command return from Lookup, kept as a convenience for the
// "register only if not already present" loop the Phase 10 built-in
// bootstrap will use.
func (r *Registry) Has(name string) bool {
	_, ok := r.Lookup(name)
	return ok
}

// builtinsRegistry is the package-level slice of every built-in
// command registered via RegisterBuiltin. Built-in packages add
// themselves from init(); gobash.New consumes the slice via
// DefaultBuiltins to populate the runtime registry (with
// BashOptions.Commands acting as a filter and CustomCommands taking
// precedence). Phase 10's `builtins` meta-package side-effect
// imports every built-in so the slice is populated by the time
// gobash.New runs.
var builtinsRegistry []Command

// RegisterBuiltin appends c to the package-level built-in slice. The
// expected callsite is a built-in package's init() function; the
// runtime never calls this. Nil c is ignored. Concurrent calls are
// NOT safe — init() is the only intended caller, and Go orders
// init()s sequentially per goroutine.
func RegisterBuiltin(c Command) {
	if c == nil {
		return
	}
	builtinsRegistry = append(builtinsRegistry, c)
}

// DefaultBuiltins returns a snapshot of every Command registered via
// RegisterBuiltin. The order matches registration order, which is
// init-graph order — stable within a single binary, but callers must
// not depend on it beyond "every entry appears exactly once".
func DefaultBuiltins() []Command {
	out := make([]Command, len(builtinsRegistry))
	copy(out, builtinsRegistry)
	return out
}
