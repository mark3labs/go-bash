# go-bash — Feature-for-Feature Go Port of `just-bash`

> **Source of truth**: `github.com/vercel-labs/just-bash`
> **Target module path**: `github.com/mark3labs/go-bash` (rename to your org before starting)
> **Language**: Go 1.22+
> **Cgo policy**: Default zero-cgo. Optional subpackages may use cgo; main package must not.

This spec is **prescriptive and step-ordered**. Implement phases in order; do not skip ahead. Each phase ends with concrete acceptance criteria. The goal is to reach byte-identical behavior with real bash for the same set of constructs `just-bash` supports.

---

## 0. Ground Rules

### 0.1 Idioms (non-negotiable)
- **Cancellation** uses `context.Context`. Every public function and every command implementation takes a `ctx` as its first parameter. No global state for timeouts.
- **Streams** use `io.Reader` / `io.Writer`. Stdin/stdout/stderr are streams, not strings. Conversion to `string` happens only at the public `Exec(...)` boundary when no writer is provided.
- **Errors** are returned. A non-zero `ExitCode` is *not* an error — only harness failures (parse error, limit hit, cancellation, I/O failure outside the script's control) return a non-nil `error`.
- **Maps** are plain `map[string]string`. Go has no prototype pollution; do not port the null-prototype gymnastics.
- **Concurrency**: A single `*Bash` may be used from multiple goroutines but `Exec` calls serialize on an internal mutex. Document this.
- **Exported identifiers** are `PascalCase`. Match TS names where they make sense (`Bash`, `OverlayFs`, `MountableFs`) but use `FS` not `Fs` in new types (Go convention is `FS`; tolerate `Fs` only where it matches the imported TS class names exactly — `OverlayFs`, `InMemoryFs`, `ReadWriteFs`, `MountableFs` — for direct API parity).

### 0.2 Dependencies (vendor decisions are FIXED)
| Need | Library | Reason |
|---|---|---|
| Bash parser + AST + tree-walking interpreter base | `mvdan.cc/sh/v3` (`syntax`, `interp`, `expand`) | Production-grade, real-bash comparison-tested |
| Glob | `mvdan.cc/sh/v3/pattern` + custom globstar | Matches `mvdan/sh` semantics |
| AWK | `github.com/benhoyt/goawk` (package interpreter mode) | Mature, no cgo |
| jq | `github.com/itchyny/gojq` | Pure Go, full jq syntax |
| YAML | `github.com/goccy/go-yaml` | Better round-trip than `gopkg.in/yaml.v3` |
| TOML | `github.com/BurntSushi/toml` | Stdlib-quality |
| XML | `encoding/xml` (std) + custom unmarshal helper | Std is sufficient for `yq` use |
| CSV | `encoding/csv` (std) | Sufficient for `xan` use |
| Hashing | `crypto/md5`, `crypto/sha1`, `crypto/sha256` | Std |
| Gzip / Tar / Bzip2 / Zstd | `compress/gzip`, `archive/tar`, `compress/bzip2`, `github.com/klauspost/compress/zstd` | Pure Go |
| LZMA | `github.com/ulikunitz/xz` | Pure Go |
| SQLite (opt-in) | `modernc.org/sqlite` | Pure Go, no cgo |
| JavaScript (opt-in) | `github.com/dop251/goja` | ES5.1 + most ES6, pure Go, sandboxable |
| Python (opt-in) | **Host-provided hook only** (interface). No embedded runtime. | See §15 |
| HTML→Markdown | `github.com/JohannesKaufmann/html-to-markdown/v2` | Pure Go |
| File type detection | `github.com/gabriel-vasile/mimetype` | Pure Go |
| Minimatch / glob to regex | Write our own `internal/glob` matching `minimatch` semantics | See §6.7 |
| Diff | `github.com/sergi/go-diff/diffmatchpatch` for unified diff | Used by `diff` builtin |
| sprintf | `fmt` (std) — augment with our own `printf` for bash-specific `%b`, `%q`, `%(...)T` | See §10.18 |

If a chosen library is later found inadequate, document the swap in a `DECISIONS.md` file — do not silently substitute.

### 0.3 What we explicitly DO NOT port
- The entire `src/security/` directory (defense-in-depth monkey-patching of JS globals). Go has no equivalent attack surface to mitigate.
- `src/timers.ts` (captures references before defense patches them).
- The browser bundle (`browser.ts`, `shims/`, `browser.bundle.test.ts`).
- `re2js` substitution — Go's `regexp` is already RE2.
- Worker thread infrastructure (`worker-bridge/`, `*.worker.ts`, `Atomics.wait` protocols). Go uses goroutines + channels.
- The `sandbox/Sandbox.security.test.ts` JS-escape regression tests.
- TypeScript-specific encoding gymnastics (`ByteString`, `latin1FromBytes`, the whole `encoding.ts` byte/text discriminator). Go's `[]byte` + `string` distinction does this natively.

### 0.4 Repository layout
```
go-bash/
├── go.mod                          // module github.com/mark3labs/go-bash, go 1.22
├── README.md                       // mirrors just-bash README, Go-flavored
├── SPEC.md                         // this file
├── DECISIONS.md                    // running log of design decisions
├── AGENTS.md                       // ported from just-bash, Go-adapted
├── bash.go                         // Bash type, New, Exec
├── options.go                      // BashOptions, ExecOptions
├── result.go                       // ExecResult, BashExecResult, Logger
├── limits.go                       // ExecutionLimits, defaults, resolveLimits
├── errors.go                       // ExitError, ExecutionLimitError, etc.
├── doc.go                          // package doc
│
├── ast/                            // re-exported AST node types
├── parser/                         // public Parse() wrapping mvdan/sh
├── interp/                         // wraps mvdan/sh interpreter, adds limits/hooks
│
├── fs/
│   ├── fs.go                       // FileSystem interface
│   ├── path.go                     // path utils (clean/join/validate/null-byte reject)
│   ├── memfs/                      // InMemoryFs
│   ├── overlayfs/                  // OverlayFs (COW over real FS)
│   ├── rwfs/                       // ReadWriteFs (writes hit disk)
│   ├── mountfs/                    // MountableFs
│   └── realfs/                     // shared real-FS security helpers (TOCTOU, O_NOFOLLOW)
│
├── command/                        // Command interface, Context, DefineCommand
│   └── builtin/                    // registry + lazy loaders
│
├── builtins/                       // one subpackage per command (see §10)
│   ├── cat/, ls/, cp/, mv/, rm/, mkdir/, rmdir/, ln/, chmod/, touch/, stat/, tree/, du/, split/, file/, readlink/,
│   ├── echo/, printf/, pwd/, basename/, dirname/, env/, printenv/, hostname/, whoami/, which/, help/, history/, alias/, unalias/,
│   ├── head/, tail/, wc/, cut/, paste/, tr/, rev/, nl/, fold/, expand/, unexpand/, strings/, tac/, sort/, uniq/, comm/, join/, tee/, column/, find/, xargs/,
│   ├── grep/, sed/, awk/,
│   ├── base64/, od/, md5sum/, sha1sum/, sha256sum/,
│   ├── jq/, yq/, xan/,
│   ├── gzip/, tar/,
│   ├── diff/, date/, sleep/, timeout/, seq/, expr/, time/, true_/, false_/, clear/,
│   ├── rg/,
│   ├── bash_/, sh_/,
│   └── htmlmd/                     // html-to-markdown
│
├── network/                        // SecureFetch, allow-list, header transforms
│
├── transform/                      // BashTransformPipeline
│   └── plugins/
│       ├── tee/                    // TeePlugin
│       └── collector/              // CommandCollectorPlugin
│
├── sandbox/                        // Vercel-Sandbox-compatible facade
│
├── jsexec/                         // OPTIONAL: js-exec via goja
├── sqlite/                         // OPTIONAL: sqlite3 via modernc.org/sqlite
├── pythonexec/                     // OPTIONAL: hook interface only
│
├── cmd/
│   ├── gobash/                     // CLI: `go-bash -c '...'`
│   └── gobash-shell/               // Interactive shell (REPL)
│
└── internal/
    ├── shellquote/                 // shell escape/join
    ├── glob/                       // minimatch-style globbing
    ├── ifs/                        // IFS word splitting helpers
    ├── pathutil/                   // joins/cleans that disallow null bytes
    ├── ringbuf/                    // bounded output buffer (for maxOutputSize)
    └── testutil/                   // golden test helpers, comparison test runner
```

### 0.5 Testing strategy
- **Unit tests**: collocated `*_test.go`. Run via `go test ./...`.
- **Comparison tests**: port the JSON fixtures from `just-bash/packages/just-bash/src/comparison-tests/fixtures/`. Driver: `internal/testutil/comparison.go`. One Go test per fixture file. Tests assert stdout/stderr/exitCode equal the recorded bash output.
- **Spec tests**: port `src/spec-tests/` shell scripts and run them; mark known failures with `t.Skip` and a tracking issue link.
- **Property tests**: use `testing/quick` or `github.com/leanovate/gopter` for parser/expansion fuzz. Mirror `src/security/fuzzing/`.
- **Golden tests** for CLI: `cmd/gobash/testdata/*.golden`.

### 0.6 CI gates
A PR may not merge unless:
1. `go vet ./...` passes
2. `staticcheck ./...` passes
3. `go test -race ./...` passes
4. The comparison fixture diff is empty for fixtures marked `"locked": true`
5. `go build ./...` works with `CGO_ENABLED=0`

---

# PHASE 1 — Skeleton & Public API Surface

**Goal**: stand up the public types and a hello-world `Exec` that runs `echo hello` via a stubbed interpreter. No real functionality yet.

## 1.1 `go.mod`
```
module github.com/mark3labs/go-bash

go 1.22

require (
    mvdan.cc/sh/v3 v3.10.0
)
```

## 1.2 Public types — `bash.go`, `options.go`, `result.go`

```go
// Package gobash provides a sandboxed bash environment with a virtual filesystem.
package gobash

import (
    "context"
    "io"
    "sync"

    "github.com/mark3labs/go-bash/command"
    "github.com/mark3labs/go-bash/fs"
    "github.com/mark3labs/go-bash/network"
)

// Bash is a reusable shell environment. Safe for concurrent Exec calls
// (calls are internally serialized).
type Bash struct {
    mu       sync.Mutex
    fs       fs.FileSystem
    cwd      string
    env      map[string]string          // mutated across Exec calls (matches just-bash semantics)
    exported map[string]struct{}
    funcs    map[string]any             // *syntax.FuncDecl or interp.FuncDecl
    registry *command.Registry
    limits   ResolvedLimits
    fetch    network.Doer               // nil if network disabled
    sleep    SleepFunc                  // nil = real time.Sleep
    logger   Logger
    trace    TraceFunc
    jsBoot   string                     // js-exec bootstrap
    invoke   InvokeToolFunc             // js-exec tool hook
    procInfo ProcessInfo
    plugins  []transform.Plugin
    // internal: shellopts, shopts, exit code, etc.
}

func New(opts BashOptions) (*Bash, error)

func (b *Bash) Exec(ctx context.Context, script string, opts ExecOptions) (BashExecResult, error)

// RegisterTransformPlugin attaches a plugin applied to every Exec call.
func (b *Bash) RegisterTransformPlugin(p transform.Plugin)

// FS returns the filesystem (for inspection/manipulation by host code).
func (b *Bash) FS() fs.FileSystem
```

```go
// options.go

type BashOptions struct {
    Files           map[string]FileInit          // see §5.3
    Env             map[string]string
    Cwd             string                       // default "/home/user" if Files+Cwd both unset, else "/"
    FS              fs.FileSystem                // overrides default InMemoryFs
    ExecutionLimits *ExecutionLimits             // nil = defaults
    Fetch           network.Doer                 // custom fetcher
    Network         *network.Config              // allow-list config
    Python          *PythonConfig                // nil = disabled
    JavaScript      *JavaScriptConfig            // nil = disabled
    Commands        []command.Name               // nil = all built-ins
    Sleep           SleepFunc
    CustomCommands  []command.Command            // override built-ins
    Logger          Logger
    Trace           TraceFunc
    ProcessInfo     *ProcessInfo                 // virtual pid/ppid/uid/gid

    // Deprecated convenience knobs — still honored to match TS surface.
    MaxCallDepth      int
    MaxCommandCount   int
    MaxLoopIterations int
}

type ExecOptions struct {
    Env        map[string]string
    ReplaceEnv bool
    Cwd        string
    RawScript  bool
    Stdin      io.Reader      // nil = empty
    Stdout     io.Writer      // nil = buffer into ExecResult.Stdout
    Stderr     io.Writer      // nil = buffer into ExecResult.Stderr
    Args       []string       // appended to first command bypassing parsing
}

type ProcessInfo struct{ PID, PPID, UID, GID int }       // defaults 1, 0, 1000, 1000

type SleepFunc      = func(ctx context.Context, d time.Duration) error
type TraceFunc      = func(TraceEvent)
type InvokeToolFunc = func(ctx context.Context, path, argsJSON string) (string, error)

type TraceEvent struct {
    Category   string
    Name       string
    Duration   time.Duration
    Details    map[string]any
}

type Logger interface {
    Info(msg string, fields map[string]any)
    Debug(msg string, fields map[string]any)
}

type JavaScriptConfig struct {
    Bootstrap  string
    InvokeTool InvokeToolFunc
}

type PythonConfig struct {
    Runtime PythonRuntime  // see §15
}
```

```go
// result.go

type ExecResult struct {
    Stdout   string
    Stderr   string
    ExitCode int
}

type BashExecResult struct {
    ExecResult
    Env      map[string]string
    Metadata map[string]any
}
```

## 1.3 Stub `Exec`
Implement `Exec` to: parse the script via `mvdan.cc/sh/v3/syntax.NewParser().Parse()`, run it through `interp.New(...)` with no custom commands, and pass stdin/stdout/stderr. Set `BashExecResult.Env` from the interp runner's `Vars`. This will already handle simple scripts (`echo`, pipes, variables) using `mvdan/sh`'s built-in support before we add anything.

## 1.4 Acceptance for Phase 1
- `Bash` constructs with no options
- `bash.Exec(ctx, "echo hello", ExecOptions{})` returns `Stdout: "hello\n"`, `ExitCode: 0`
- `bash.Exec(ctx, "exit 7", ExecOptions{})` returns `ExitCode: 7`
- `context.Cancel` mid-script causes `Exec` to return `context.Canceled`

---

# PHASE 2 — Execution Limits

**Goal**: port `src/limits.ts` exactly.

## 2.1 `limits.go`
```go
type ExecutionLimits struct {
    MaxCallDepth             *int
    MaxCommandCount          *int
    MaxLoopIterations        *int
    MaxAwkIterations         *int
    MaxSedIterations         *int
    MaxJqIterations          *int
    MaxSqliteTimeout         *time.Duration
    MaxPythonTimeout         *time.Duration
    MaxJsTimeout             *time.Duration
    MaxGlobOperations        *int
    MaxStringLength          *int
    MaxArrayElements         *int
    MaxHeredocSize           *int
    MaxSubstitutionDepth     *int
    MaxBraceExpansionResults *int
    MaxOutputSize            *int
    MaxFileDescriptors       *int
    MaxSourceDepth           *int
}

type ResolvedLimits struct { /* same fields, non-pointer */ }

func ResolveLimits(in *ExecutionLimits) ResolvedLimits
```

Defaults (must match exactly):
| Field | Default |
|---|---|
| `MaxCallDepth` | 100 |
| `MaxCommandCount` | 10000 |
| `MaxLoopIterations` | 10000 |
| `MaxAwkIterations` | 10000 |
| `MaxSedIterations` | 10000 |
| `MaxJqIterations` | 10000 |
| `MaxSqliteTimeout` | 5 s |
| `MaxPythonTimeout` | 10 s (60 s with network) |
| `MaxJsTimeout` | 10 s (60 s with network) |
| `MaxGlobOperations` | 100000 |
| `MaxStringLength` | 10 MiB (10485760) |
| `MaxArrayElements` | 100000 |
| `MaxHeredocSize` | 10 MiB |
| `MaxSubstitutionDepth` | 50 |
| `MaxBraceExpansionResults` | 10000 |
| `MaxOutputSize` | 10 MiB |
| `MaxFileDescriptors` | 1024 |
| `MaxSourceDepth` | 100 |

## 2.2 `errors.go`
```go
type ExecutionLimitError struct {
    Limit string
    Value int
}
func (e *ExecutionLimitError) Error() string

type ParseError struct{ Msg string; Line, Col int }
type LexerError struct{ Msg string; Line, Col int }
type SecurityViolationError struct{ Msg string }   // kept for API parity; rarely fires in Go
type PosixFatalError struct{ Msg string; Code int }
type ArithmeticError struct{ Msg string }
type ExitError struct{ Code int }                  // internal: signals `exit N` builtin
type AbortedError struct{ Reason string }
```

## 2.3 Wire limits into the interpreter
- Wrap `interp.Runner` with our own runner that:
  - Counts commands and aborts when `MaxCommandCount` exceeded
  - Counts loop iterations (`for`, `while`, `until`) and aborts on `MaxLoopIterations`
  - Tracks call depth on function entry/exit; aborts on `MaxCallDepth`
  - Tracks source nesting on `source`/`.` builtin
  - Wraps writers in a `ringbuf.LimitedWriter` enforcing `MaxOutputSize` (returns `*ExecutionLimitError`)
- Limits on string growth (`MaxStringLength`) are enforced at the **expansion** layer — see Phase 4.

## 2.4 Acceptance for Phase 2
- `while true; do :; done` aborts with `ExecutionLimitError{Limit:"MaxLoopIterations"}`
- A script writing > MaxOutputSize bytes aborts with the same error type, `Limit:"MaxOutputSize"`
- Deep recursive shell function aborts on `MaxCallDepth`
- Defaults exactly match the table above (assert in tests)

---

# PHASE 3 — Filesystem Layer

**Goal**: port all 4 filesystems. This is the foundation everything else builds on.

## 3.1 `fs/fs.go` — Interface
```go
package fs

import (
    "io/fs"
    "os"
    "time"
)

// FileSystem is the writable VFS interface. Embeds io/fs.FS so callers can
// use stdlib helpers (fs.WalkDir, fs.ReadFile) on the sandbox.
type FileSystem interface {
    fs.FS
    fs.StatFS
    fs.ReadDirFS

    OpenFile(name string, flag int, perm os.FileMode) (File, error)
    Create(name string) (File, error)
    Mkdir(name string, perm os.FileMode) error
    MkdirAll(name string, perm os.FileMode) error
    Remove(name string) error
    RemoveAll(name string) error
    Rename(old, new string) error
    Symlink(target, link string) error
    Link(old, new string) error
    Readlink(name string) (string, error)
    Lstat(name string) (os.FileInfo, error)
    Realpath(name string) (string, error)
    Chmod(name string, mode os.FileMode) error
    Chtimes(name string, atime, mtime time.Time) error

    // ReadFile / WriteFile / AppendFile shortcuts (default impls via OpenFile in helpers).
    ReadFile(name string) ([]byte, error)
    WriteFile(name string, data []byte, perm os.FileMode) error
    AppendFile(name string, data []byte, perm os.FileMode) error

    // For glob expansion's getAllPaths() — may return empty if not supported.
    AllPaths() []string
}

type File interface {
    fs.File
    io.Writer
    io.Seeker
    Truncate(size int64) error
}
```

Provide a `BaseImpl` struct in `fs/base.go` that implements `ReadFile`/`WriteFile`/`AppendFile` via `OpenFile`, so concrete FS implementations only need to implement the primitives.

## 3.2 `fs/path.go` — Path utilities
Port from `src/fs/path-utils.ts`:
- `Clean(path string) string` — POSIX clean, normalizes `..`, removes trailing `/` except root
- `Join(parts ...string) string`
- `Resolve(base, path string) string` — if `path` absolute return Clean(path); else Clean(Join(base, path))
- `Validate(path string) error` — rejects null bytes (`\x00`), rejects empty
- `Dirname(path string) string`, `Basename(path string) string`
- `MaxSymlinkDepth = 40` (matches Linux)
- `IsWithinRoot(root, path string) bool`

## 3.3 `fs/memfs` — InMemoryFs
Port from `src/fs/in-memory-fs/in-memory-fs.ts`.

- Tree of `*node` keyed by name. Node has type (file/dir/symlink), mode, mtime, content (for files), target (for symlinks).
- **Lazy file providers**: when constructed via `BashOptions.Files`, entries whose `FileInit.Lazy` is non-nil are stored as `lazyNode`s. First read calls the provider and replaces the node with a regular file. If a write occurs before any read, the provider is never called.
- Hard links share content: implement via a shared `*fileContent` pointer; rename/unlink decrements refcount.
- Symlinks: stored as-is; `Open` follows them; `Lstat`/`Readlink` do not.
- All ops are protected by an internal `sync.RWMutex`.

**Acceptance**: pass a battery of tests mirroring `fs/in-memory-fs/*.test.ts` and `fs/interface.contract.test.ts`.

## 3.4 `fs/realfs` — Shared real-FS security
Port from `src/fs/real-fs-utils.ts`:
- `ResolveAndValidate(root, requested string, allowSymlinks bool) (canonical string, err error)`
- Implements the "compare `realPath[len(root):]` vs `canonical[len(canonicalRoot):]`" zero-extra-I/O symlink check.
- `OpenFileNoFollow(path string, flags int, mode os.FileMode) (*os.File, error)` — uses `syscall.O_NOFOLLOW` when `allowSymlinks=false`.
- `SanitizeError(err error, root string) error` — strips real paths from messages.

## 3.5 `fs/overlayfs` — OverlayFs
Port from `src/fs/overlay-fs/`.

```go
type Options struct {
    Root          string                 // required, must be abs
    AllowSymlinks bool                   // default false
    ReadOnly      bool                   // if true, writes return EROFS
    MountPoint    string                 // default "/" — where reads appear in VFS
}

func New(opts Options) (*OverlayFs, error)
func (*OverlayFs) MountPoint() string
```

Semantics:
- Reads: serve from overlay (in-memory) if present; else from real disk under `Root`
- Writes: always go into overlay; mark path "dirty" so reads see the overlay
- Deletes: store a tombstone in overlay
- Listing: union of real readdir + overlay entries, minus tombstones
- Hardens symlinks by default (`AllowSymlinks=false`)

## 3.6 `fs/rwfs` — ReadWriteFs
Port from `src/fs/read-write-fs/`. Direct read-write under `Root`. Same symlink hardening, same path containment, same TOCTOU protections (`O_NOFOLLOW`, post-mkdir re-validation).

## 3.7 `fs/mountfs` — MountableFs
Port from `src/fs/mountable-fs/`.

```go
type Options struct {
    Base   FileSystem            // required
    Mounts []Mount
}
type Mount struct {
    Path       string            // mount point in unified namespace
    FileSystem FileSystem
}

func (*MountableFs) Mount(path string, fs FileSystem) error
func (*MountableFs) Unmount(path string) error
```

Resolves each call to the longest-prefix mount and translates the path. Cross-mount `Rename`/`Link` fall back to copy+delete.

## 3.8 Acceptance for Phase 3
- Each FS passes a shared contract test suite (`fs/contract_test.go`) covering: file CRUD, dir CRUD, rename, hard link, symlink, lstat, readlink, realpath, chmod, chtimes, mkdir recursive, rm recursive, walk.
- Symlink escape tests pass: `Symlink("/etc/passwd", "x")` followed by `Open("x")` returns `EPERM` (or `ENOENT`).
- Null byte injection: `ReadFile("a\x00b")` returns error.
- Path traversal: `OverlayFs{Root:"/tmp/sandbox"}.ReadFile("/etc/shadow")` returns `ENOENT`.

---

# PHASE 4 — Parser & AST (via mvdan/sh)

**Goal**: produce a `ScriptNode` AST equivalent to `src/ast/types.ts`. Use `mvdan.cc/sh/v3/syntax` under the hood; translate its AST to our own.

## 4.1 AST design (`ast/`)
Mirror the TS shape so transform plugins can be ported with minimal change:

```go
package ast

type Node interface{ nodeMarker() }

type Script struct {
    Statements []*Statement
    Line       int
}

type Statement struct {
    Pipelines  []*Pipeline
    Operators  []string         // each "&&" | "||" | ";"
    Background bool
    Line       int
}

type Pipeline struct {
    Commands   []Command         // SimpleCommand | Subshell | Group | IfStmt | ForStmt | ... | FunctionDef
    PipeStderr []bool            // length = len(Commands) - 1
    Negated    bool
    Line       int
}

type Command interface{ Node; commandMarker() }

type SimpleCommand struct {
    Assignments  []*Assignment
    Name         *Word            // may be nil for assignment-only
    Args         []*Word
    Redirections []*Redirection
    Line         int
}

type Assignment struct {
    Name   string
    Value  *Word
    Append bool                   // +=
    Array  *ArrayInit             // nil unless `a=(...)`
}

type Word struct{ Parts []WordPart }

type WordPart interface{ wordPartMarker() }
type Literal              struct{ Value string }
type SingleQuoted         struct{ Value string }
type DoubleQuoted         struct{ Parts []WordPart }
type ParameterExpansion   struct{ Parameter string; Operation any }
type CommandSubstitution  struct{ Body []*Statement; Backtick bool }
type ArithmeticExpansion  struct{ Expr string /* keep raw, evaluate at runtime */ }
type ProcessSubstitution  struct{ Direction string; Body []*Statement }
type AnsiCQuoted          struct{ Value string }    // $'...'

// Compound commands:
type IfStmt   struct{ Branches []IfBranch; Else []*Statement; ... }
type ForStmt  struct{ Var string; Words []*Word; Body []*Statement; ... }
type CStyleFor struct{ Init, Cond, Post string; Body []*Statement }
type WhileStmt struct{ Cond, Body []*Statement; Until bool }
type CaseStmt struct{ Subject *Word; Items []CaseItem }
type Subshell  struct{ Body []*Statement; Redirections []*Redirection }
type Group     struct{ Body []*Statement; Redirections []*Redirection }
type FunctionDef struct{ Name string; Body Command }
type ArithCmd  struct{ Expr string }
type CondCmd   struct{ Expr CondExpr }       // [[ ... ]]
type DBracket  = CondCmd

type Redirection struct {
    FD       int
    Op       string              // ">", ">>", "<", "<<", "<<<", "<>", "2>&1", ...
    Word     *Word
    Heredoc  *Heredoc            // nil unless << or <<-
}
type Heredoc struct{ Tag, Body string; StripTabs, Expand bool }
```

Keep parameter expansion `Operation` as a closed interface implemented by:
`Length`, `DefaultValue`, `Assign`, `Error`, `Alternative`, `SubstringRange`, `Replace`, `PatternRemove`, `CaseModify`, `Indirect`, `Transform`, `Names`, `Keys`, `ArrayIndex`, etc.

## 4.2 `parser/parser.go`
```go
func Parse(src string) (*ast.Script, error)
func ParseString(src string) (*ast.Script, error)        // alias
```

- Internally call `syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")`.
- Walk the resulting `*syntax.File` and build our `*ast.Script`. This translator lives in `parser/translate.go`.
- Enforce the parser-side hard limits:
  - `MaxInputSize = 1 << 20` (1 MiB) — pre-check on `len(src)`
  - `MaxTokens = 100000` — count `syntax.Walk` visits
  - `MaxParserDepth = 200` — track during translation
  - Heredoc body size <= `MaxHeredocSize`
  - Return `*ParseError` with line/column on failure

## 4.3 `transform/serialize.go`
Implement `Serialize(*ast.Script) string` — the inverse of Parse. Used by transform plugins.

- The straightforward approach is to bypass our AST and use `mvdan.cc/sh/v3/syntax.Printer`. But transform plugins mutate **our** AST. So:
  - Maintain a reverse translator: `astToSh(*ast.Script) *syntax.File`
  - Use `syntax.NewPrinter().Print(w, file)` to serialize
- For plugin-synthesized nodes (no original syntax pos), populate positions with `syntax.Pos{}` and let the printer use defaults.

## 4.4 Acceptance for Phase 4
- `Parse("echo $((1+2)) | grep o")` returns a `*ast.Script` whose first statement has one pipeline of two SimpleCommands
- `Serialize(Parse(src))` round-trips a curated set of 100+ representative scripts to byte-identity or equivalent semantics
- Limits enforced on pathological inputs

---

# PHASE 5 — Interpreter Bridge

**Goal**: replace the stub `Exec` with a real interpreter that walks our AST and dispatches to our command registry — backed by `mvdan/sh`'s `interp` package for the heavy lifting (expansion, redirection, control flow).

## 5.1 Strategy
We do **not** translate our AST to `mvdan/sh`'s AST and run `interp`. Instead:

1. Parse via `mvdan/sh/syntax` → `*syntax.File`
2. Run via `mvdan/sh/interp.Runner` with extensive customization hooks
3. Use the AST translator only for the **Transform API** (Parse → Translate → Plugins → Serialize → re-Parse → Run)

This lets us inherit `mvdan/sh`'s correct semantics for parameter expansion, brace expansion, arithmetic, globbing, `[[ ]]`, etc. We only need to implement the **commands** and **filesystem hooks**.

## 5.2 `interp/runner.go`
```go
type Runner struct{ /* wraps *interp.Runner */ }

func (r *Runner) Run(ctx context.Context, file *syntax.File) (exitCode int, err error)
```

Configure `interp.New` with:
- `interp.StdIO(stdin, stdout, stderr)` — wrap stdout/stderr in `ringbuf.LimitedWriter`
- `interp.Env(expand.ListEnviron(envSlice...))` — initial env
- `interp.Dir(opts.Cwd)`
- `interp.ExecHandlers(commandExecHandler)` — dispatches every external command to our registry
- `interp.OpenHandler(fsOpenHandler)` — all file opens go through our VFS
- `interp.StatHandler(fsStatHandler)`
- `interp.ReadDirHandler(fsReadDirHandler)`
- `interp.CallHandler(callHandler)` — for limit counting (depth, command count)

## 5.3 `commandExecHandler`
```go
func (b *Bash) commandExecHandler(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
    return func(ctx context.Context, args []string) error {
        // 1) Look up args[0] in our registry (custom > built-in)
        // 2) Build CommandContext from interp.HandlerCtx(ctx)
        // 3) Call cmd.Execute(ctx, args[1:], cctx)
        // 4) Translate the result into interp.NewExitStatus(code) and write
        //    stdout/stderr from the Result to the handler's writers.
        // 5) For commands not in our registry: return interp.ErrNotFound
        //    (mvdan/sh will then fall back to its default — which we override
        //    to also return "command not found" rather than exec'ing real binaries).
    }
}
```

**Critical**: the default `interp.DefaultExecHandler` *will* `os/exec` real binaries. Replace it entirely. Never invoke `os/exec.Command` anywhere in the codebase. Lint rule: `staticcheck` + a custom `internal/lint/no-os-exec` check.

## 5.4 `fsOpenHandler`, `fsStatHandler`, `fsReadDirHandler`
Translate `interp.HandlerCtx.Dir` + the requested path through `Bash.fs` (which is one of our `FileSystem` impls). Return `*os.PathError` shaped errors so `mvdan/sh` reports them like real shells.

## 5.5 Cancellation
- `Exec` ctx is the one passed to `interp.Runner.Run`. `mvdan/sh` already honors it at statement boundaries.
- Build a `*ringbuf.LimitedWriter` that also returns an error when `ctx.Err() != nil`.

## 5.6 Env mutation semantics
`just-bash` **mutates** the env across `Exec` calls (`bash.Exec("export X=1")` then `bash.Exec("echo $X")` prints `1`). After each `Exec`:
- Copy back the runner's `Vars` into `Bash.env` (export-only) **unless** `ExecOptions.Env` was provided without `ReplaceEnv` — in which case we restore the snapshot taken before `Exec` (the TS code does this too).
- This must match exactly. Add a comparison test.

## 5.7 Acceptance for Phase 5
- `for i in 1 2 3; do echo $i; done` outputs `1\n2\n3\n`
- `x=1; (x=2; echo $x); echo $x` → `2\n1\n`
- `bash.Exec("export X=hello"); bash.Exec("echo $X")` → `hello\n`
- `bash.Exec("X=ephemeral env", ExecOptions{Env: map[string]string{"X":"once"}})` does not pollute the env
- All ~50 sample scripts from `src/comparison-tests/` for plain shell features pass

---

# PHASE 6 — Expansion & Shell Features

Most of this is delivered "for free" by `mvdan/sh`. **Verify** each feature listed below works correctly against the comparison fixtures; **patch** the few that don't via `interp` hooks or pre-Parse rewriting.

## 6.1 Variables
- `$VAR`, `${VAR}`, `${VAR:-default}`, `${VAR:=assign}`, `${VAR:?error}`, `${VAR:+alt}`
- `${VAR:offset:length}` (substring)
- `${VAR/pat/repl}`, `${VAR//pat/repl}`, `${VAR/#pat/repl}`, `${VAR/%pat/repl}`
- `${#VAR}` (length)
- `${VAR#pat}`, `${VAR##pat}`, `${VAR%pat}`, `${VAR%%pat}`
- `${VAR^^}`, `${VAR^}`, `${VAR,,}`, `${VAR,}` (case)
- `${VAR@U}`, `${VAR@L}`, `${VAR@Q}`, `${VAR@E}`, `${VAR@P}`, `${VAR@A}`, `${VAR@K}`, `${VAR@a}` (transforms)
- `${!ref}` (indirect), `${!prefix*}`, `${!prefix@}` (names by prefix)
- Arrays: `${arr[i]}`, `${arr[@]}`, `${arr[*]}`, `${!arr[@]}` (keys), `${#arr[@]}`

## 6.2 Positional parameters
- `$1..$9`, `${10}+`, `$@`, `$*`, `$#`, `$0`
- `shift [N]`, `set -- ...`
- `"$@"` expands element-per-word; `"$*"` joins with first char of `$IFS`

## 6.3 Special parameters
- `$$` (virtual pid from `ProcessInfo`), `$PPID`, `$!`, `$?`, `$-`, `$_`, `$BASHPID`
- `$RANDOM` — seed from script start time XOR call counter (deterministic per Bash instance for tests)
- `$LINENO`, `$FUNCNAME`, `$BASH_SOURCE`, `$BASH_LINENO`, `$SECONDS`, `$EPOCHSECONDS`, `$EPOCHREALTIME`
- `$UID`, `$EUID`, `$GID` from `ProcessInfo`
- `$SHELLOPTS`, `$BASHOPTS` (readonly, built from current options)
- `$PWD`, `$OLDPWD`, `$HOME`, `$PATH`, `$IFS`, `$OSTYPE=linux-gnu`, `$MACHTYPE=x86_64-pc-linux-gnu`, `$HOSTTYPE=x86_64`, `$HOSTNAME=localhost`

## 6.4 Brace expansion
`{a,b,c}`, `{1..10}`, `{1..10..2}`, `{a..z}`, `{a..z..2}`, nested, with prefix/suffix.
**Limit**: `MaxBraceExpansionResults`. If `mvdan/sh` lacks the limit, wrap pre-parse with our own counter that walks `*syntax.BraceExp` and rejects oversize.

## 6.5 Globs
`*`, `?`, `[abc]`, `[!abc]`, plus extglob when `shopt -s extglob`:
`?(pat)`, `*(pat)`, `+(pat)`, `@(pat)`, `!(pat)`.

`**` only when `shopt -s globstar`. Implement `MAX_GLOBSTAR_SEGMENTS = 5` rejection.

Other shopt flags to honor: `dotglob`, `nullglob`, `failglob`, `globskipdots` (default on), `nocaseglob`, `nocasematch`.

Backend: `internal/glob` package — port `src/shell/glob-to-regex.ts`. Tests must accept the same inputs and produce the same matches as bash.

## 6.6 Pathname expansion
Routes through `Bash.fs.AllPaths()` for in-memory FS (cheap) or readdir-walk for real FS (with `MaxGlobOperations` counter).

## 6.7 Arithmetic
`$((expr))`, `((expr))`, `let`, `i=$((i+1))`, `${arr[$((i*2))]}`.
Operators: `+ - * / % ** << >> & | ^ ~ ! && || == != < <= > >= ?: , = += -= *= /= %= **= <<= >>= &= |= ^= ++ --`.
Integer only (signed 53-bit to match TS; document as `int64` for Go). Overflow clamps to `math.MaxInt64`. Division by zero → `*ArithmeticError`.

## 6.8 Conditionals
- `[ ... ]` (test) builtin
- `[[ ... ]]` extended test: `==`, `!=`, `=`, `=~` (regex), `<`, `>`, file tests (`-f`, `-d`, `-r`, `-w`, `-x`, `-L`, `-e`, `-s`, etc.), `&&`/`||` short-circuit
- `=~` uses Go `regexp` (RE2). Captures populate `BASH_REMATCH`.

## 6.9 Redirections (full list)
`>`, `>>`, `<`, `<>`, `>|`, `&>`, `&>>`, `>&N`, `<&N`, `<<TAG`, `<<-TAG`, `<<<STRING`, `2>&1`, `2>`, `N>FILE`, `N<FILE`, `exec N>FILE`.

Process substitution `<(cmd)`, `>(cmd)`: in `just-bash`, these are implemented via temp files in the VFS. Do the same — write the subcommand's output to `/tmp/.gobash-procsub-<N>` and pass the path to the parent. Track FDs against `MaxFileDescriptors`.

## 6.10 Control flow
- `if/elif/else/fi`
- `for x in ...; do; done`, `for ((init;cond;post)); do; done`
- `while cond; do; done`, `until cond; do; done`
- `case word in pat) ;; esac`
- `select x in ...; do; done` — interactive; in `just-bash` it reads from stdin
- `break [N]`, `continue [N]`
- `&&`, `||`, `;`, `&` (background — emulated; see §6.11)

## 6.11 Background jobs (emulated)
`cmd &` spawns a goroutine. `wait` blocks until all background goroutines finish. `$!` returns a virtual PID (counter starting at `ProcessInfo.PID+1`). Job control (`fg`, `bg`, `jobs`) is a stub that matches bash output but doesn't manipulate real processes. Match `just-bash`'s exact behavior.

## 6.12 Functions
```bash
foo() { echo hi; }
function foo { echo hi; }
foo a b c
```
- Local variables via `local` builtin
- `return [N]` builtin
- Function definitions stored on `Bash.funcs` so they persist across `Exec` calls (matches TS — verify in TS code; the env is shared but functions are reset between exec calls per the README). **Check**: re-read TS Bash.ts to confirm; the README says only env/cwd reset, not functions, but `bash.ts` shows `functions: new Map()` is set in constructor and not reset per-exec → port that behavior verbatim.

## 6.13 Acceptance
- All comparison fixtures under `comparison-tests/` for shell features pass

---

# PHASE 7 — Filesystem Init & Default Layout

Port `src/fs/init.ts`.

When `New(BashOptions{})` is called with no `Cwd` and no `Files`:
- Use default layout `useDefaultLayout = true`
- Create directories: `/`, `/home`, `/home/user`, `/bin`, `/usr`, `/usr/bin`, `/tmp`, `/etc`, `/dev`, `/proc`, `/proc/self`
- Create `/etc/hostname` = `"localhost\n"`
- Create `/proc/self/status` = templated with virtual PID/UID/GID
- For each registered command `X`, create `/bin/X` as a no-op stub file (mode 0755). This lets `which`, `command -v`, and absolute-path invocation work.
- `$HOME=/home/user`, `$PATH=/usr/bin:/bin`

When `Cwd` is set or `Files` is provided, skip the default layout but still create needed directories under `Cwd`.

---

# PHASE 8 — Command Registry

Port `src/commands/registry.ts`.

## 8.1 `command/command.go`
```go
package command

type Name string

type Command interface {
    Name() Name
    Execute(ctx context.Context, args []string, c *Context) Result
    Trusted() bool                      // analog of TS `trusted` flag — used by sandbox subpackage only
}

type Context struct {
    FS          fs.FileSystem
    Cwd         string
    Env         map[string]string         // mutable view; commands may set/unset
    ExportedEnv map[string]string         // for printenv/env
    Stdin       io.Reader
    Stdout      io.Writer
    Stderr      io.Writer
    Limits      ResolvedLimits
    Trace       TraceFunc
    Fetch       network.Doer              // nil if disabled
    Sleep       SleepFunc
    Exec        func(ctx context.Context, script string, opts SubExecOptions) (Result, error)
    Registry    *Registry
    Signal      <-chan struct{}           // closed when parent ctx done
    JSBootstrap string
    InvokeTool  InvokeToolFunc
    FDs         map[int]io.ReadWriter     // for exec N>file etc.
}

type Result struct {
    Stdout   string             // populated if Stdout writer was nil
    Stderr   string             // populated if Stderr writer was nil
    ExitCode int
}

type SubExecOptions struct { /* mirrors top-level ExecOptions, Cwd required */ }
```

## 8.2 `command/registry.go`
```go
type Registry struct {
    cmds map[Name]Command
}

func New() *Registry
func (r *Registry) Register(c Command)
func (r *Registry) Lookup(name string) (Command, bool)
func (r *Registry) Names() []Name
```

## 8.3 Lazy loading
TS uses dynamic `import()`. In Go, just include all built-ins by importing their subpackages with side-effect registration:

```go
// builtins/builtins.go
import (
    _ "github.com/mark3labs/go-bash/builtins/cat"
    _ "github.com/mark3labs/go-bash/builtins/ls"
    // ... all builtins
)
```

Each builtin's `init()` registers itself in a package-level slice. `command.NewDefault()` returns a registry populated from that slice. Filter by `BashOptions.Commands` if set.

Optional command groups (`network`, `python`, `javascript`) live in separate registration functions:
- `RegisterNetwork(r *Registry, fetch Doer)` — adds `curl`, `html-to-markdown` (`html-to-markdown` doesn't need fetch but it's tied to the same group per TS)
- `RegisterPython(r *Registry, rt PythonRuntime)` — adds `python`, `python3`
- `RegisterJavaScript(r *Registry, cfg JavaScriptConfig)` — adds `js-exec`, `node`

## 8.4 `defineCommand` equivalent
```go
func Define(name string, fn func(ctx context.Context, args []string, c *Context) Result) Command
```

---

# PHASE 9 — Network Layer

Port `src/network/`.

## 9.1 `network/config.go`
```go
type Config struct {
    AllowedURLPrefixes              []AllowedURLEntry
    AllowedMethods                  []string         // default ["GET","HEAD"]
    DangerouslyAllowFullAccess      bool
    MaxRedirects                    int              // default 20
    Timeout                         time.Duration    // default 30s
    MaxResponseSize                 int64            // default 10 MiB
    DenyPrivateRanges               bool
    DNSResolve                      func(host string) ([]DNSLookupResult, error)  // testing
}

type AllowedURLEntry struct {
    URL       string
    Transform []RequestTransform
}

type RequestTransform struct {
    Headers map[string]string
}

type DNSLookupResult struct { Address string; Family int }
```

## 9.2 `network/allowlist.go`
- Parse each `URL` entry via `net/url.Parse`; reject if missing scheme or host
- Normalize origin to `scheme://host[:port]`; default ports 80/443 stripped
- Reject `%2f` / `%5c` ambiguity in path prefix
- Per-redirect re-validation

## 9.3 `network/securefetch.go`
```go
type Doer interface {
    Do(ctx context.Context, req *http.Request) (*Response, error)
}

type Response struct {
    Status     int
    StatusText string
    Headers    http.Header
    Body       []byte         // bounded by MaxResponseSize
    URL        string         // final URL after redirects
}

func NewSecureFetch(cfg *Config) Doer
```

Implementation:
- Custom `http.Client{ CheckRedirect: ..., Transport: &transport{} }`
- `transport.RoundTrip` injects header transforms by re-matching the URL on each hop
- `CheckRedirect` re-validates the destination against the allow-list and method
- Wrap response body in `io.LimitReader(body, MaxResponseSize+1)`, error if exceeded
- If `DenyPrivateRanges`: resolve via `DNSResolve` (or `net.DefaultResolver.LookupIP`), reject any private/loopback/link-local addresses (RFC1918, ::1, fe80::/10, etc.)

## 9.4 Errors
- `NetworkAccessDeniedError`
- `TooManyRedirectsError`
- `RedirectNotAllowedError`
- `MethodNotAllowedError`
- `ResponseTooLargeError`

All match TS message format for parity.

---

# PHASE 10 — Built-in Commands

This is the largest phase. **Order matters**: ship simple commands first to validate the harness, then complex ones.

For every command:
1. Create `builtins/<name>/<name>.go` with the implementation
2. Create `builtins/<name>/<name>_test.go` with unit tests
3. Create `builtins/<name>/<name>_comparison_test.go` that loads the matching fixture from `internal/testdata/fixtures/<name>/`
4. Register in `init()` via `command.RegisterBuiltin(...)`
5. Always support `--help` (print usage, exit 0) and `-h` where standard
6. Error on unknown options (exit 2 with `usage: ...` to stderr) — **except** when real bash silently ignores them
7. Honor `$LC_ALL=C` for byte-locale sort/grep/etc.

### Wave A — trivial (ship first, ~1 day)
| Cmd | Notes |
|---|---|
| `true` | exit 0 |
| `false` | exit 1 |
| `clear` | output ANSI clear sequence `\033[H\033[2J` |
| `pwd` | `-L` (logical, default) vs `-P` (physical, resolve symlinks via `fs.Realpath`) |
| `echo` | flags: `-n`, `-e`, `-E`; honor `xpg_echo` shopt |
| `printf` | implement bash semantics (`%b`, `%q`, `%(fmt)T`, format reuse over extra args); see §10.18 |
| `basename` | `-a`, `-s SUFFIX`, optional SUFFIX arg |
| `dirname` | bash semantics |
| `hostname` | reads from `/etc/hostname`; defaults to `"localhost"` |
| `whoami` | always `"user"` (matches just-bash) |
| `which` | searches `$PATH` against registry + filesystem |
| `sleep` | uses `Context.Sleep` if set, else `time.Sleep`; supports `1`, `1s`, `1m`, `1h`, `1d`; fractional values |
| `seq` | start/end/incr semantics matching coreutils |
| `expr` | arithmetic + string ops |

### Wave B — file ops
| Cmd | Notes |
|---|---|
| `ls` | flags `-l -a -A -d -F -h -i -R -1 -t -S -r --color -L -n -p` (port TS exactly). Use `column` helper for multi-col output. |
| `mkdir` | `-p`, `-m MODE` |
| `rmdir` | `-p` |
| `touch` | `-c`, `-m`, `-a`, `-t TIMESTAMP`, `-r REFFILE`, `-d DATE` |
| `rm` | `-r`, `-rf`, `-i` (interactive — stub: rejects), `-d` |
| `cp` | `-r`, `-R`, `-a`, `-p`, `-f`, `-i` (reject), `-L`, `-P`, `-T` |
| `mv` | preserve perms; cross-FS via copy+rm if `fs.Rename` returns EXDEV |
| `ln` | `-s` symlink, `-f` force, hard links by default |
| `chmod` | numeric `755`, symbolic `u+x,g-w,o=r`, recursive `-R`, `-v` |
| `readlink` | `-f`, `-e`, `-m`, `-n` |
| `stat` | `-c FORMAT` with all bash `%` codes (`%n %s %F %a %A %u %g %x %y %z %i %h %d %t %T`) |
| `tree` | depth `-L`, dirs-only `-d`, all `-a`, JSON `-J`, colors `-C` (off by default) |
| `du` | `-s`, `-h`, `-a`, `-c`, `-d DEPTH`, `--max-depth=N`, `-b`, `-k`, `-m`, `-x` (no-op in single FS) |
| `file` | mime detection via `mimetype` lib; output format matches `file` command exactly |
| `split` | `-l N`, `-b N[K\|M\|G]`, `-a SUFFIXLEN`, `-d`, `--filter` |

### Wave C — text processing
| Cmd | Notes |
|---|---|
| `cat` | `-n`, `-b`, `-A`, `-E`, `-T`, `-s`, `-v` |
| `head` | `-n N`, `-n -N` (drop trailing), `-c N` |
| `tail` | `-n N`, `-n +N`, `-c N`, `-f` (rejected in sandbox; print error) |
| `wc` | `-l`, `-w`, `-c`, `-m`, `-L` |
| `cut` | `-f`, `-c`, `-b`, `-d`, `--complement`, `--output-delimiter`, `-s` |
| `paste` | `-d`, `-s` |
| `tr` | classes (`[:alpha:]`, `[:digit:]`, ...), `-d`, `-s`, `-c`, ranges |
| `rev` |  |
| `nl` | `-b`, `-n`, `-w`, `-s` |
| `fold` | `-w`, `-s`, `-b` |
| `expand` | tabs to spaces, `-t LIST`, `-i` |
| `unexpand` | reverse |
| `strings` | `-n MIN`, `-a`, `-t`, finds printable runs in binary |
| `tac` |  |
| `sort` | `-n`, `-r`, `-u`, `-k`, `-t`, `-f`, `-V`, `-h`, `-R`, `-s` (stable), `-c`, `-m`, `-z` |
| `uniq` | `-c`, `-d`, `-D`, `-u`, `-i`, `-f`, `-s`, `-w` |
| `comm` | `-1 -2 -3` |
| `join` | `-1 N -2 N -t SEP -a FILE -e EMPTY -o FORMAT` |
| `tee` | `-a` append; honor `MaxFileDescriptors`; bytes through, not text |
| `column` | `-t`, `-s SEP`, `-o SEP`, `-n` |
| `find` | massive: `-name`, `-iname`, `-type`, `-size`, `-mtime`, `-newer`, `-prune`, `-exec`, `-execdir`, `-print`, `-print0`, `-not`, `-and`, `-or`, parens; honor `MaxGlobOperations` |
| `xargs` | `-0`, `-n`, `-I`, `-P` (parallel — emulate via goroutines bounded by N), `--max-args`, `-d DELIM`, `-a FILE` |
| `od` | `-A`, `-t`, `-v`, `-w` |
| `base64` | encode/decode, `-d`, `-w N` |
| `md5sum`, `sha1sum`, `sha256sum` | output format matches GNU coreutils |
| `diff` | `-u`, `-q`, `-r`, `-N`, `-y`, exit codes 0/1/2 |

### Wave D — pattern engines
| Cmd | Notes |
|---|---|
| `grep` / `egrep` / `fgrep` | flags `-E -F -i -v -n -c -l -L -H -h -r -R -w -x -o -A -B -C -e -f -q -s --include --exclude --color`. Use Go `regexp` (RE2). Multi-file = sort filenames. |
| `rg` (ripgrep subset) | port the subset just-bash implements: `-i -v -n -c -l -A -B -C -e -t TYPE -g GLOB --hidden --no-ignore --json` |
| `sed` | port `src/commands/sed/`. Implement parser + executor in `builtins/sed/`. Commands: `s/// (flags g/i/p/N)`, `d`, `p`, `q`, `n`, `N`, `a\\`, `i\\`, `c\\`, `y///`, `=`, addresses (`N`, `$`, `/regex/`, ranges `N,M`, `N~step`), labels `:`, `b LABEL`, `t LABEL`, `T LABEL`. Extended regex `-E`/`-r`. **Limits**: `MaxSedIterations`. |
| `awk` | port `src/commands/awk/`. Wrap `github.com/benhoyt/goawk/interp` — but verify its feature parity matches the `just-bash` AWK first; if there are gaps, write a thin shim. **Limits**: `MaxAwkIterations` — enforced via a custom hook in goawk's loop guard (file an issue if upstream lacks one; fall back to time-based abort via context). |
| `jq` | wrap `github.com/itchyny/gojq`. Honor `--raw-input`, `--raw-output`, `--slurp`, `--compact`, `--null-input`, `-c`, `-n`, `-r`, `-s`, `-R`, `--arg`, `--argjson`. `MaxJqIterations` enforced via `gojq.Code.Run` ticker. |

### Wave E — data formats
| Cmd | Notes |
|---|---|
| `yq` | YAML/JSON/XML/TOML/CSV in & out. Port `src/commands/yq/`. Build atop `goccy/go-yaml`, `BurntSushi/toml`, `encoding/xml`, `encoding/csv`. Use `gojq` to evaluate `.` expressions. |
| `xan` | CSV toolkit (subset of [xan](https://github.com/medialab/xan)). Port commands implemented in `src/commands/xan/`: `select`, `slice`, `filter`, `count`, `flatten`, `headers`, `stats`, `cat`, `to`, `from`. |
| `sqlite3` | OPTIONAL — see Phase 14. Default: registered only when SQLite subpackage is imported and `BashOptions` opts in. |

### Wave F — archive / compression
| Cmd | Notes |
|---|---|
| `gzip` / `gunzip` / `zcat` | `-d`, `-k`, `-c`, `-N`, `-r`, `-l`, levels `-1..-9`. `compress/gzip`. |
| `tar` | `-c -x -t -v -f -z (gzip) -j (bzip2) -J (xz) --zstd (zstd) --no-same-owner --strip-components`. `archive/tar` + the codec libs. Honor `MaxStringLength` on any single member. |

### Wave G — environment / shell
| Cmd | Notes |
|---|---|
| `env` | `-i`, `-u VAR`, `NAME=VALUE prog args...` (sets env then exec). When called with no args, list exported env |
| `printenv` | list exported env |
| `alias` / `unalias` | shared state on `Bash.aliases`. Aliases only expand at parse time when `shopt expand_aliases` is on. |
| `history` | virtual ring buffer of last 500 commands per `Bash` instance |
| `date` | full strftime; **defaults to UTC** unless `$TZ` is set in env. `-u` forces UTC. `-d STRING` parses GNU date-ish input. `-r FILE` uses file mtime. `+FORMAT`. |
| `timeout` | `-s SIG -k KILL_AFTER DURATION cmd...`. Run via `Context.Exec` with a derived `ctx`; on timeout return 124. |
| `time` | run command and print elapsed time to stderr |
| `bash` / `sh` | `-c SCRIPT`, `-e`, `-x`, `-n`, `-s`, `--`. Recursive `Context.Exec`. Tracked against `MaxSourceDepth`. |
| `help` | list registered commands (uses `Context.Registry.Names()`) |

### Wave H — network
| Cmd | Notes |
|---|---|
| `curl` | only registered when `Network` or `Fetch` configured. Support `-X -H -d --data-binary -o -O -s -S -L -i -I -k -u -A -e -f --max-time -w -F (form) --data-urlencode -G`. Use `network.Doer`. |
| `html-to-markdown` | reads HTML from stdin, writes Markdown to stdout. Uses `html-to-markdown/v2` |

### 10.18 `printf` details (called out because hard)
- Bash supports `%b` (backslash escapes), `%q` (shell-quoted), `%(strftime)T` for timestamps, format reuse cycling extra args, integer base prefixes `'X` (single-char arg → ASCII)
- Go's `fmt.Sprintf` does NOT do these. Write a small printf in `internal/printf` that:
  1. Parses the format string into tokens
  2. Dispatches `%d`/`%s`/`%x` etc. to `fmt`
  3. Handles `%b`, `%q`, `%(...)T` natively
  4. Cycles the format over remaining args (`printf "%s\n" a b c` → three lines)

### 10.19 Acceptance per command
- All comparison fixtures under `comparison-tests/fixtures/<cmd>/` pass byte-for-byte for unlocked fixtures
- Locked fixtures (Linux-adjusted by `just-bash` maintainers) pass after re-recording in the equivalent Go runner

---

# PHASE 11 — Interpreter Built-ins (Shell Built-in Commands)

These are commands that affect interpreter state and cannot be implemented as external commands. Port `src/interpreter/builtins/`.

| Builtin | Notes |
|---|---|
| `cd` | `-L` / `-P`, `-`, `~`, sets `PWD`/`OLDPWD` |
| `set` | `-e`, `-u`, `-x`, `-o pipefail`, `-o noglob`, `-o errexit`, `+e` to disable, `set --` to clear positionals, `set arg1 arg2` to set positionals |
| `shopt` | flags listed in §6.5; `-s`, `-u`, `-p`, `-q` |
| `export` | `export VAR`, `export VAR=val`, `export -p`, `export -n` |
| `unset` | `-v`, `-f` |
| `local` | within functions; throws outside |
| `declare` / `typeset` | `-a -A -i -r -x -p -n` (nameref); array initialization syntax |
| `readonly` | `-a -f -p` |
| `eval` | re-parse & run; counts against `MaxSourceDepth` |
| `source` / `.` | read+run a file; depth-limited |
| `exit` | sets `ExitError{Code}`, unwinds to top |
| `return` | from a function |
| `break` / `continue` | `[N]` |
| `getopts` | OPTSTRING VAR; sets `OPTARG`, `OPTIND` |
| `let` | arithmetic; exit code = (last expr == 0) ? 1 : 0 |
| `read` | `-r`, `-p PROMPT`, `-t TIMEOUT`, `-n N`, `-N N`, `-d DELIM`, `-a ARRAY`, `-s` (silent) |
| `mapfile` / `readarray` | `-t`, `-n N`, `-O ORIGIN`, `-s SKIP` |
| `dirs` / `pushd` / `popd` | directory stack |
| `hash` | PATH lookup cache: `hash -r`, `hash -p PATH NAME`, `hash -d NAME`, `hash NAME` |
| `compgen` / `complete` / `compopt` | completion machinery — stubs that match TS output |
| `trap` | record handlers per signal name. Honored signals: `EXIT`, `ERR`, `DEBUG`, `RETURN`. `INT`/`TERM`/`HUP` recorded but not delivered (no real signals). |
| `:` (colon) | exit 0 |
| `[` (test) | full POSIX test |
| `wait` | wait for background jobs |
| `jobs` / `fg` / `bg` | stub matching bash output format |
| `umask` | virtual — sets `Bash.umask`, used as default mode mask for newly created files |

Implementation strategy: most of these are handled by `mvdan/sh/interp` already. Add **callback hooks** to intercept the ones we need to extend (limits, source depth, etc.). Where `mvdan/sh` lacks a builtin entirely (rare — `compgen`, etc.), register via `interp.BuiltinHandler`.

---

# PHASE 12 — Process Info & Defaults

Port `src/Bash.ts` constructor logic.

- `processInfo` defaults: `pid=1, ppid=0, uid=1000, gid=1000`
- `BASHPID` starts at virtual pid; each subshell increments a counter
- `$$` = virtualPid (never the host PID)
- `/proc/self/status` text built from these values:
```
Name:	bash
Pid:	{pid}
PPid:	{ppid}
Uid:	{uid}	{uid}	{uid}	{uid}
Gid:	{gid}	{gid}	{gid}	{gid}
```
- `whoami` always prints `user`
- `hostname` reads `/etc/hostname` (default `localhost`)

---

# PHASE 13 — Transform API

Port `src/transform/`.

## 13.1 `transform/types.go`
```go
type Plugin interface {
    Name() string
    Transform(ctx Context) Result
}

type Context struct {
    AST      *ast.Script
    Metadata map[string]any
}

type Result struct {
    AST      *ast.Script
    Metadata map[string]any
}

type BashTransformResult struct {
    Script   string
    AST      *ast.Script
    Metadata map[string]any
}
```

## 13.2 `transform/pipeline.go`
```go
type Pipeline struct {
    plugins []Plugin
}

func New() *Pipeline
func (p *Pipeline) Use(plugin Plugin) *Pipeline
func (p *Pipeline) Transform(script string) (*BashTransformResult, error)
```

Process: Parse → apply plugins in order → Serialize → return.

## 13.3 Built-in plugins

### `transform/plugins/collector` — CommandCollectorPlugin
Walks the AST, collects names of every `SimpleCommand`. Stores in metadata as `commands: []string`.

### `transform/plugins/tee` — TeePlugin
Port `src/transform/plugins/tee-plugin.ts` faithfully. Wraps each non-trivial pipeline stage with `| tee /OUT/<idx>-<cmd>.stdout.txt`. Preserves `PIPESTATUS` semantics by saving/restoring via a synthesized restore-pipeline.

```go
type TeeOptions struct {
    OutputDir            string
    TargetCommandMatch   func(string) bool
    Timestamp            time.Time      // default time.Now()
}

type TeeFileInfo struct {
    CommandIndex int
    CommandName  string
    Command      string
    StdoutFile   string
}
```

## 13.4 Integration with `Bash.Exec`
`Bash.RegisterTransformPlugin(p)` appends to a slice. Before running a script:
1. Build the AST
2. Apply each registered plugin in order
3. Serialize back
4. Re-parse (cheaper than running with synthesized AST) and run
5. Attach plugin metadata to `BashExecResult.Metadata` keyed by plugin name

---

# PHASE 14 — Optional Runtime: SQLite

Subpackage `github.com/mark3labs/go-bash/sqlite`. Import this subpackage to enable.

```go
package sqlite

import (
    gobash "github.com/mark3labs/go-bash"
    _ "modernc.org/sqlite"
    "database/sql"
)

// Register adds sqlite3 to the registry.
func Register(b *gobash.Bash, opts Options) error

type Options struct {
    Timeout time.Duration       // default: ResolvedLimits.MaxSqliteTimeout
}
```

Implementation:
- `sqlite3 :memory:` opens an `:memory:` DB
- `sqlite3 file.db "QUERY"` opens the DB file using a temp real-FS path; map VFS file → temp file on disk for the query duration, then write back. (Simpler MVP: only support `:memory:` and reject file-based until v2.)
- Honor `-header`, `-csv`, `-json`, `-line` output modes
- Timeout: run the query in a goroutine; on `ctx.Done()` call `db.Close()` to interrupt

---

# PHASE 15 — Optional Runtime: JavaScript (js-exec)

Subpackage `github.com/mark3labs/go-bash/jsexec`. Backed by `github.com/dop251/goja`.

```go
package jsexec

func Register(b *gobash.Bash, cfg gobash.JavaScriptConfig) error
```

Features to support:
- `js-exec -c CODE` — run inline
- `js-exec FILE` — run file
- `js-exec -m -c CODE` — ESM mode (goja has limited module support; emulate via `require` shim)
- `console.log`, `console.error`, `console.warn`
- `fetch` — wired to `Context.Fetch` if non-nil
- `Buffer`, `URL`, `URLSearchParams` — provide minimal compat layer
- `fs` (subset): `readFileSync`, `writeFileSync`, `readdirSync`, `statSync`, `existsSync`, `mkdirSync`, `rmSync`
- `path`: `join`, `resolve`, `dirname`, `basename`, `extname`, `relative`, `normalize`
- `child_process`: `execSync` and `spawnSync` that route through `Context.Exec`
- `process`: `argv`, `cwd()`, `exit()`, `env`, `platform="linux"`, `version="v18.0.0"`
- `os`, `url`, `assert`, `util`, `events`, `buffer`, `stream`, `string_decoder`, `querystring` — minimal shims
- Tool invocation: when `cfg.InvokeTool != nil`, install a `tools` global proxy that builds a dot path on access and calls `InvokeTool(ctx, path, argsJSON)`. Since goja is single-threaded, no `Atomics.wait` needed — call sync from Go via `runtime.RunOnLoop`.

Timeout: `goja` supports interruption via `runtime.Interrupt(reason)`. Set up a goroutine that calls `Interrupt` after `MaxJsTimeout`.

Also register `node` as an alias of `js-exec` (matches TS).

---

# PHASE 16 — Optional Runtime: Python

Subpackage `github.com/mark3labs/go-bash/pythonexec`. We do **not** embed CPython. We expose a host hook.

```go
package pythonexec

type Runtime interface {
    Run(ctx context.Context, in RunInput) (RunOutput, error)
}

type RunInput struct {
    Script   string             // -c CODE
    File     string             // path within VFS
    Module   string             // -m MODULE
    Args     []string
    Env      map[string]string
    Cwd      string
    Stdin    io.Reader
    Timeout  time.Duration
    FS       fs.FileSystem      // for the runtime to read files from VFS
}

type RunOutput struct {
    Stdout   string
    Stderr   string
    ExitCode int
}

func Register(b *gobash.Bash, rt Runtime) error
```

Provide one canned runtime in `pythonexec/dockerrt` (executes `docker run python:3.13 -c ...`) and one in `pythonexec/exec` (shells out to a real `python3` on disk — caller must opt in explicitly because it punches through the sandbox).

Skipping embedded Python is an intentional, documented divergence from `just-bash`. Note in DECISIONS.md.

---

# PHASE 17 — Sandbox API (Vercel Sandbox Compat)

Port `src/sandbox/`.

```go
package sandbox

type Sandbox struct{ /* wraps *gobash.Bash */ }

type Options struct {
    Cwd          string
    Env          map[string]string
    Timeout      time.Duration
    FS           fs.FileSystem
    OverlayRoot  string                // mutually exclusive with FS
    Network      *network.Config
    // (no DefenseInDepth — Go doesn't need it)
}

func Create(ctx context.Context, opts Options) (*Sandbox, error)

func (s *Sandbox) RunCommand(ctx context.Context, p RunCommandParams) (*Command, error)
func (s *Sandbox) WriteFiles(ctx context.Context, files map[string]string) error
func (s *Sandbox) ReadFile(ctx context.Context, path string) (string, error)
func (s *Sandbox) MkDir(ctx context.Context, path string, opts MkDirOptions) error
func (s *Sandbox) Stop(ctx context.Context) error                  // no-op

type RunCommandParams struct {
    Cmd      string
    Args     []string
    Cwd      string
    Env      map[string]string
    Sudo     bool                 // no-op
    Detached bool
    Stdin    io.Reader
}

type Command struct{ /* live process handle */ }

func (c *Command) Stdout() (string, error)
func (c *Command) Stderr() (string, error)
func (c *Command) Wait() (Finished, error)

type Finished struct{ ExitCode int }
```

Detached commands run in a goroutine; `Wait` blocks. Stdout/Stderr buffered up to `MaxOutputSize`.

---

# PHASE 18 — CLI

## 18.1 `cmd/gobash/main.go`
```
gobash [flags] [script-file]
gobash -c 'script'
echo 'script' | gobash
```

Flags:
- `-c SCRIPT` — execute script string
- `--root PATH` — root directory (default `.`); mounted at `/home/user/project` via OverlayFs
- `--cwd PATH` — working directory in sandbox (default `/home/user/project`)
- `--allow-write` — use ReadWriteFs instead of OverlayFs
- `--json` — emit `{"stdout":..., "stderr":..., "exitCode":...}` to stdout, nothing else
- `-e`, `--errexit` — `set -e`
- `--no-network` — explicit disable (default is also disabled)
- `--network-allow PREFIX` — repeatable; build a `Network.AllowedURLPrefixes`

Exit code of the CLI = exit code of the script.

## 18.2 `cmd/gobash-shell/main.go`
Interactive REPL with line editing via `github.com/peterh/liner` or `github.com/chzyer/readline`. Supports multi-line continuation (matches bash `PS2`). `--no-network` flag. Session env persists across input lines (same `*Bash` instance).

## 18.3 CLI golden tests
`cmd/gobash/testdata/*.txt` — recorded stdout/stderr/exitcode for a curated set of invocations.

---

# PHASE 19 — Comparison Test Harness

## 19.1 Fixture format
Port the JSON fixture format from `src/comparison-tests/`. Each fixture file:
```json
{
  "script": "...",
  "files": {"/path": "content"},
  "env": {"VAR": "val"},
  "cwd": "/home/user",
  "locked": false,
  "expected": {
    "stdout": "...",
    "stderr": "...",
    "exitCode": 0
  }
}
```

## 19.2 Harness — `internal/testutil/comparison.go`
```go
func RunFixture(t *testing.T, fixturePath string)
```
Loads JSON, constructs `*Bash` with fixture's files+env+cwd, runs `Exec(fixture.Script)`, asserts result equals `expected`. If `RECORD_FIXTURES=1` env var set, re-records the fixture from real bash (`exec.Command("bash", "-c", fixture.Script)` — **only** in the recording tool, never in production code).

## 19.3 Record tool
`cmd/record-fixtures/main.go`:
- Walks `internal/testdata/fixtures/**/*.json`
- For each non-locked fixture: runs the script with real bash, updates the `expected` block
- Locked fixtures skipped unless `RECORD_FIXTURES=force`

## 19.4 Bulk import from just-bash
Write a one-off script `scripts/import-fixtures.sh` that copies `just-bash/packages/just-bash/src/comparison-tests/fixtures/` into `internal/testdata/fixtures/` and converts the TS-specific format keys to ours. Run once at the start of Phase 10, commit the result.

---

# PHASE 20 — Documentation

## 20.1 README.md
Mirror `just-bash/packages/just-bash/README.md` 1:1, translating TS examples to Go. Include:
- Quick start
- All four FS modes with code samples
- Network config
- JS / Python / SQLite (opt-in)
- AST transforms
- Sandbox API
- CLI usage
- Execution limits table
- Threat model summary linking to THREAT_MODEL.md

## 20.2 THREAT_MODEL.md
Port `just-bash/THREAT_MODEL.md` and:
- Delete the entire "Code Execution Escape" table (no `eval`/`Function` in Go)
- Delete §3.9 prototype pollution table (n/a)
- Delete §4.1 dynamic import (n/a)
- Add a new section "What Go gives us for free" explaining the architectural advantages

## 20.3 AGENTS.md
Port `packages/just-bash/AGENTS.md`. Tells AI agents how to use go-bash.

## 20.4 godoc
Every exported identifier needs a doc comment starting with the identifier name (`golint` style). Run `pkgsite` locally to verify rendering.

---

# PHASE 21 — Hardening & Polish

## 21.1 Lints
- Custom `internal/lint/no-os-exec`: bans `os/exec` imports outside the `cmd/record-fixtures` tool and optional `pythonexec/exec` runtime
- Custom `internal/lint/no-net-default`: bans `net/http.DefaultClient`, `net.Dial`, etc. outside `network/`
- `staticcheck` with no exclusions
- `gosec` with curated rule set

## 21.2 Fuzz testing
`go test -fuzz` corpora for:
- `parser.Parse` (arbitrary bytes)
- `internal/glob.Match` (random patterns)
- `expansion` (parameter expansions with weird chars)

## 21.3 Race detector
`go test -race ./...` must be clean.

## 21.4 Benchmarks
`bash_bench_test.go` covers:
- `Exec("echo hello")` cold and warm
- 1 MB script parse time
- 1000-iteration loop runtime
- VFS read/write throughput

Target: cold `Exec` < 5ms, parse 1MB < 100ms, vfs read 1MB < 10ms.

---

# Detailed File-by-File Implementation Order

A coding agent should implement in this order. Each numbered item should be a separate commit + PR.

1. `go.mod`, `doc.go`, `errors.go`, `result.go`, `options.go`, `limits.go` (Phase 1, 2)
2. `fs/fs.go`, `fs/base.go`, `fs/path.go` (Phase 3.1–3.2)
3. `fs/memfs/` (Phase 3.3)
4. `fs/realfs/` shared helpers (Phase 3.4)
5. `fs/overlayfs/` (Phase 3.5)
6. `fs/rwfs/` (Phase 3.6)
7. `fs/mountfs/` (Phase 3.7)
8. FS contract test suite (Phase 3.8)
9. `ast/` types (Phase 4.1)
10. `parser/parser.go` + translator (Phase 4.2)
11. `transform/serialize.go` (Phase 4.3)
12. `interp/runner.go` + handlers (Phase 5)
13. `bash.go` `Exec` real implementation (Phase 5.6)
14. FS init layout (Phase 7)
15. `command/` package + registry (Phase 8)
16. `network/` package (Phase 9)
17. **Comparison test harness** (Phase 19) — needed before bulk command work
18. Import all fixtures from just-bash
19. **Wave A** builtins (Phase 10 Wave A)
20. Interpreter built-ins (Phase 11) — cd, export, unset, set, shopt, exit, return, break, continue, source, eval, let, getopts, read, mapfile, declare, local, readonly, dirs/pushd/popd, hash, trap, [, :, wait, jobs, umask, history, alias/unalias, compgen/complete/compopt
21. **Wave B** builtins (file ops)
22. **Wave C** builtins (text processing) — most code-heavy single phase
23. **Wave D** builtins (grep, sed, awk, jq, rg) — sed and awk are the largest individual ports
24. **Wave E** builtins (yq, xan)
25. **Wave F** builtins (gzip, tar)
26. **Wave G** builtins (env, date, timeout, time, bash, sh, help, etc.)
27. **Wave H** builtins (curl, html-to-markdown) — requires network
28. Process info & defaults polish (Phase 12)
29. Transform API (Phase 13)
30. SQLite optional (Phase 14)
31. JavaScript optional (Phase 15)
32. Python hook (Phase 16)
33. Sandbox API (Phase 17)
34. CLI (Phase 18)
35. Documentation (Phase 20)
36. Hardening (Phase 21)

---

# Concrete Quality Bars

- **Comparison test pass rate**: ≥ 98% of fixtures pass on Linux/amd64. Failures must be tagged with reason in `internal/testdata/fixtures/SKIP.json`.
- **Spec test pass rate**: ≥ 80% of `spec-tests` pass; rest skipped with TODO.
- **Public API stability**: after Phase 5 ships, `bash.go`, `options.go`, `result.go` are frozen. Internal changes ok; signatures aren't.
- **Cgo-free**: `go build` with `CGO_ENABLED=0` must work for the main package and `jsexec`. `sqlite` (via `modernc.org/sqlite`) is also cgo-free. `pythonexec/exec` is the only cgo-allowed (and even there, only if user opts in).
- **Zero `os/exec` in the runtime**: lint enforced.
- **No global state**: every test creates its own `*Bash`.

---

# Resolved Design Decisions (Mirror just-bash Exactly)

All six previously-open questions are resolved by **mimicking `just-bash` exactly**. Source references below are to `vercel-labs/just-bash` at the version vendored at start of project.

1. **Function persistence across Exec calls — RESET BETWEEN CALLS.**
   `src/Bash.ts:639` snapshots functions per-exec: `functions: new Map(this.state.functions)`. The persistent `Bash.state.functions` is only ever written by the constructor. Function definitions inside a script live for that `Exec` only.
   - Go impl: `Bash` holds `baseFuncs map[string]*ast.FunctionDef` (set at `New`; immutable). Each `Exec` deep-copies it into the runner's function table. Mutations during exec are discarded.

2. **Background job semantics — VIRTUAL PIDs ONLY, `wait` IS A NO-OP.**
   `src/interpreter/types.ts` defines `lastBackgroundPid` and `nextVirtualPid`. `src/interpreter/builtin-dispatch.ts:234` comments `wait — no-op in this context`. `&` increments the counter and continues synchronously. `$!` returns the last counter value.
   - Go impl: `Bash.state` has `lastBackgroundPID int` and `nextVirtualPID int` (init to `ProcessInfo.PID+1`). `&` runs the command synchronously, then sets `lastBackgroundPID = atomic.AddInt32(&nextVirtualPID, 1) - 1`. `wait` is a no-op returning 0. Document this in godoc on `bash.Exec`.

3. **`$RANDOM` determinism — NON-DETERMINISTIC.**
   `src/interpreter/expansion/variable.ts:187`: `String(Math.floor(Math.random() * 32768))`. No seeding knob.
   - Go impl: `math/rand/v2.IntN(32768)`. Do **not** expose a seed in `BashOptions`. Tests that need determinism construct a custom command or override via env.

4. **AWK — PORT just-bash's AWK; do NOT use `goawk`.**
   `src/commands/awk/` is ~6,265 LOC of hand-written lexer (`lexer.ts`), parser (`parser2.ts`), AST (`ast.ts`), interpreter (`interpreter/`), and builtins (`builtins.ts`). Semantics differ from goawk in user-function bodies, getline, printf `%c`, and the `nextfile` builtin. Feature-for-feature parity requires porting it.
   - Go impl: subpackage `builtins/awk/internal/` containing direct ports of `lexer.go`, `parser.go`, `ast.go`, `interp.go`, `builtins.go`. Update the dependency table in §0.2 to remove `benhoyt/goawk` — it is **not** used.
   - Enforce `MaxAwkIterations` by incrementing a counter on every loop iteration (`for`, `while`, `do-while`) in the AWK interpreter and returning an `ExecutionLimitError`.

5. **Globstar — PORT just-bash's custom matcher; do NOT use `mvdan/sh/expand`'s globber.**
   `src/shell/glob.ts` is hand-written with `MAX_GLOBSTAR_SEGMENTS = 5` and honors `dotglob`/`nullglob`/`failglob`/`globskipdots` (default true on bash ≥5.2)/`nocaseglob` exactly. `src/shell/glob-to-regex.ts` is the regex compiler.
   - Go impl: `internal/glob/glob.go` ports `shell/glob.ts` line-for-line; `internal/glob/toregex.go` ports `shell/glob-to-regex.ts`. Use Go `regexp` (RE2) as the backend. Hook into the `mvdan/sh/interp` runner via a custom `interp.GlobFunc` so our matcher replaces the default. Honor all five shopt flags. Reject any pattern with more than `MAX_GLOBSTAR_SEGMENTS` `**` segments.

6. **Timezone — UTC DEFAULT, `$TZ` OPT-IN, INVALID TZ FALLS BACK TO UTC.**
   `src/commands/date/date.ts:155-163` spells out the exact contract:
   - `-u` → always UTC
   - `$TZ` unset → UTC (the sandbox non-disclosure default; host TZ never leaks)
   - `$TZ=<valid IANA zone>` → that zone
   - `$TZ=<invalid zone>` → UTC fallback (matches GNU date silent-fallback behavior)
   - Go impl: blank-import `_ "time/tzdata"` in `bash.go` so the zoneinfo DB is embedded (works under `CGO_ENABLED=0`). Helper `resolveTZ(env) *time.Location`:
     ```go
     func resolveTZ(env map[string]string) *time.Location {
         tz, ok := env["TZ"]
         if !ok || tz == "" { return time.UTC }
         loc, err := time.LoadLocation(tz)
         if err != nil { return time.UTC }
         return loc
     }
     ```
     Use this in `date`, `stat`, `ls -l`, and any other command that formats wall-clock time.

## Other Decisions Locked at Project Start

- **`mvdan/sh` is the bash engine, but we override the globber and may override more.** Globber, exec handler, open/stat/readdir handlers are always replaced. Other defaults are kept until a comparison-fixture failure forces a divergence.
- **`exec.Command` is forbidden in the runtime.** Only `cmd/record-fixtures` may import `os/exec` (gated by a custom lint).
- **Encoding is bytes-and-strings, full stop.** No `ByteString` port. Stdin/stdout pipelines are `[]byte`. UTF-8 decoding happens explicitly at command-implementation sites that need text.
- **No defense-in-depth subsystem.** Threat model is enforced architecturally (no exec, no env leakage, virtual FS, allow-listed network).
- **AWK and globber ports above add ~7-9 KLOC of dedicated work** that the spec previously allocated to libraries. Re-estimate Wave D effort accordingly (sed ~4k LOC, awk ~6k LOC port, plus jq via gojq, plus rg).

---

# Smoke Test (Phase 1 Completion Gate)

Once Phase 1–5 are done, running:

```go
b, _ := gobash.New(gobash.BashOptions{})
res, _ := b.Exec(context.Background(), `
    echo "Hello" > greeting.txt
    cat greeting.txt
`, gobash.ExecOptions{})
fmt.Println(res.Stdout)    // "Hello\n"
fmt.Println(res.ExitCode)  // 0
```

…must work. That single test is the contract that everything else builds on.
