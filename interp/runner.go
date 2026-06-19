// Package interp builds and configures the mvdan.cc/sh/v3/interp runner
// with the gobash customization hooks. It is the bridge between the
// public gobash.Bash surface and mvdan/sh's tree-walking interpreter:
//
//   - VFS-backed Open/Stat/ReadDir handlers route every file operation
//     through the FileSystem on the caller's *Bash instead of the host
//     disk.
//   - An ExecHandlers middleware dispatches every non-mvdan-builtin
//     command through the gobash command Registry. The
//     middleware NEVER falls through to mvdan/sh's DefaultExecHandler;
//     unregistered commands trigger a `command not found` stderr write
//     plus ExitStatus(127), closing the os/exec gap that existed in
//     Phases 5–7. The Phase 8 acceptance test
//     `TestPhase8UnknownCommandIsNotFound` locks this in.
//   - Env, Cwd, and the three stdio streams are passed in via Config;
//     the caller is responsible for wrapping stdout/stderr in the
//     MaxOutputSize ringbuf writer before BuildRunner sees them.
//
// # Import cycle avoidance
//
// Phase 5's kickoff suggested a signature of
// `BuildRunner(ctx, *Bash, ExecOptions)`. Because gobash imports this
// package and *gobash.Bash lives in the root package, taking *Bash here
// would create a cycle. Instead we accept a flat Config struct that the
// caller (gobash.(*Bash).Exec) assembles from the *Bash + ExecOptions
// inputs. The semantic intent (wire env / cwd / streams / FS /
// callHandler into the runner) is identical.
//
// # Phase 3 mvdan/sh quirks (still load-bearing)
//
//   - interp.Dir(path) runs an os.Stat(path) on the host disk at
//     runner-init time. We never use it; we set runner.Dir = cwd
//     directly after interp.New returns.
//   - interp.HandlerCtx(ctx) panics when ctx has no HandlerContext.
//     mvdan/sh's Runner.stat / Runner.lstat path call our handlers with
//     the bare ctx during early init. HandlerDir below recovers and
//     falls back to empty, which gbfs.Resolve handles correctly for
//     absolute inputs.
package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"strings"

	"mvdan.cc/sh/v3/expand"
	mvinterp "mvdan.cc/sh/v3/interp"

	"github.com/mark3labs/go-bash/command"
	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/network"
)

