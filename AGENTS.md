# AGENTS.md — Using go-bash from an AI agent

This file is the agent-facing operating guide for **go-bash**. If you are
an AI agent (Claude, GPT, Gemini, etc.) wiring go-bash into a tool-call
loop, read this in full before issuing any `Exec` calls. If you are a
human contributor working on go-bash internals, read the package-level
docs and `DECISIONS.md` instead.

## What go-bash is

go-bash is an **in-process, sandboxed bash environment** written in Go.
Scripts run inside a virtual filesystem with no host disk access by
default and no network access by default. The runtime is byte-compatible
with `just-bash` (the TypeScript original) for the same set of bash
constructs and built-in commands.

Concretely, you call `bash.Exec(ctx, "echo hi", ExecOptions{})` from
Go. The script never reaches the host shell, never reaches `os/exec`,
and (by default) never reaches the host filesystem. You get back
stdout, stderr, and an exit code as strings.

```go
import gobash "github.com/mark3labs/go-bash"

b, _ := gobash.New(gobash.BashOptions{})
res, _ := b.Exec(ctx, "echo hello", gobash.ExecOptions{})
// res.Stdout == "hello\n", res.ExitCode == 0
```

## When to use go-bash vs. shelling out

Use **go-bash** when you want:

- A bash environment an LLM can write scripts against without you
  trusting the host shell.
- Per-call file isolation — writes vanish when the call returns
  (with `memfs`) or stay scoped to an overlay (with `overlayfs`).
- Deterministic limits — every limit in `ExecutionLimits` is enforced
  before runaway scripts can DoS your process.
- A test harness for bash-using prompts without needing a container.

Use **`os/exec` directly** when you want:
- To launch a real long-running binary (`ffmpeg`, `git`, etc.).
- To talk to the host kernel (mounting, raw sockets, ptrace).
- The full real-bash surface (job control, signals, `disown`, etc.).

go-bash deliberately does NOT cover those cases.

## The standard agent-tool setup

A typical AI-agent integration looks like this:

```go
import (
    "context"
    gobash "github.com/mark3labs/go-bash"
    "github.com/mark3labs/go-bash/network"
)

func newAgentBash() (*gobash.Bash, error) {
    return gobash.New(gobash.BashOptions{
        // Seed initial files the model can read.
        Files: map[string]gobash.FileInit{
            "/work/README.md": {Content: []byte("...")},
        },
        // Start in /work so relative paths feel natural.
        Cwd: "/work",
        // Allow network only to your AI gateway or known APIs.
        Network: &network.Config{
            AllowedURLPrefixes: []network.AllowedURLEntry{
                {URL: "https://api.example.com/v1"},
            },
        },
    })
}

func toolBashExec(ctx context.Context, b *gobash.Bash, script string) string {
    res, err := b.Exec(ctx, script, gobash.ExecOptions{})
    if err != nil {
        return "ERROR: " + err.Error()
    }
    return res.Stdout + res.Stderr
}
```

Wire `toolBashExec` into your tool registry (Anthropic tool-use, OpenAI
function-calling, MCP, the AI SDK). The model sees a single `bash` tool;
you control what it can touch.

## What the model can do by default

When called with `BashOptions{}`:

- **Filesystem.** A fresh in-memory FS (`memfs`). `/home/user` is the
  default cwd. `/bin/<cmd>` stubs exist for every registered builtin.
  `/etc/hostname`, `/proc/self/status` are populated.
- **Builtins.** Every Wave A–H builtin (~140 commands): `cat`, `ls`,
  `grep`, `sed`, `awk`, `jq`, `cut`, `sort`, `uniq`, `tar`, `gzip`,
  `base64`, `find`, `xargs`, `bash`, `sh`, `eval`, `source`, `.`,
  `timeout`, `printf`, `date` (UTC by default), and many more.
