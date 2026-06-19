# go-bash

A virtual bash environment with an in-memory filesystem, written in Go
and designed for AI agents.

Broad support for standard Unix commands and bash syntax with optional
network access, SQLite, and (planned) JavaScript and Python runtimes.

go-bash is inspired by
[just-bash](https://github.com/vercel-labs/just-bash) (TypeScript) —
similar bash constructs, builtin set, and VFS shape — but running
in-process under a Go runtime, with `CGO_ENABLED=0` builds and no
`os/exec` reachable from script context.

**Note**: This is beta software. The public API is stable, but
byte-level output of some builtins may shift as we close gaps against
real bash. Pin a version in `go.mod`. See [security
model](#security-model).

## Quick Start

```bash
go get github.com/mark3labs/go-bash@latest
```

```go
package main

import (
    "context"
    "fmt"

    gobash "github.com/mark3labs/go-bash"
)

func main() {
    ctx := context.Background()
    b, _ := gobash.New(gobash.BashOptions{})
    _, _ = b.Exec(ctx, `echo "Hello" > greeting.txt`, gobash.ExecOptions{})
    res, _ := b.Exec(ctx, "cat greeting.txt", gobash.ExecOptions{})
    fmt.Println(res.Stdout)   // "Hello\n"
    fmt.Println(res.ExitCode) // 0
}
```

Each `Exec()` call gets its own isolated shell state — environment
variables, functions, and working directory reset between calls. The
**filesystem is shared** across calls, so files written in one
`Exec()` are visible in the next.

## Custom Commands

Extend go-bash with your own Go commands using `command.Define`:

```go
import (
    "context"
    "fmt"
    "io"
    "strings"

    gobash "github.com/mark3labs/go-bash"
    "github.com/mark3labs/go-bash/command"
)

hello := command.Define("hello", func(ctx context.Context, args []string, c *command.Context) command.Result {
    name := "world"
    if len(args) > 1 {
        name = args[1]
    }
    fmt.Fprintf(c.Stdout, "Hello, %s!\n", name)
    return command.Result{ExitCode: 0}
})

upper := command.Define("upper", func(ctx context.Context, args []string, c *command.Context) command.Result {
    data, _ := io.ReadAll(c.Stdin)
    fmt.Fprint(c.Stdout, strings.ToUpper(string(data)))
    return command.Result{ExitCode: 0}
})

b, _ := gobash.New(gobash.BashOptions{
    CustomCommands: []command.Command{hello, upper},
})

b.Exec(ctx, "hello Alice", gobash.ExecOptions{})    // "Hello, Alice!\n"
b.Exec(ctx, "echo 'test' | upper", gobash.ExecOptions{}) // "TEST\n"
```

Custom commands receive a `command.Context` with `FS`, `Cwd`, `Env`,
`Stdin`, `Stdout`, `Stderr`, `Fetch` (network Doer), and `Exec` (for
sub-shell invocation). They participate in pipes, redirections,
exit-code semantics, and `set -e` propagation like any builtin.

Custom commands **override** builtins with the same name — useful for
swapping in a stricter `curl` or a tracing `cat`.

## Supported Commands

<details>
<summary>Click to expand the full builtin list</summary>

### File Operations

`cat`, `cp`, `file`, `ln`, `ls`, `mkdir`, `mv`, `readlink`, `rm`,
`rmdir`, `split`, `stat`, `touch`, `tree`

### Text Processing

`awk`, `base64`, `column`, `comm`, `cut`, `diff`, `expand`, `fold`,
`grep` (+ `egrep`, `fgrep`), `head`, `join`, `md5sum`, `nl`, `od`,
`paste`, `printf`, `rev`, `rg`, `sed`, `sha1sum`, `sha256sum`, `sort`,
`strings`, `tac`, `tail`, `tr`, `unexpand`, `uniq`, `wc`, `xargs`

### Data Processing

`jq` (JSON), `sqlite3` (SQLite — opt-in via `sqlite` subpackage for
the real runtime), `xan` (CSV), `yq` (YAML/XML/TOML/CSV)

### Optional Runtimes

- `sqlite3` — pure-Go `modernc.org/sqlite`, opt-in via
  `sqlite.Register`
- `js-exec` (JavaScript) — **planned**
- `python3` / `python` — **planned**

### Compression & Archives

`gzip` (+ `gunzip`, `zcat`), `tar`

### Navigation & Environment

`basename`, `cd`, `dirname`, `du`, `echo`, `env`, `export`, `find`,
`hostname`, `printenv`, `pwd`, `tee`

### Shell Utilities

`alias`, `bash`, `chmod`, `clear`, `date`, `expr`, `false`, `help`,
`history`, `seq`, `sh`, `sleep`, `time`, `timeout`, `true`, `unalias`,
`which`, `whoami`

### Network

`curl`, `html-to-markdown` (require [network configuration](#network-access))

All commands support `--help` for usage information.

### Shell Features

- **Pipes**: `cmd1 | cmd2`
- **Redirections**: `>`, `>>`, `2>`, `2>&1`, `<`, heredocs
- **Command chaining**: `&&`, `||`, `;`
- **Variables**: `$VAR`, `${VAR}`, `${VAR:-default}`, arrays
- **Positional parameters**: `$1`, `$2`, `$@`, `$#`
- **Glob patterns**: `*`, `?`, `[...]`, `**` (with `globstar`)
- **Brace expansion**: `{1..10}`, `{a,b,c}`
- **Command substitution**: `$(cmd)`, `` `cmd` ``
- **Process substitution**: `<(cmd)`, `>(cmd)`
- **If statements**: `if COND; then CMD; elif COND; then CMD; else CMD; fi`
- **Functions**: `function name { ... }` or `name() { ... }`
- **Local variables**: `local VAR=value`
- **Loops**: `for`, `while`, `until`
- **Symbolic links**: `ln -s target link`
- **Hard links**: `ln target link`
- **Source / `.`**: `source script.sh`, `. script.sh`
- **Eval**: `eval "..."`
- **Aliases**: `alias name='cmd'`, controlled by `shopt expand_aliases`
- **Shell options**: `shopt`, `set -e`, `set -u`, `set -o pipefail`

</details>

## Configuration

```go
b, _ := gobash.New(gobash.BashOptions{
    // Initial files seeded into the in-memory FS.
    Files: map[string]gobash.FileInit{
        "/data/file.txt": {Content: []byte("content")},
    },
    // Initial environment.
    Env: map[string]string{"MY_VAR": "value"},
    // Starting directory (default: /home/user).
    Cwd: "/app",
    // Execution limits — see "Execution Protection" below.
    ExecutionLimits: &gobash.ExecutionLimits{
        MaxCallDepth: intPtr(50),
    },
})

// Per-Exec overrides.
b.Exec(ctx, "echo $TEMP", gobash.ExecOptions{
    Env: map[string]string{"TEMP": "value"},
    Cwd: "/tmp",
})

// Pass stdin to the script.
b.Exec(ctx, "cat", gobash.ExecOptions{
    Stdin: strings.NewReader("hello from stdin\n"),
})

// Start with a clean environment.
b.Exec(ctx, "env", gobash.ExecOptions{
    ReplaceEnv: true,
    Env:        map[string]string{"ONLY": "this"},
})

// Pass arguments without shell escaping (like spawnSync).
b.Exec(ctx, "grep", gobash.ExecOptions{
    Args: []string{"-r", "TODO", "src/"},
})

// Cancel long-running scripts via context.
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel()
b.Exec(ctx, "while true; do sleep 1; done", gobash.ExecOptions{})
```

### Timezone

`date` defaults to UTC (`%Z=UTC`, `%z=+0000`) regardless of the host
clock, so the sandbox does not leak the host timezone. To opt into a
specific zone, pass `TZ` as an initial env var:

```go
b, _ := gobash.New(gobash.BashOptions{
    Env: map[string]string{"TZ": "America/New_York"},
})
b.Exec(ctx, "date", gobash.ExecOptions{})
// Mon Jun  1 09:30:00 EDT 2026
```

`-u` always forces UTC; an unset or invalid `$TZ` falls back to UTC.
Setting `TZ` exposes that timezone to scripts running in the sandbox,
so only pass a value you are comfortable revealing — forwarding the
host's real `$TZ` (e.g. `os.Getenv("TZ")`) reintroduces the
disclosure that the UTC default exists to prevent.

### `Exec` Options

| Option | Type | Description |
|---|---|---|
| `Env` | `map[string]string` | Environment variables for this execution only |
| `Cwd` | `string` | Working directory for this execution only |
| `Stdin` | `io.Reader` | Standard input passed to the script |
| `Stdout` | `io.Writer` | Where stdout goes; nil → captured into `ExecResult.Stdout` |
| `Stderr` | `io.Writer` | Where stderr goes; nil → captured into `ExecResult.Stderr` |
| `Args` | `[]string` | Additional argv passed directly to the first command (bypasses shell parsing; does not change `$1`, `$2`, ...) |
| `ReplaceEnv` | `bool` | Start with empty env instead of merging (default: `false`) |
| `RawScript` | `bool` | Skip leading-whitespace normalization (default: `false`) |

Cancellation is via `context.Context` — the first argument to `Exec`.

## Filesystem Options

Four filesystem implementations:

**`memfs`** (default) — pure in-memory filesystem, no disk access:

```go
import gobash "github.com/mark3labs/go-bash"

b, _ := gobash.New(gobash.BashOptions{
    Files: map[string]gobash.FileInit{
        "/data/config.json": {Content: []byte(`{"key": "value"}`)},
        // Lazy: called on first read, cached. Never called if written before read.
        "/data/large.csv": {Lazy: func(ctx context.Context) ([]byte, error) {
            return []byte("col1,col2\na,b\n"), nil
        }},
        // Remote: fetch on first read.
        "/data/remote.txt": {Lazy: func(ctx context.Context) ([]byte, error) {
            resp, err := http.Get("https://example.com")
            if err != nil { return nil, err }
            defer resp.Body.Close()
            return io.ReadAll(resp.Body)
        }},
    },
})
```

**`overlayfs`** — copy-on-write over a real directory. Reads come from
disk, writes stay in memory:

```go
import (
    gobash "github.com/mark3labs/go-bash"
    "github.com/mark3labs/go-bash/fs/overlayfs"
)

overlay, _ := overlayfs.New(overlayfs.Options{Root: "/path/to/project"})
b, _ := gobash.New(gobash.BashOptions{FS: overlay, Cwd: "/path/to/project"})

b.Exec(ctx, "cat package.json", gobash.ExecOptions{})      // reads from disk
b.Exec(ctx, `echo "modified" > package.json`, gobash.ExecOptions{}) // in memory
```

**`rwfs`** — direct read-write access to a real directory. Use this
if you want the agent to be able to write to your disk:

```go
import "github.com/mark3labs/go-bash/fs/rwfs"

rw, _ := rwfs.New(rwfs.Options{Root: "/path/to/sandbox"})
b, _ := gobash.New(gobash.BashOptions{FS: rw})

b.Exec(ctx, `echo "hello" > file.txt`, gobash.ExecOptions{}) // writes to real disk
```

Keep `rwfs` pointed at a workspace directory, not at the installed
go-bash module or any other trusted runtime code. Guest-writable
roots should stay separate from trusted code.

**`mountfs`** — mount multiple filesystems at different paths.
Combines read-only and read-write filesystems into a unified
namespace:

```go
import (
    "github.com/mark3labs/go-bash/fs/memfs"
    "github.com/mark3labs/go-bash/fs/mountfs"
    "github.com/mark3labs/go-bash/fs/overlayfs"
    "github.com/mark3labs/go-bash/fs/rwfs"
)

// Set up the base + mounts.
ro, _ := overlayfs.New(overlayfs.Options{Root: "/path/to/knowledge", ReadOnly: true})
rw, _ := rwfs.New(rwfs.Options{Root: "/path/to/workspace"})
m, _ := mountfs.New(mountfs.Options{
    Base: memfs.New(),
    Mounts: []mountfs.Mount{
        {Path: "/mnt/knowledge", FileSystem: ro},
        {Path: "/home/agent", FileSystem: rw},
    },
})

b, _ := gobash.New(gobash.BashOptions{FS: m, Cwd: "/home/agent"})

b.Exec(ctx, "ls /mnt/knowledge", gobash.ExecOptions{})       // from knowledge base
b.Exec(ctx, "cp /mnt/knowledge/doc.txt ./", gobash.ExecOptions{}) // cross-mount copy
b.Exec(ctx, `echo "notes" > notes.txt`, gobash.ExecOptions{}) // writes to workspace
```

## Optional Capabilities

### Network Access

Network access is disabled by default. Enable it with the `Network`
option:

```go
import "github.com/mark3labs/go-bash/network"

// Allow specific URLs with GET/HEAD only (safest).
b, _ := gobash.New(gobash.BashOptions{
    Network: &network.Config{
        AllowedURLPrefixes: []network.AllowedURLEntry{
            {URL: "https://api.github.com/repos/myorg/"},
            {URL: "https://api.example.com"},
        },
    },
})

// Allow specific URLs with additional methods.
b, _ = gobash.New(gobash.BashOptions{
    Network: &network.Config{
        AllowedURLPrefixes: []network.AllowedURLEntry{
            {URL: "https://api.example.com"},
        },
        AllowedMethods: []string{"GET", "HEAD", "POST"}, // default: ["GET", "HEAD"]
    },
})

// Inject credentials via header transforms (secrets never enter the sandbox).
b, _ = gobash.New(gobash.BashOptions{
    Network: &network.Config{
        AllowedURLPrefixes: []network.AllowedURLEntry{
            {URL: "https://public-api.com"}, // no transforms
            {
                URL: "https://ai-gateway.vercel.sh",
                Transform: []network.RequestTransform{{
                    Headers: map[string]string{
                        "Authorization": "Bearer " + os.Getenv("API_TOKEN"),
                    },
                }},
            },
        },
    },
})

// Allow all URLs and methods (use with caution).
b, _ = gobash.New(gobash.BashOptions{
    Network: &network.Config{DangerouslyAllowFullAccess: true},
})
```

**Note:** The `curl` command exists in the registry whether or not
network is configured. With no network config, `curl` returns
"network disabled" and exits non-zero.

#### Allow-List Security

The allow-list enforces:

- **Origin matching**: URLs must match the exact origin (scheme + host + port)
- **Path prefix**: Only paths starting with the specified prefix are
  allowed (case-sensitive)
- **HTTP method restrictions**: Only `GET` and `HEAD` by default
  (configure `AllowedMethods` for more)
- **Redirect protection**: Redirects to non-allowed URLs are blocked;
  the allow-list is re-checked at every hop
- **Header transforms**: Headers in `Transform` are injected at the
  fetch boundary and **override** any user-supplied headers with the
  same name, preventing credential substitution from inside the
  sandbox. Headers are re-evaluated on each redirect so credentials
  are never leaked to non-transform hosts.
- **Private-range deny**: When `DenyPrivateRanges: true`, the resolver
  rejects RFC-1918 / link-local / loopback addresses before dial,
  blocking SSRF to cloud metadata endpoints.

#### Using curl

```bash
# Fetch and process data
curl -s https://api.example.com/data | grep pattern

# Download and convert HTML to Markdown
curl -s https://example.com | html-to-markdown

# POST JSON data
curl -X POST -H "Content-Type: application/json" \
  -d '{"key":"value"}' https://api.example.com/endpoint
```

### SQLite Support

The `sqlite3` builtin ships as a stub in the default registry. For the
real runtime (pure-Go `modernc.org/sqlite`), import the `sqlite`
subpackage and call `Register`:

```go
import gbsqlite "github.com/mark3labs/go-bash/sqlite"

b, _ := gobash.New(gobash.BashOptions{})
_ = gbsqlite.Register(b, gbsqlite.Options{Timeout: 5 * time.Second})

// Query in-memory database
b.Exec(ctx, `sqlite3 :memory: "SELECT 1 + 1"`, gobash.ExecOptions{})

// Query file-based database
b.Exec(ctx, `sqlite3 data.db "SELECT * FROM users"`, gobash.ExecOptions{})
```

File DBs shuttle through `os.MkdirTemp` for the query duration and
write back to the VFS via `c.FS.WriteFile` on cleanup. Concurrent
writers race the cleanup; last writer wins.

Queries run with a configurable timeout (default 5 s) enforced via
`context.WithTimeout` + a goroutine that closes the DB on
`ctx.Done()`.

### JavaScript Support

**Planned.** When implemented, the
`jsexec/` subpackage will provide `js-exec` backed by
`github.com/dop251/goja` (pure Go, no cgo). The TS port uses QuickJS;
the Go port will use goja for the same `CGO_ENABLED=0` guarantee.

### Python Support

**Planned.** Unlike just-bash (which
embeds CPython compiled to WASM), go-bash will NOT embed CPython.
The `pythonexec/` subpackage will expose a `Runtime` interface that
the host implements — typically by routing to `docker run python:3.13`
or a real `python3` binary the host opts into. This is an
intentional divergence; see `DECISIONS.md` for the rationale.

## AST Transform Plugins

Parse bash scripts into an AST, transform them, and serialize back to
bash. Good for instrumenting scripts (e.g., capturing per-command
stdout/stderr) or extracting metadata before execution.

```go
import (
    gobash "github.com/mark3labs/go-bash"
    "github.com/mark3labs/go-bash/transform"
    "github.com/mark3labs/go-bash/transform/plugins/collector"
    "github.com/mark3labs/go-bash/transform/plugins/tee"
)

// Standalone pipeline — output can be run by any shell.
pipeline := transform.New()
pipeline.Use(tee.New(tee.Options{OutputDir: "/tmp/logs"}))
pipeline.Use(collector.New())
result, _ := pipeline.Transform("echo hello | grep hello")
result.Script                                // transformed bash string
result.Metadata["command-collector"]        // {"commands": ["echo", "grep", "tee"]}

// Integrated API — Exec() auto-applies transforms and returns metadata.
b, _ := gobash.New(gobash.BashOptions{})
b.RegisterTransformPlugin(collector.New())
res, _ := b.Exec(ctx, "echo hello | grep hello", gobash.ExecOptions{})
res.Metadata["command-collector"]            // {"commands": ["echo", "grep"]}
```

See the package docs for the full API, built-in plugins, and how to
write custom plugins.

## Sandbox API

The `sandbox/` subpackage is a drop-in replacement for
[`@vercel/sandbox`](https://vercel.com/docs/vercel-sandbox) — same
API surface, but runs entirely in-process with the virtual filesystem.
Start with go-bash for development and testing, swap in a real
sandbox when you need a full VM.

```go
import "github.com/mark3labs/go-bash/sandbox"

sb, _ := sandbox.Create(ctx, sandbox.Options{Cwd: "/app"})

// Write files to the virtual filesystem.
sb.WriteFiles(ctx, map[string]string{
    "/app/script.sh": `echo "Hello World"`,
    "/app/data.json": `{"key": "value"}`,
})

// Run commands and get results.
cmd, _ := sb.RunCommand(ctx, sandbox.RunCommandParams{
    Cmd:  "bash",
    Args: []string{"/app/script.sh"},
})
stdout, _ := cmd.Stdout()    // "Hello World\n"
fin, _ := cmd.Wait()
fmt.Println(fin.ExitCode)    // 0

// Read files back.
content, _ := sb.ReadFile(ctx, "/app/data.json")

// Create directories.
sb.MkDir(ctx, "/app/logs", sandbox.MkDirOptions{Recursive: true})

// Clean up (no-op for Bash, but API-compatible).
sb.Stop(ctx)
```

## CLI

### CLI Binary

Install globally for a sandboxed CLI:

```bash
go install github.com/mark3labs/go-bash/cmd/gobash@latest

# Execute inline script.
gobash -c 'ls -la && cat package.json | head -5'

# Execute with specific project root.
gobash -c 'grep -r "TODO" src/' --root /path/to/project

# Pipe script from stdin.
echo 'find . -name "*.go" | wc -l' | gobash

# Execute a script file.
gobash ./scripts/deploy.sh

# Get JSON output for programmatic use.
gobash -c 'echo hello' --json
# {"stdout":"hello\n","stderr":"","exitCode":0}
```

The CLI uses overlayfs — reads come from the real filesystem, but all
writes stay in memory and are discarded after execution (unless
`--allow-write` is supplied).

**Important**: The project root is mounted at `/home/user/project`.
Use this path (or relative paths from the default cwd) to access your
files inside the sandbox.

Options:

| Flag | Description |
|---|---|
| `-c <script>` | Execute script from argument |
| `--root <path>` | Root directory (default: current directory) |
| `--cwd <path>` | Working directory in sandbox (default: project mount point) |
| `--allow-write` | Allow write operations (default: read-only) |
| `--python` | Accept the flag for `just-bash` compat (no-op until Phase 16) |
| `--javascript` | Accept the flag for `just-bash` compat (no-op until Phase 15) |
| `-e`, `--errexit` | Exit on first error |
| `--json` | Output as JSON |
| `--no-network` | Explicitly disable network (default) |
| `--network-allow PREFIX` | Allow network requests to PREFIX (repeatable; go-bash-specific) |
| `-h`, `--help` | Show help |
| `-v`, `--version` | Show version |

The flags are a **strict superset** of just-bash's CLI — every
invocation that works against `just-bash` works against `gobash`,
with go-bash-specific additions (`--no-network`, `--network-allow`)
documented above.

## Execution Protection

go-bash protects against infinite loops, deep recursion, and runaway
output with configurable limits:

```go
b, _ := gobash.New(gobash.BashOptions{
    ExecutionLimits: &gobash.ExecutionLimits{
        MaxCallDepth:      intPtr(100),
        MaxCommandCount:   intPtr(10000),
        MaxLoopIterations: intPtr(10000),
        MaxAwkIterations:  intPtr(10000),
        MaxSedIterations:  intPtr(10000),
    },
})
```

All limits have defaults. Error messages tell you which limit was hit
via `*gobash.ExecutionLimitError` — use `errors.As` to inspect.

| Field | Default |
|---|---|
| `MaxCallDepth` | 100 |
| `MaxCommandCount` | 10 000 |
| `MaxLoopIterations` | 10 000 |
| `MaxAwkIterations` | 10 000 |
| `MaxSedIterations` | 10 000 |
| `MaxJqIterations` | 10 000 |
| `MaxSqliteTimeout` | 5 s |
| `MaxPythonTimeout` | 10 s |
| `MaxJsTimeout` | 10 s |
| `MaxGlobOperations` | 100 000 |
| `MaxStringLength` | 10 MiB |
| `MaxArrayElements` | 100 000 |
| `MaxHeredocSize` | 10 MiB |
| `MaxSubstitutionDepth` | 50 |
| `MaxBraceExpansionResults` | 10 000 |
| `MaxOutputSize` | 10 MiB |
| `MaxFileDescriptors` | 1024 |
| `MaxSourceDepth` | 100 |

## Security Model

- The shell only has access to the provided filesystem.
- All execution happens **in-process**, without VM isolation. The
  code base is designed to be robust against the structural attacks
  that JavaScript-based sandboxes have to defend against (prototype
  pollution, `eval`, dynamic `import()`) — those vectors do not
  exist in Go.
- There is **no network access by default**. When enabled, requests
  are checked against URL prefix allow-lists and HTTP-method
  allow-lists at every hop.
- Python and JavaScript execution are off by default — those are
  opt-in subpackages that represent additional security surface.
- Execution is protected against infinite loops, deep recursion,
  and runaway output with configurable limits.
- `os/exec` is **not imported** in the runtime package — no code path
  exists from the script to a host process.
- Use a real container ([Vercel Sandbox](https://vercel.com/docs/vercel-sandbox),
  Firecracker, etc.) if you need a full VM with arbitrary binary
  execution.

See [THREAT_MODEL.md](THREAT_MODEL.md) for the full threat model,
including a "What Go gives us for free" section explaining the
structural advantages over the TypeScript reference implementation.

## Default Layout

When created without options, `Bash` provides a Unix-like directory
structure:

- `/home/user` — default working directory (and `$HOME`)
- `/bin` — contains stubs for all built-in commands
- `/usr/bin` — additional binary directory
- `/tmp` — temporary files directory
- `/etc/hostname` — synthesized hostname
- `/proc/self/status` — synthesized procfs entry with virtualized
  PID / UID

Commands can be invoked by path (e.g., `/bin/ls`) or by name.

## AI Agent Instructions

For AI agents wiring go-bash into a tool-use loop, see
[`AGENTS.md`](AGENTS.md) — it covers the standard agent-tool setup,
filesystem choices, network policy, custom commands, and common
failure modes you'll want to surface back to the model.

## Status

go-bash is feature-complete for its core runtime, builtins, sandbox
API, and CLI:

| Area | Status |
|---|---|
| Runtime + builtins | Complete |
| SQLite runtime | Complete |
| Sandbox API | Complete |
| CLI | Complete |
| JavaScript runtime (via goja) | Deferred |
| Python runtime (via host hook) | Pending |

## Contributing

If you're working on go-bash internals (not using it as a library),
start with `AGENTS.md` and the package-level docs.

## License

MIT. See [LICENSE](LICENSE).