// Config carries everything BuildRunner needs to construct a runner.
// The caller — typically gobash.(*Bash).Exec — assembles this from the
// *Bash + ExecOptions inputs. All fields except CallHandler are
// required; the zero value of Config is not usable.
type Config struct {
	// Env is the initial environment as a slice of KEY=VAL pairs,
	// matching expand.ListEnviron's input format. Nil is treated as
	// an empty environment.
	Env []string

	// Cwd is the script's working directory. Resolved through the VFS
	// (NOT the host disk) — see the package-level Phase 3 quirk note.
	// Empty string leaves the runner's default Dir unchanged.
	Cwd string

	// Stdin / Stdout / Stderr are the three streams handed to
	// interp.StdIO. Nil Stdin is treated as an EOF reader so scripts
	// that try to read still terminate cleanly. Nil Stdout / Stderr
	// are illegal — the caller must supply at least a discard sink
	// (e.g. the MaxOutputSize ringbuf writer wrapping io.Discard).
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// FS is the virtual filesystem backing the Open / Stat / ReadDir
	// handlers. Required: nil triggers an explicit error so a missing
	// wire-up surfaces immediately instead of silently shelling out to
	// the host disk.
	FS gbfs.FileSystem

	// CallHandler is the per-command interception point used today by
	// the limit accounting (MaxLoopIterations / MaxCommandCount /
	// MaxCallDepth). Nil disables call-handler wiring; mvdan/sh's
	// default behavior takes over.
	CallHandler mvinterp.CallHandlerFunc

	// Registry is the command dispatch registry. Nil is treated as an
	// empty registry — every non-mvdan-builtin command will resolve
	// to `command not found` (ExitStatus 127). The runtime supplies a
	// fully-populated *command.Registry; tests in this package omit
	// the field deliberately so the not-found path is exercised in
	// isolation.
	Registry *command.Registry

	// Fetch is the resolved network Doer. When nil, command.Context.Fetch
	// is also nil and network-touching commands must error cleanly.
	// The spec: the runtime decides whether to supply a SecureFetch or
	// honor a host-provided override; the dispatch bridge just passes
	// through whatever the host gave it.
	Fetch network.Doer

	// Sleep is the host-supplied sleep hook surfaced to commands via
	// command.Context.Sleep (Phase 10 Wave A's `sleep` is the first
	// consumer). Nil leaves command.Context.Sleep nil; the builtin
	// falls back to time.Sleep.
	Sleep command.SleepFunc

	// Trace is the host-supplied trace hook surfaced to commands via
	// command.Context.Trace. Wave A does not emit events; the field
	// is plumbed today so later waves can without a follow-up
	// Context surface bump.
	Trace command.TraceFunc

	// Limits is the per-Exec resolved limit set surfaced to commands
	// via command.Context.Limits. Wave A built-ins ignore it; Wave D
	// and the optional runtimes are the first real consumers.
	Limits command.Limits

	// ExportedEnv is the per-dispatch exported-env view surfaced to
	// commands via command.Context.ExportedEnv. Wave A does not
	// consume it; landing zone for env/printenv/export (Wave G).
	ExportedEnv map[string]string

	// Aliases is the per-Bash alias table surfaced to commands via
	// command.Context.Aliases (Phase 10 Wave G `alias` / `unalias`).
	// The runtime always supplies a live table; a nil interface is
	// not safe to call through.
	Aliases command.AliasTable

	// History is the per-Bash command-history ring surfaced to
	// commands via command.Context.History (Phase 10 Wave G
	// `history`).
	History command.HistoryRing

	// Exec is the sub-shell invocation hook surfaced to commands via
	// command.Context.Exec (Phase 10 Wave G `bash` / `sh` /
	// `timeout`). The spec reserves this for Phase 11; Wave G needs
	// it earlier.
	Exec command.SubExecFunc

	// SourceDepth is the depth this dispatch starts at. Used by the
	// Phase 11 source / eval / . / bash / sh / timeout builtins to
	// enforce Limits.MaxSourceDepth across nested invocations.
	SourceDepth int

	// Shopt is the per-Bash shopt table surfaced to commands via
	// command.Context.Shopt (Phase 11). Concrete implementation in
	// internal/runtimestate.
	Shopt command.ShoptTable

	// ReadDirHook, if non-nil, is invoked before every VFS ReadDir.
	// Returning a non-nil error fails the ReadDir without consulting
	// the underlying FS — this is the Phase 6 wire-up point for the
	// MaxGlobOperations limit (every ReadDir contributes one glob op).
	// The spec routes pathname expansion through this hook because
	// mvdan/sh does not expose a glob-specific counter.
	ReadDirHook func(ctx context.Context, path string) error
}

// BuildRunner constructs a fully-configured *mvinterp.Runner.
//
// ctx is reserved for future use (mvdan/sh's constructor takes no ctx
// today) and is intentionally part of the public signature so the
// eventual richer hookpoint — e.g. cancelling pending I/O during
// runner-init — does not require a breaking change.
func BuildRunner(ctx context.Context, cfg Config) (*mvinterp.Runner, error) {
	_ = ctx
	if cfg.FS == nil {
		return nil, errors.New("interp: Config.FS is required")
	}
	if cfg.Stdout == nil || cfg.Stderr == nil {
		return nil, errors.New("interp: Config.Stdout and Config.Stderr are required")
	}
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = eofReader{}
	}

	opts := []mvinterp.RunnerOption{
		mvinterp.StdIO(stdin, cfg.Stdout, cfg.Stderr),
		mvinterp.Env(expand.ListEnviron(cfg.Env...)),
		// Registry dispatch: every non-mvdan-builtin
		// command goes through the registry. The chain terminates in
		// notFoundExecHandler; mvdan/sh's DefaultExecHandler is
		// NEVER reached, so no os/exec call escapes to the host.
		mvinterp.ExecHandlers(registryDispatchMiddleware(cfg.Registry, dispatchEnv{
			fs:          cfg.FS,
			fetch:       cfg.Fetch,
			sleep:       cfg.Sleep,
			trace:       cfg.Trace,
			limits:      cfg.Limits,
			exportedEnv: cfg.ExportedEnv,
			aliases:     cfg.Aliases,
			history:     cfg.History,
			exec:        cfg.Exec,
			sourceDepth: cfg.SourceDepth,
			shopt:       cfg.Shopt,
		})),
		mvinterp.OpenHandler(openHandler(cfg.FS)),
		mvinterp.StatHandler(statHandler(cfg.FS)),
		mvinterp.ReadDirHandler2(readDirHandler(cfg.FS, cfg.ReadDirHook)),
	}
	if cfg.CallHandler != nil {
		opts = append(opts, mvinterp.CallHandler(cfg.CallHandler))
	}

	runner, err := mvinterp.New(opts...)
	if err != nil {
		return nil, err
	}

	// The spec / Phase 3 quirk: do NOT use interp.Dir(cwd) — that
	// runs an os.Stat() on the host disk at runner-init time. Setting
	// runner.Dir directly bypasses that check and trusts our VFS to
	// validate the cwd lazily via the StatHandler.
	if cfg.Cwd != "" {
		runner.Dir = cfg.Cwd
	}
	return runner, nil
}