- **Shell features.** Pipes, redirections (`>`, `>>`, `<`, `2>&1`,
  heredocs), `&&` / `||` / `;`, variables (`$VAR`, `${VAR:-default}`,
  arrays), functions, `for` / `while` / `until` loops, `if` / `case`,
  glob expansion, brace expansion (`{1..10}`), command substitution
  (`$(...)`), parameter expansion, process substitution (`<(...)`).
- **Limits.** Loop iterations, call depth, command count, output size,
  string length, brace expansion size — all enforced by default.

What the model **cannot** do by default:
- **No network.** `curl https://example.com` returns "network
  disabled" until you supply `BashOptions.Network`.
- **No host disk.** `cat /etc/passwd` returns `No such file or
  directory` — the VFS contains only what you seeded.
- **No `os/exec`.** There is no path from the script to a host
  binary. Unknown commands return exit 127 (`command not found`).
- **No Python / JavaScript runtimes.** Those are opt-in subpackages
  (see "Optional capabilities" below) and not built into the default
  binary.

## Filesystem choices

Four FS implementations ship; pick one per agent session.

| FS | Reads from | Writes go to | When to use |
|---|---|---|---|
| `memfs` (default) | in-memory | in-memory | Pure isolation — agent can't touch the host. |
| `overlayfs` | host dir + overlay | in-memory overlay | Read agent's project read-only; writes vanish. |
| `rwfs` | host dir | host dir | Let agent modify a scratch dir on disk. |
| `mountfs` | composable | composable | Mix-and-match (e.g. `/data` read-only + `/work` writable). |

Examples:

```go
// Read project read-only, capture writes in memory.
overlay, _ := overlayfs.New(overlayfs.Options{Root: "/path/to/project"})
b, _ := gobash.New(gobash.BashOptions{FS: overlay, Cwd: "/path/to/project"})

// Let the agent write to a scratch dir on disk.
rw, _ := rwfs.New(rwfs.Options{Root: "/tmp/agent-scratch"})
b, _ := gobash.New(gobash.BashOptions{FS: rw})

// Read-only knowledge base + writable workspace.
m, _ := mountfs.New(mountfs.Options{Base: memfs.New()})
ro, _ := overlayfs.New(overlayfs.Options{Root: "/kb", ReadOnly: true})
rw, _ := rwfs.New(rwfs.Options{Root: "/tmp/workspace"})
m.Mount("/mnt/kb", ro)
m.Mount("/home/agent", rw)
b, _ := gobash.New(gobash.BashOptions{FS: m, Cwd: "/home/agent"})
```

**`rwfs` is the only mode that touches the host disk.** Point it at a
scratch directory, never at the project root or the directory
containing your agent binary.

## Network policy

Network is **off by default**. Enable per-host:

```go
import "github.com/mark3labs/go-bash/network"

b, _ := gobash.New(gobash.BashOptions{
    Network: &network.Config{
        AllowedURLPrefixes: []network.AllowedURLEntry{
            // Public read-only API — no auth.
            {URL: "https://api.github.com/repos/myorg/"},

            // Authed API — inject the bearer at the fetch boundary
            // so the token never enters the sandbox script context.
            {
                URL: "https://api.openai.com/v1",
                Transform: []network.RequestTransform{{
                    Headers: map[string]string{
                        "Authorization": "Bearer " + os.Getenv("OPENAI_API_KEY"),
                    },
                }},
            },
        },
        AllowedMethods:  []string{"GET", "POST"},
        Timeout:         30 * time.Second,
        MaxResponseSize: 10 * 1024 * 1024,
    },
})
```

The allow-list enforces:
- Exact origin match (`scheme://host:port`)
- Path-prefix match (case-sensitive)
- Method allow-list (default `GET`, `HEAD` only)
- Per-redirect re-validation (no SSRF via 302)
- Headers from `Transform` override any header the script set with
  the same name — credentials are unreachable from inside the script.

For raw escape, `DangerouslyAllowFullAccess: true` opens everything.
**Never** combine that with a script source you don't trust.

## Execution limits

Every limit in `ExecutionLimits` has a default. Tighten them when an
agent's tool loop is short; loosen them only with reason.

