// Package sandbox is an OPTIONAL convenience wrapper around *gobash.Bash
// that mimics the shape of Vercel's @vercel/sandbox SDK so code targeting
// that hosted runtime can swap to go-bash as a drop-in local mock.
//
// The package adds NO new security boundary on top of the one Bash
// already provides — the security model is the same VFS + the same
// network allow-list. The "sandbox" name reflects the SDK shape, not an
// extra layer of isolation.
//
// Cited surface: the spec
package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"mvdan.cc/sh/v3/syntax"

	gobash "github.com/mark3labs/go-bash"
	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/overlayfs"
	"github.com/mark3labs/go-bash/network"
)

// Options configures a Sandbox at construction time. FS and OverlayRoot
// are mutually exclusive: supply at most one. When both are empty the
// Sandbox uses the default in-memory filesystem.
type Options struct {
	// Cwd is the initial working directory inside the Sandbox.
	// Empty defers to the Bash default ("/home/user" when Files is
	// empty, "/" otherwise).
	Cwd string

	// Env seeds the persistent environment. Per-call RunCommand env
	// overrides this map.
	Env map[string]string

	// Timeout caps the wall-clock duration of a single RunCommand.
	// Zero disables the per-call timeout cap; ctx-side cancellation
	// still applies.
	Timeout time.Duration

	// FS is a host-supplied virtual filesystem. Mutually exclusive
	// with OverlayRoot. Nil + empty OverlayRoot → default memfs.
	FS gbfs.FileSystem

	// OverlayRoot mounts the named host directory read-only at /
	// inside the Sandbox, with writes captured in an in-memory
	// overlay. Mutually exclusive with FS.
	OverlayRoot string

	// Network is forwarded to BashOptions.Network. Nil leaves
	// network disabled.
	Network *network.Config
}

// ErrFSConflict is returned by Create when both FS and OverlayRoot
// are supplied. Use errors.Is to test for it.
var ErrFSConflict = errors.New("sandbox: FS and OverlayRoot are mutually exclusive")

// Sandbox wraps a *gobash.Bash and re-shapes its public API to match
// @vercel/sandbox. Concurrent RunCommand calls on the same Sandbox
// serialize through the underlying Bash mutex.
type Sandbox struct {
	b       *gobash.Bash
	timeout time.Duration
}

// Create constructs a Sandbox from opts. The ctx argument is reserved
// for API parity with the Vercel SDK; nothing in the construction
// path observes it today.
func Create(_ context.Context, opts Options) (*Sandbox, error) {
	if opts.FS != nil && opts.OverlayRoot != "" {
		return nil, ErrFSConflict
	}
	bopts := gobash.BashOptions{
		Cwd:     opts.Cwd,
		Env:     opts.Env,
		Network: opts.Network,
	}
	switch {
	case opts.OverlayRoot != "":
		ofs, err := overlayfs.New(overlayfs.Options{Root: opts.OverlayRoot})
		if err != nil {
			return nil, fmt.Errorf("sandbox: overlay: %w", err)
		}
		bopts.FS = ofs
	case opts.FS != nil:
		bopts.FS = opts.FS
	}
	b, err := gobash.New(bopts)
	if err != nil {
		return nil, fmt.Errorf("sandbox: new bash: %w", err)
	}
	return &Sandbox{
		b:       b,
		timeout: opts.Timeout,
	}, nil
}

// Bash returns the underlying *gobash.Bash. Hosts that need access to
// the full Bash surface (transform plugins, registry mutations,
// pre-test history seeding) can drop down through this accessor.
func (s *Sandbox) Bash() *gobash.Bash { return s.b }

// FS returns the underlying virtual filesystem.
func (s *Sandbox) FS() gbfs.FileSystem { return s.b.FS() }

// RunCommandParams describes a single command invocation. Sudo is
// accepted for API parity with @vercel/sandbox but has no effect —
// the in-process runtime has no privilege boundary to elevate
// across. Detached runs the Exec on a background goroutine; the
// caller drives completion via Command.Wait().
type RunCommandParams struct {
	Cmd      string
	Args     []string
	Cwd      string
	Env      map[string]string
	Sudo     bool
	Detached bool
	Stdin    io.Reader
}

// MkDirOptions configures Sandbox.MkDir. Recursive mirrors `mkdir -p`;
// Perm defaults to 0o755 when zero.
type MkDirOptions struct {
	Recursive bool
	Perm      os.FileMode
}