// dispatchEnv bundles the per-Exec values that every dispatched
// command sees on its command.Context. Bundled rather than passed
// pairwise so adding a future field (Phase 11 Exec/Signal, Phase 15
// JSBootstrap/InvokeTool) does not churn every signature in the
// middleware chain.
type dispatchEnv struct {
	fs          gbfs.FileSystem
	fetch       network.Doer
	sleep       command.SleepFunc
	trace       command.TraceFunc
	limits      command.Limits
	exportedEnv map[string]string
	aliases     command.AliasTable
	history     command.HistoryRing
	exec        command.SubExecFunc
	sourceDepth int
	shopt       command.ShoptTable
}

// registryDispatchMiddleware returns the ExecHandlers middleware that
// resolves every non-mvdan-builtin command through reg. The returned
// middleware ignores `next` for unregistered commands; mvdan/sh's
// DefaultExecHandler (host os/exec) is therefore never reached. The spec
// §5.3 and §8 specify this contract.
func registryDispatchMiddleware(reg *command.Registry, env dispatchEnv) func(next mvinterp.ExecHandlerFunc) mvinterp.ExecHandlerFunc {
	return func(next mvinterp.ExecHandlerFunc) mvinterp.ExecHandlerFunc {
		// next is intentionally unused: the spec requires we close
		// the os/exec fall-through. Keeping it in the signature
		// preserves ExecHandlers' chain-of-middlewares shape so a
		// future phase can prepend tracing or limits middleware
		// without restructuring BuildRunner.
		_ = next
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return nil
			}
			if cmd, ok := lookupCommand(reg, args[0]); ok {
				return dispatchCommand(ctx, cmd, args, reg, env)
			}
			return notFoundExecHandler(ctx, args)
		}
	}
}

// lookupCommand resolves name against reg, trying the literal name
// first and then the basename if name is an absolute path under one
// of the spec stub directories (/bin, /usr/bin).
func lookupCommand(reg *command.Registry, name string) (command.Command, bool) {
	if reg == nil {
		return nil, false
	}
	if cmd, ok := reg.Lookup(name); ok {
		return cmd, true
	}
	if strings.HasPrefix(name, "/bin/") || strings.HasPrefix(name, "/usr/bin/") {
		base := path.Base(name)
		if base != "" && base != "/" && base != "." {
			if cmd, ok := reg.Lookup(base); ok {
				return cmd, true
			}
		}
	}
	return nil, false
}

// dispatchCommand invokes cmd.Execute with a Context built from the
// HandlerContext currently stored on ctx, then flushes any fallback
// strings from the returned Result and translates the ExitCode.
func dispatchCommand(ctx context.Context, cmd command.Command, args []string, reg *command.Registry, env dispatchEnv) error {
	hc := mvinterp.HandlerCtx(ctx)
	// Materialize the per-call env view (which includes any KEY=VAL
	// prefixes mvdan/sh parsed in front of the command word) into a
	// plain map. Built-ins read c.Env to honor those overrides.
	callEnv := make(map[string]string)
	if hc.Env != nil {
		hc.Env.Each(func(name string, vr expand.Variable) bool {
			callEnv[name] = vr.String()
			return true
		})
	}
	cctx := &command.Context{
		FS:          env.fs,
		Cwd:         hc.Dir,
		Env:         callEnv,
		Stdin:       hc.Stdin,
		Stdout:      hc.Stdout,
		Stderr:      hc.Stderr,
		Registry:    reg,
		Fetch:       env.fetch,
		Sleep:       env.sleep,
		Trace:       env.trace,
		Limits:      env.limits,
		ExportedEnv: env.exportedEnv,
		Aliases:     env.aliases,
		History:     env.history,
		Exec:        env.exec,
		SourceDepth: env.sourceDepth,
		Shopt:       env.shopt,
	}
	res := cmd.Execute(ctx, args, cctx)
	if res.Stdout != "" && hc.Stdout != nil {
		if _, err := io.WriteString(hc.Stdout, res.Stdout); err != nil {
			return err
		}
	}
	if res.Stderr != "" && hc.Stderr != nil {
		if _, err := io.WriteString(hc.Stderr, res.Stderr); err != nil {
			return err
		}
	}
	if res.ExitCode == 0 {
		return nil
	}
	return mvinterp.ExitStatus(clampExit(res.ExitCode))
}