```go
b, _ := gobash.New(gobash.BashOptions{
    ExecutionLimits: &gobash.ExecutionLimits{
        MaxLoopIterations: intPtr(1000),
        MaxOutputSize:     intPtr(64 * 1024),
    },
})
```

Defaults (full table in `README.md`):

| Limit | Default |
|---|---|
| MaxCallDepth | 100 |
| MaxCommandCount | 10000 |
| MaxLoopIterations | 10000 |
| MaxOutputSize | 10 MiB |
| MaxStringLength | 10 MiB |
| MaxHeredocSize | 10 MiB |
| MaxJqIterations | 10000 |
| MaxAwkIterations | 10000 |

When a limit trips, `Exec` returns an `*ExecutionLimitError` you can
test with `errors.As`. Surface the limit name back to the model so it
can retry with a smaller script.

## Optional capabilities

| Capability | Package | Built into default binary? |
|---|---|---|
| `sqlite3` (pure-Go modernc.org/sqlite) | `github.com/mark3labs/go-bash/sqlite` | Yes (Wave H stub); call `sqlite.Register` for the real runtime |
| JavaScript (`js-exec` via goja) | NOT YET BUILT | No — Phase 15 deferred |
| Python (`python3` via host hook) | NOT YET BUILT | No — Phase 16 deferred |

To enable the SQLite real runtime:

```go
import gbsqlite "github.com/mark3labs/go-bash/sqlite"

b, _ := gobash.New(gobash.BashOptions{})
_ = gbsqlite.Register(b, gbsqlite.Options{Timeout: 5 * time.Second})

b.Exec(ctx, `sqlite3 :memory: "SELECT 1 + 1"`, gobash.ExecOptions{})
```

## Per-call options

`ExecOptions` carries per-call overrides:

```go
res, _ := b.Exec(ctx, "echo $TEMP", gobash.ExecOptions{
    Env:        map[string]string{"TEMP": "value"},     // merged on top
    Cwd:        "/tmp",
    Stdin:      strings.NewReader("piped input"),
    ReplaceEnv: false,                                   // true = wipe parent env
    Args:       []string{"extra", "argv"},               // passed to first cmd
})
```

`Env` merges on top of the per-Bash env by default; `ReplaceEnv: true`
wipes it first. Env mutations from `export` survive across calls
unless you set `ReplaceEnv`.

## Cancellation

The first parameter is always `context.Context`. Cancel the context
to abort a runaway script:

```go
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel()
res, err := b.Exec(ctx, untrustedScript, gobash.ExecOptions{})
// On timeout: err is context.DeadlineExceeded.
```

Cancellation is **cooperative** — the runtime checks ctx at every
statement boundary. A pathological tight loop in a single builtin
(e.g. `awk`) may take a full iteration to notice; the iteration
limits cover that case.

## Custom commands

Register Go commands the script can call by name:

```go
import "github.com/mark3labs/go-bash/command"

hello := command.Define("hello", func(ctx context.Context, args []string, c *command.Context) command.Result {
    name := "world"
    if len(args) > 1 {
        name = args[1]
    }
    fmt.Fprintf(c.Stdout, "Hello, %s!\n", name)
    return command.Result{ExitCode: 0}
})

b, _ := gobash.New(gobash.BashOptions{
    CustomCommands: []command.Command{hello},
})

b.Exec(ctx, "hello Alice", gobash.ExecOptions{})
// stdout: "Hello, Alice!\n"
```

Custom commands receive a `command.Context` with `FS`, `Cwd`, `Env`,
`Stdin`, `Stdout`, `Stderr`, `Fetch` (the network Doer), and `Exec`
(for sub-shell invocation). They participate in pipes, redirections,
and exit-code semantics like any builtin.

Custom commands **override** builtins with the same name — useful for
swapping in a stricter `curl` or a tracing `cat`.

## Sandbox helper

