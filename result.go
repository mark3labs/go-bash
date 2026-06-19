package gobash

// ExecResult is the basic outcome of a script execution. A non-zero
// ExitCode is not, by itself, an error (see SPEC §0.1).
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// BashExecResult is the result type returned by Bash.Exec. It embeds
// ExecResult and adds the post-execution exported environment plus a
// free-form Metadata bag populated by registered transform plugins
// (Phase 13).
type BashExecResult struct {
	ExecResult
	Env      map[string]string
	Metadata map[string]any
}