// notFoundExecHandler is the chain terminator. The spec mandates this
// path NEVER reach host os/exec. We emit the canonical `command not
// found` diagnostic and exit 127, matching real bash.
func notFoundExecHandler(ctx context.Context, args []string) error {
	hc := mvinterp.HandlerCtx(ctx)
	if hc.Stderr != nil {
		_, _ = fmt.Fprintf(hc.Stderr, "%s: command not found\n", args[0])
	}
	return mvinterp.ExitStatus(127)
}

// clampExit narrows res.ExitCode into the uint8 mvdan/sh expects.
// Anything < 0 or > 255 collapses to 1 (the conventional "generic
// failure" code) rather than silently wrapping mod 256, which would
// turn a legitimate error into a deceptive success.
func clampExit(code int) uint8 {
	if code < 0 || code > 255 {
		return 1
	}
	return uint8(code)
}

// HandlerDir returns the current Dir from the HandlerContext stored in
// ctx, or empty string if mvdan/sh has not injected one yet.
//
// mvdan/sh's Runner.stat / Runner.lstat call our StatHandler with the
// bare ctx (no HandlerContext attached), and interp.HandlerCtx panics
// in that case. The defer/recover absorbs the panic and falls back to
// empty — which gbfs.Resolve treats correctly for absolute paths.
//
// Exported so tests in the gobash root package can re-use the exact
// same guard.
func HandlerDir(ctx context.Context) string {
	defer func() { _ = recover() }()
	return mvinterp.HandlerCtx(ctx).Dir
}

func openHandler(fs gbfs.FileSystem) mvinterp.OpenHandlerFunc {
	return func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		resolved := gbfs.Resolve(HandlerDir(ctx), path)
		f, err := fs.OpenFile(resolved, flag, perm)
		if err != nil {
			return nil, ensurePathError(err, "open", path)
		}
		return f, nil
	}
}

func statHandler(fs gbfs.FileSystem) mvinterp.StatHandlerFunc {
	return func(ctx context.Context, name string, followSymlinks bool) (iofs.FileInfo, error) {
		resolved := gbfs.Resolve(HandlerDir(ctx), name)
		if followSymlinks {
			fi, err := fs.Stat(resolved)
			return fi, ensurePathError(err, "stat", name)
		}
		fi, err := fs.Lstat(resolved)
		return fi, ensurePathError(err, "lstat", name)
	}
}

func readDirHandler(fs gbfs.FileSystem, hook func(ctx context.Context, path string) error) mvinterp.ReadDirHandlerFunc2 {
	return func(ctx context.Context, path string) ([]iofs.DirEntry, error) {
		if hook != nil {
			if err := hook(ctx, path); err != nil {
				return nil, err
			}
		}
		resolved := gbfs.Resolve(HandlerDir(ctx), path)
		entries, err := fs.ReadDir(resolved)
		return entries, ensurePathError(err, "readdir", path)
	}
}

// ensurePathError normalizes an FS error into the *os.PathError shape
// mvdan/sh expects so file-related diagnostics look like a real shell.
// Nil err passes through unchanged.
func ensurePathError(err error, op, path string) error {
	if err == nil {
		return nil
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		return err
	}
	return &os.PathError{Op: op, Path: path, Err: err}
}

// eofReader is the implicit stdin used when Config.Stdin is nil. A
// strings.NewReader("") would do the same job; eofReader avoids
// reaching for the strings import for a one-off stub.
type eofReader struct{}

func (eofReader) Read(p []byte) (int, error) { return 0, io.EOF }
