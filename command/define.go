package command

import "context"

// Define wraps a plain function in a Command, mirroring the TS
// `defineCommand` helper from the spec The resulting Command's
// Name is the supplied string; Trusted is true (the sandbox-untrust
// case is opt-in via DefineUntrusted once Phase 17 needs it).
//
// Define is the recommended way to construct simple commands in
// tests and in the Phase 10 built-in packages; it keeps the
// boilerplate of the Command interface out of every package init.
func Define(name string, fn func(ctx context.Context, args []string, c *Context) Result) Command {
	return &funcCommand{name: Name(name), fn: fn, trusted: true}
}

type funcCommand struct {
	name    Name
	fn      func(ctx context.Context, args []string, c *Context) Result
	trusted bool
}

func (f *funcCommand) Name() Name { return f.name }
func (f *funcCommand) Execute(ctx context.Context, args []string, c *Context) Result {
	return f.fn(ctx, args, c)
}
func (f *funcCommand) Trusted() bool { return f.trusted }