// Command is a live (or completed) command handle. Stdout/Stderr/Wait
// all block until the underlying Exec returns; for synchronous
// (non-detached) invocations the Exec has already returned by the
// time RunCommand returns, so those methods do not block.
type Command struct {
	done chan struct{}

	mu     sync.Mutex
	res    gobash.BashExecResult
	err    error
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

// Finished is the terminal state of a Command. ExitCode mirrors the
// underlying script's exit code (0 on success, non-zero otherwise).
type Finished struct {
	ExitCode int
}

// RunCommand executes p inside the Sandbox. The supplied Cmd plus
// Args are shell-quoted and joined into a single script line, which
// the Bash runtime parses and dispatches like any other script.
//
// When p.Detached is true the call returns immediately and the Exec
// runs on a background goroutine; Stdout/Stderr/Wait block until
// completion. When false (the default) the Exec runs synchronously
// before RunCommand returns.
func (s *Sandbox) RunCommand(ctx context.Context, p RunCommandParams) (*Command, error) {
	if p.Cmd == "" {
		return nil, errors.New("sandbox: RunCommand: empty Cmd")
	}
	script, err := buildScript(p.Cmd, p.Args)
	if err != nil {
		return nil, fmt.Errorf("sandbox: quote args: %w", err)
	}

	cmd := &Command{
		done:   make(chan struct{}),
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
	}
	eopts := gobash.ExecOptions{
		Cwd:    p.Cwd,
		Env:    p.Env,
		Stdin:  p.Stdin,
		Stdout: cmd.stdout,
		Stderr: cmd.stderr,
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if s.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, s.timeout)
	}

	run := func() {
		defer close(cmd.done)
		if cancel != nil {
			defer cancel()
		}
		res, err := s.b.Exec(runCtx, script, eopts)
		cmd.mu.Lock()
		cmd.res = res
		cmd.err = err
		cmd.mu.Unlock()
	}

	if p.Detached {
		go run()
	} else {
		run()
	}
	return cmd, nil
}

// WriteFiles writes every entry in files to the Sandbox's VFS. The
// containing directories are created (mode 0o755) as needed. The
// value is the file content; an empty string writes an empty file.
func (s *Sandbox) WriteFiles(_ context.Context, files map[string]string) error {
	fs := s.b.FS()
	for path, content := range files {
		if err := gbfs.Validate(path); err != nil {
			return fmt.Errorf("sandbox: WriteFiles %q: %w", path, err)
		}
		clean := gbfs.Clean(path)
		if dir := gbfs.Dirname(clean); dir != "." && dir != "/" {
			if err := fs.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("sandbox: WriteFiles %q: %w", path, err)
			}
		}
		if err := fs.WriteFile(clean, []byte(content), 0o644); err != nil {
			return fmt.Errorf("sandbox: WriteFiles %q: %w", path, err)
		}
	}
	return nil
}

// ReadFile reads path from the Sandbox's VFS and returns the content
// as a string.
func (s *Sandbox) ReadFile(_ context.Context, path string) (string, error) {
	if err := gbfs.Validate(path); err != nil {
		return "", fmt.Errorf("sandbox: ReadFile %q: %w", path, err)
	}
	data, err := s.b.FS().ReadFile(gbfs.Clean(path))
	if err != nil {
		return "", fmt.Errorf("sandbox: ReadFile %q: %w", path, err)
	}
	return string(data), nil
}

// MkDir creates path in the Sandbox's VFS. With opts.Recursive=true
// it behaves like `mkdir -p`; otherwise it errors if any parent
// component is missing.
func (s *Sandbox) MkDir(_ context.Context, path string, opts MkDirOptions) error {
	if err := gbfs.Validate(path); err != nil {
		return fmt.Errorf("sandbox: MkDir %q: %w", path, err)
	}
	perm := opts.Perm
	if perm == 0 {
		perm = 0o755
	}
	fs := s.b.FS()
	if opts.Recursive {
		if err := fs.MkdirAll(gbfs.Clean(path), perm); err != nil {
			return fmt.Errorf("sandbox: MkDir %q: %w", path, err)
		}
		return nil
	}
	if err := fs.Mkdir(gbfs.Clean(path), perm); err != nil {
		return fmt.Errorf("sandbox: MkDir %q: %w", path, err)
	}
	return nil
}

// Stop is a no-op kept for API parity with @vercel/sandbox. The
// in-process runtime has no live container to tear down.
func (s *Sandbox) Stop(_ context.Context) error { return nil }

// Stdout blocks until the underlying Exec completes and returns the
// captured stdout. If the Exec returned an error other than a
// non-zero exit code, that error is returned alongside the partial
// buffer.
func (c *Command) Stdout() (string, error) {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stdout.String(), c.err
}

// Stderr blocks until the underlying Exec completes and returns the
// captured stderr.
func (c *Command) Stderr() (string, error) {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stderr.String(), c.err
}

// Wait blocks until the underlying Exec completes and returns the
// terminal Finished state. A non-nil error indicates the Exec itself
// failed (parse error, limit overrun, ctx cancellation, etc.); the
// script's own non-zero exit code is reported via Finished.ExitCode,
// not via the error return.
func (c *Command) Wait() (Finished, error) {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return Finished{ExitCode: c.res.ExitCode}, c.err
}

// buildScript joins cmd and args into a single shell-quoted script
// line. Empty Args yields the bare command. Each arg is quoted via
// mvdan/sh's syntax.Quote so embedded spaces, $, *, etc. survive
// round-trip into the parser without unintended expansion.
func buildScript(cmd string, args []string) (string, error) {
	var sb strings.Builder
	// Quote the command word too — it could in principle contain
	// special chars (path with spaces). syntax.Quote leaves
	// already-safe words alone.
	q, err := syntax.Quote(cmd, syntax.LangBash)
	if err != nil {
		return "", err
	}
	sb.WriteString(q)
	for _, a := range args {
		sb.WriteByte(' ')
		q, err := syntax.Quote(a, syntax.LangBash)
		if err != nil {
			return "", err
		}
		sb.WriteString(q)
	}
	return sb.String(), nil
}
