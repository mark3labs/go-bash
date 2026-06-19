package gobash

// Phase 10 wires the built-in command registry. The builtins
// meta-package side-effect imports every Wave A–H built-in via
// command.RegisterBuiltin, populating the package-level slice that
// New() drains into the per-Bash *command.Registry.
//
// Kept in a dedicated file so the blank import is obvious from the
// package's file listing — burying it inside bash.go (which already
// has a long import block) made the registration mechanism harder to
// trace during the Phase 10 build.
import _ "github.com/mark3labs/go-bash/builtins"