If your existing code targets [`@vercel/sandbox`](https://vercel.com/docs/vercel-sandbox),
the `sandbox` subpackage wraps `*Bash` in the same surface:

```go
import "github.com/mark3labs/go-bash/sandbox"

sb, _ := sandbox.Create(ctx, sandbox.Options{
    OverlayRoot: "/path/to/project",
})
cmd, _ := sb.RunCommand(ctx, sandbox.RunCommandParams{
    Cmd:  "ls",
    Args: []string{"-la"},
})
stdout, _ := cmd.Stdout()
fin, _ := cmd.Wait()
// fin.ExitCode is the script's exit code.
```

Same security model as the underlying `*Bash`. No real container, no
process isolation — just the VFS + the network allow-list.

## CLI

The `gobash` binary is a thin shell around the library. Useful for
shell scripts and as an executable tool you can hand to other agents:

```bash
go install github.com/mark3labs/go-bash/cmd/gobash@latest

gobash -c 'echo hello'
gobash --root /path/to/project -c 'ls -la'
gobash --json -c 'echo a; echo b 1>&2; exit 3'
# {"stdout":"a\n","stderr":"b\n","exitCode":3}
```

The CLI flags are a strict superset of `just-bash`'s — invocations
that work against `just-bash` also work here. See `README.md` for
the full flag list.

## What to tell the model

When wiring go-bash into a tool-use loop, the most useful
single-paragraph description for the system prompt is:

> You have access to a `bash` tool that runs scripts in a sandboxed
> in-memory Linux environment. The filesystem starts empty (or
> contains the files I provided); writes do not persist outside the
> session unless I configured a writable mount. There is no network
> unless explicitly allowed. You can use every standard Unix command
> you know: `cat`, `grep`, `sed`, `awk`, `jq`, `find`, `tar`, etc.
> Background jobs (`&`) and `wait` work synchronously. The
> environment is byte-compatible with bash for the commands and
> syntax it supports.

## Common failure modes & how to surface them

| Symptom | Likely cause | Action |
|---|---|---|
| `exit code 127, "X: command not found"` | Builtin not registered, or `BashOptions.Commands` filtered it out | Add it to `Commands` or implement it as a `CustomCommand`. |
| `error: ExecutionLimitError{Limit: "MaxOutputSize"}` | Script produced > 10 MiB of output | Raise `MaxOutputSize` or pipe through `head`. |
| `exit code 0, empty stdout` after a network call | Script wrote to a file you can't see | Use `memfs.AllPaths()` to inspect; or have script `cat` the file. |
| `"network disabled"` | `BashOptions.Network` not set | Provide a `Network` config with the URL in the allow-list. |
| `context.DeadlineExceeded` | Script ran past your ctx timeout | Either raise the timeout or instruct the model to use `timeout` builtin. |
| Mysterious `*` glob behavior | `MaxGlobOperations` (default 100k) tripped silently | Tighten the glob pattern; or raise the limit. |

When surfacing failures to the model, **include the limit name and
the trigger condition** — models recover well when told exactly
which knob they hit.

## Threat model (TL;DR)

Read `THREAT_MODEL.md` for the full version. Short summary:

- **Untrusted scripts cannot:** read the host disk, make network
  calls outside the allow-list, spawn host processes, read your
  environment variables, escape via prototype pollution, escape via
  `eval` (no `eval` reaches Go code — `eval` in bash stays inside
  the bash interpreter).
- **Untrusted scripts CAN:** consume CPU and memory up to the
  configured limits, write arbitrary content into the VFS (which
  vanishes with the `*Bash`), read whatever you seeded into the VFS
  with `Files` or `FS`.
- **You ARE responsible for:** keeping `rwfs` pointed at a scratch
  directory, not feeding the model your `os.Getenv` directly, and
  tightening limits below the defaults for short-lived agent loops.

The host hooks you pass in (`CustomCommands`, `Fetch`, `Logger`,
`InvokeTool`) run with full Go privileges — they are trusted code by
definition. Don't load them from untrusted sources at runtime.

## License & status

go-bash is MIT licensed.

go-bash is **beta software**. The public surface is stable, but the
byte-level output of some builtins may shift as we close gaps against
real bash. Pin a version in `go.mod`.
