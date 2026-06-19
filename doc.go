// Package gobash provides a sandboxed bash environment with a virtual
// filesystem.
//
// gobash is a feature-for-feature Go port of the just-bash TypeScript
// interpreter (github.com/vercel-labs/just-bash). The public surface, the
// phase-by-phase build plan, and the resolved design decisions live in
// The spec at the repository root.
//
// # Phase 1 status
//
// Only the public skeleton is implemented: Bash, BashOptions, ExecOptions,
// BashExecResult, the limits/error types, and a stub Exec that delegates
// to mvdan.cc/sh/v3. The virtual filesystem (Phase 3), command registry
// (Phase 8), and network layer (Phase 9) are not yet wired. Until the
// virtual filesystem lands, file operations in scripts hit the host disk
// through mvdan/sh's default handlers.
//
// # Concurrency
//
// A *Bash is safe for concurrent Exec calls; calls serialize on an
// internal mutex.
package gobash
