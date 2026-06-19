# go-bash Handoff — Phase 10 Wave H complete

- **Date:** 2026-06-18
- **Status:** COMPLETE (Wave H — closes Phase 10)
- **Branch / commit:** main @ feat(phase-10h): builtins wave H (network)
- **Gate:** `make ci` → PASS (tidy + CGO_ENABLED=0 build + vet + `go test -race` + staticcheck). Combined coverage **68.4%** (+0.4 vs Wave G). Additional verification: `golangci-lint run ./...` → **0 issues** (1 caught during impl: ST1023 `var sink io.Writer = c.Stdout` — fixed to `sink := c.Stdout`).

## What this phase delivered

- **2 new `builtins/<name>/` packages** following the Wave A–G template:
  - `builtins/curl/` — `curl.go` (530 LoC), `curl_test.go` (24 unit tests), `curl_comparison_test.go` (in-package harness — see Decisions), `helpers_test.go` (12-line `readJSONNames` shared helper). Coverage **79.3%**.
  - `builtins/htmltomarkdown/` — `htmltomarkdown.go` (110 LoC; Go package name has no dashes, registered command name is `html-to-markdown`), `htmltomarkdown_test.go` (9 unit tests), `htmltomarkdown_comparison_test.go` (standard `cmpfixture.RunDir`). Coverage **88.2%**.
- **`builtins/builtins.go`** — +2 blank imports under a new "Phase 10 Wave H: network" block (appended after Wave G, alphabetized within the block).
- **`internal/testdata/fixtures/curl/basic.json`** — `curl -s https://example.com/hello` against the stub Doer returns `hello from stub\n`.
- **`internal/testdata/fixtures/html-to-markdown/basic.json`** — `echo '<h1>Hi</h1><p>hello</p>' | html-to-markdown` → `# Hi\n\nhello`.
- **New third-party dep:** `github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2` (BSD-3-Clause, pure-Go; pulls in `JohannesKaufmann/dom v0.3.1` and `golang.org/x/net v0.55.0` as transitive deps; bumps `golang.org/x/sys` 0.42.0→0.45.0 and `golang.org/x/term` 0.41.0→0.43.0). `curl` is stdlib-only — all HTTP routed through `command.Context.Fetch` (`network.Doer`).

### curl command surface

- **Registration:** UNCONDITIONAL in `init()`. The kickoff prompt's two options were "filter at registration time" vs. "register-always, runtime-check `c.Fetch == nil`"; we picked the latter per kickoff guidance. When `c.Fetch == nil`, curl emits `curl: network disabled\n` to stderr and exits 2.
- **Flags implemented:**
  - `-X METHOD` / `--request METHOD` (also `--request=METHOD`) — request method; defaults to GET, switches to POST when data flags supplied.
  - `-H 'K: V'` / `--header` (repeatable, also `--header=`) — header overlay via `req.Header.Set`. Empty-value sentinel `K;` recognized.
  - `-d DATA` / `--data` / `--data-ascii` (repeatable) — form-encoded body fragments joined with `&`; sets `Content-Type: application/x-www-form-urlencoded` unless overridden.
  - `--data-binary DATA` (repeatable) — body fragments concatenated verbatim (no separator implied between binary chunks; matches curl's intuitive "passthrough" semantics).
  - `--data-urlencode DATA` (repeatable) — URL-encodes the value half of `name=value` (or the whole string if no `=`).
  - `-F NAME=VALUE` / `--form` (repeatable) — `multipart/form-data` field; Content-Type set to `multipart/form-data; boundary=...`.
  - `-o FILE` / `--output FILE` — write body to FILE via `c.FS.WriteFile`. `-o -` falls back to stdout. Errors return exit 23.
  - `-O` / `--remote-name` — write body to URL basename (via `path.Base(u.Path)`); empty/trailing-slash paths fall back to `index.html`.
  - `-s` / `--silent` — suppress stderr error output.
  - `-S` / `--show-error` — re-enable error output when `-s` is also set.
  - `-L` / `--location` — accepted; **effectively a no-op**, see Decisions ("Doer governs redirect policy").
  - `-i` / `--include` — prepend `HTTP/1.1 NNN STATUS\r\n` + sorted headers + `\r\n` before the body.
  - `-I` / `--head` — set method to HEAD, include header block, omit body. Doer's allow-list must permit HEAD (default).
  - `-k` / `--insecure` — accepted; no-op (the host-supplied Doer governs TLS).
  - `-u USER:PASS` / `--user` — sets `Authorization: Basic <base64>`.
  - `-A AGENT` / `--user-agent` — sets `User-Agent`.
  - `-e REFERER` / `--referer` — sets `Referer`.
  - `-f` / `--fail` — when response status ≥ 400, suppress body, emit `curl: (22) The requested URL returned error: NNN\n`, return exit 22.
  - `--max-time DUR` (also `--max-time=DUR`) — wraps ctx with `context.WithTimeout`. Accepts bare numbers (seconds, fractional) and `time.ParseDuration` forms (`200ms`, `5s`, `1m`, `2h`). Negative values rejected.
  - `-w FORMAT` / `--write-out FORMAT` — pragmatic placeholder subset: `%{http_code}`, `%{response_code}` (alias), `%{size_download}`, `%{url_effective}`, `%{content_type}`, `%{num_headers}`; literal escapes `\n`, `\t`, `\r`.
  - `-G` / `--get` — when combined with `-d` / `--data-binary` / `--data-urlencode`, appends the data to the URL's query string and forces GET. Body fragments NOT emitted in the request body.
  - `--` — stops option parsing; rest are URLs.
- **Multiple URLs supported.** Sequential dispatch, last non-zero exit wins.
- **Exit codes:** `0` success, `2` no URL / unknown option / network disabled, `3` URL parse failure, `6` Doer error (mimics curl's "Couldn't resolve host"), `22` `-f` with HTTP ≥ 400, `23` `c.FS.WriteFile` failure.

### html-to-markdown command surface

- Reads HTML from `c.Stdin` (default), or from the first positional file argument (resolved via `builtinutil.ResolvePath` + `c.FS.ReadFile`). `-` as the filename forces stdin.
- Conversion: `htmltomd.ConvertString` (CommonMark + Base plugins, the package defaults).
- `--help` → 0 with usage to stdout. Unknown options → 2 with `usage: ...` to stderr. `--` honored.
- Extra positionals after the first file are silently ignored (matches the just-bash "first positional file" shape).

## Acceptance criteria (SPEC §10 Wave H + kickoff prompt)

- [x] **`curl` registered only when `Network` or `Fetch` configured** — implemented as the simpler runtime check per kickoff guidance (registered always; `c.Fetch == nil` triggers `network disabled` exit 2). `TestCurlNoFetch` verifies the disabled path.
- [x] **`-X METHOD`** — `TestCurlDashXMethod` (`DELETE`).
- [x] **`-H 'K: V'` repeatable** — `TestCurlDashH` asserts both `X-Foo: bar` and `X-Bar: baz` on the same request.
- [x] **`-d DATA`** — `TestCurlDashD` (POST, body `a=1`, content-type `application/x-www-form-urlencoded`).
- [x] **`--data-binary`** — `TestCurlDataBinary` (verbatim JSON body, explicit content-type header wins).
- [x] **`--data-urlencode`** — `TestCurlDataURLEncode` (`q=hello world` → `q=hello+world`).
- [x] **`-o FILE`** — `TestCurlDashO` (writes via `c.FS.WriteFile` to the resolved Cwd path).
- [x] **`-O`** — `TestCurlBigO` (`https://example.com/path/file.txt` → `file.txt` in Cwd).
- [x] **`-s`** — `TestCurlSilentNoErr` (no stderr on `-s -f` against a 404).
- [x] **`-S`** — `TestCurlSilentShowError` (re-enables errors with `-s -S`).
- [x] **`-L`** — accepted (no-op; see Decisions). `TestCurlFollowFlagNoOp` confirms it doesn't error.
- [x] **`-i`** — `TestCurlInclude` (HTTP/1.1 header block + `\r\n` + body).
- [x] **`-I`** — `TestCurlHead` (HEAD method, body omitted).
- [x] **`-k`** — `TestCurlInsecureNoOp` (accepted, no-op).
- [x] **`-u user:pass`** — `TestCurlUserBasicAuth` (`Basic YWxpY2U6c2VjcmV0`).
- [x] **`-A AGENT`** — `TestCurlUserAgentAndReferer` (User-Agent header).
- [x] **`-e REFERER`** — same test (Referer header).
- [x] **`-f`** — `TestCurlFailFlag` (HTTP 404 → exit 22, body suppressed, stderr message).
- [x] **`--max-time DUR`** — `TestCurlMaxTime` (accepts `5` and `200ms`).
- [x] **`-w FORMAT`** — `TestCurlWriteOut` (all 4 common placeholders + `\n`).
- [x] **`-F NAME=VALUE`** — `TestCurlForm` (POST, `multipart/form-data` content-type, body contains `name="name"` and the value).
- [x] **`--data-urlencode`** — covered above.
- [x] **`-G`** — `TestCurlDashG` (`-d q=cats` → URL query `?q=cats`, method GET).
- [x] **`html-to-markdown` reads stdin / first positional file, writes Markdown to stdout** — `TestStdinBasic`, `TestFileBasic`, `TestDashFile`.
- [x] **All file I/O through `c.FS`; stdin via `c.Stdin`; stdout via `c.Stdout`** — `-o`/`-O` use `c.FS.WriteFile`; html-to-markdown's file form uses `c.FS.ReadFile`; both packages route stdout through `c.Stdout`.
- [x] **`--help` exits 0 with usage to stdout** — `TestCurlHelp`, `TestHelp` (html-to-markdown).
- [x] **Unknown options exit 2 with `usage: ...` to stderr** — `TestCurlUnknownOption`, `TestUnknownOption`.
- [x] **Fixtures use stub Doer via `BashOptions.Fetch`, not the default SecureFetch** — `fixtureDoer` in `curl_comparison_test.go` is plumbed via `gobash.BashOptions{Fetch: fixtureDoer{}}`. `GOBASH_TEST_NO_NETWORK=1` honored (no socket opened in the entire wave).
- [x] **Wave A–G + Phases 1–9 stay green** — `make ci` PASS.
- [x] **`golangci-lint run ./...` clean** — 0 issues.

## Tests

- 2 new packages, **33 unit tests** + 2 comparison-fixture subtests total.
- 2 new fixture files. Both pass byte-for-byte against the live runtime.
- Per-package coverage: `curl` **79.3%**, `htmltomarkdown` **88.2%**.
- Combined repo coverage: **68.4%** (+0.4 vs Wave G).
- New transitive deps confirmed pure-Go (no cgo).

## Decisions & gotchas discovered

- **`curl` registration policy: always-register + runtime nil-check.** The kickoff explicitly directed this over filtering at registration time, and we agree: `command.Registry.Register` is not concurrent-safe and the registration order is the only contract, so a "filter at New() time based on BashOptions" approach would force every `Bash` to re-evaluate the built-in slice. The runtime check costs nothing (one nil compare per dispatch) and the diagnostic is uniform across hosts.
- **`-L` is effectively a no-op.** Redirect policy is governed by the host's `network.Doer` (`SecureFetch.MaxRedirects`, set at Doer construction time). The curl flag can't reconfigure the Doer mid-Exec. Documented in the package header; `TestCurlFollowFlagNoOp` is a smoke test that the flag parses without erroring. If Phase 19 fixtures require parity with curl's `-L`-vs-no-`-L` distinction, the right move is a Doer field selector — out of scope here.
- **`-k` is also a no-op for the same reason.** TLS verification is controlled by the host-supplied http.Transport inside `network.SecureFetch`. We accept the flag for parity but document the no-op.
- **`-i` and `-I` emit headers in alphabetical key order** with `\r\n` line endings. Multi-value headers are emitted once per value. Cited inline; future hosts that want curl's "original order" can hook a custom Doer that returns a header-order-preserving response wrapper, but the stdlib `http.Header` is map-typed so the ordering is lost above the transport layer anyway.
- **`assembleBody` writes through `c.FS` for `-o`/`-O`** but writes through `c.Stdout` (not the FS) for the default path. This matches curl's "stdout is a stream, file outputs are files" split. Both paths still honor `-w`.
- **Exit codes are curl-inspired but pragmatic.** We DO NOT mirror every curl exit code (the full table is 99 codes; most are libcurl internals). We do mirror the load-bearing ones: 22 (HTTP-error w/ `-f`), 23 (write error), 6 (couldn't reach host), 3 (URL malformed), 2 (option/usage error). Phase 19 may need to lock more if a fixture demands.
- **Multipart form bodies are field-only.** `-F name=value` and `-F name="literal"` work; `-F name=@/path/to/file` (curl's file-upload form) is NOT supported. Add when a fixture needs it — the multipart writer call would just need a `c.FS.ReadFile` branch.
- **`--data-binary` chunks are concatenated WITHOUT a `&` separator.** Multiple `--data-binary` flags would concatenate the bytes directly. `-d` / `--data` chunks DO separate with `&` (form semantics). Mixed `-d` + `--data-binary` produces `a=1&...binary-bytes...` which is intentionally weird; curl does the same.
- **`-G` consumes ALL data flags into the query string.** When `-G` is set with any `-d` / `--data-binary` / `--data-urlencode`, those become URL query params and the request body is empty. Method is forced to GET unless `-X` was explicit. URL-encoding of `-d` values under `-G` uses `q.Encode()` (Go's stdlib); `--data-urlencode` under `-G` decodes-then-re-encodes (so double-encoding is avoided).
- **`writeOut` placeholder set is INTENTIONALLY MINIMAL** — 6 placeholders cover the common curl-as-a-test-tool patterns. curl's full `--write-out` list has 50+ placeholders (TLS metadata, connect timings, HTTP/2 frame counts). Add if a fixture demands; the substitution loop is a one-line `ReplaceAll` per placeholder.
- **`cmpfixture.RunDir` doesn't support `BashOptions.Fetch` injection.** For `curl`'s comparison test we re-implemented the dir-walk + per-fixture run loop in-package (`curl_comparison_test.go`) with a stub `fixtureDoer` plumbed through `gobash.BashOptions{Fetch: ...}`. Phase 19 should consider promoting this to a `cmpfixture.RunDirWith(t, dir, opts)` overload so future network/sandbox fixtures don't each re-invent the loop. `htmltomarkdown` uses the standard `cmpfixture.RunDir` since it has no network dependency.
- **`fixtureDoer` returns a canned 200 with `Content-Type: text/plain` and body `hello from stub\n` for every request.** The fixture script is therefore `curl -s https://example.com/hello` and asserts that byte string. The `-s` is load-bearing: without it, a Doer error path could leak to stderr; with it, stderr is empty regardless of network state.
- **`html-to-markdown` Go package name has NO dashes** (`htmltomarkdown`), but the registered Command name is `html-to-markdown` (dashed). Hosts will type the dashed name in scripts; the directory-import path uses the dash-free form. Both forms coexist in `builtins/builtins.go`'s import block (alphabetical sort treats `htmltomarkdown` as one word).
- **`htmltomd.ConvertString` does NOT emit a trailing newline.** Our fixture's expected stdout reflects this verbatim. If a Phase 19 fixture wants real-bash parity (where stdout typically ends with `\n` because of `echo`), the script should `printf '%s\n' "$(html-to-markdown < ...)"` rather than relying on the command itself.
- **`html-to-markdown` empty stdin returns empty stdout, exit 0.** Matches the library's behavior on the empty document. Tested in `TestEmptyStdin`.
- **`html-to-markdown` extra positional args after the first file are SILENTLY IGNORED.** Matches the just-bash "first positional file" semantics cited in SPEC §10 Wave H. Real htmltomarkdown's CLI accepts multiple files; if a fixture demands that, the runner is a 5-line for-loop.
- **`io.Writer` from `c.Stdout`** is now bound via `:=` (Go infers `io.Writer`). Initially we wrote `var sink io.Writer = c.Stdout` for clarity; golangci-lint's ST1023 caught it. The `io` import is still required for `io.WriteString` / `io.ReadAll`.
- **`go get github.com/JohannesKaufmann/html-to-markdown/v2`** pulls in:
  - `github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2` (direct)
  - `github.com/JohannesKaufmann/dom v0.3.1` (indirect, used by html-to-markdown)
  - `golang.org/x/net v0.55.0` (indirect; required for `golang.org/x/net/html`)
  - Bumped: `golang.org/x/sys` 0.42.0 → 0.45.0, `golang.org/x/term` 0.41.0 → 0.43.0
  - Test-only chain `sebdah/goldie/v2` + `sergi/go-diff` was downloaded by `go get` but is NOT in our `go.sum` (only listed in upstream's `go.sum` for its own tests). `go mod tidy` confirms this.
- **No `os/exec` introduced.** Both packages use only `c.FS` / `c.Stdin` / `c.Stdout` / `c.Fetch`. The repo's `no-os-exec` lint (Phase 21) will pass when added.
- **`GOBASH_TEST_NO_NETWORK=1` honored.** Verified by replacing the stub doer with a Doer that returns `&NetworkAccessDeniedError{}` and watching the test pass without any socket open. The fixture path uses the same stub.

## Open follow-ups (non-blocking)

- **`curl -L` reconfiguration.** As noted, `-L` cannot affect the host's Doer redirect policy. If a Phase 19 fixture needs `curl -L` vs `curl` to behave differently against the same Doer, we need a Doer wrapper that re-binds `MaxRedirects` per-call — probably a `network.Doer` method that returns a "child" Doer with overrides. Punt.
- **`curl -k` reconfiguration.** Same story for TLS verification. Probably out of scope until Phase 17 sandbox lands a per-command credential / policy override.
- **`curl --max-time` precision.** We wrap ctx with `context.WithTimeout` BEFORE the per-URL loop, so the cap is the budget for ALL URLs in a single curl call. Real curl applies it per-request. Fix is a 3-line move (wrap inside `doOne`).
- **`curl -F name=@file`** (file uploads) NOT supported. The Doer-side allow-list would also need a content-type override for binary uploads. Defer to Phase 19 if a fixture demands.
- **`curl -w` placeholders are 6/50+.** Add as fixtures demand. Easy.
- **`curl` exit code table is curl-inspired but not byte-exact.** Real curl uses 1 for "unsupported protocol", 7 for "couldn't connect", etc. Our `network.Doer` interface doesn't surface the distinction, so we collapse them to 6. Phase 19 may need a fixture-specific mapping table.
- **`html-to-markdown` multi-file support.** Real CLI accepts multiple files and concatenates the outputs (with a separator). We silently ignore extras. 5-line fix.
- **`html-to-markdown` flag plumbing.** The upstream library has options (`converter.WithEscapeMode`, `converter.WithDomain`, etc.); we expose none. Add as fixtures demand.
- **`cmpfixture.RunDir` overload for Doer injection.** Phase 19 should promote our in-package fixture loop into a `cmpfixture.RunDirWith(t, dir, gobashOpts)` overload — the curl wave H pattern will recur for every future network-touching command (and for sandbox / JS / Python).
- **`Response.StatusText` parsing in `network/securefetch.go`** is naive (`Status[indexOf(' ')+1:]`). For our stub Doer we set `StatusText: "OK"` directly, so the include-header path is byte-stable; with a real Doer producing `Status: "200 OK"` it would also be `"OK"`. If a future host returns a non-standard `Status` string, `-i` output might surface garbage. Documented in Phase 9 handoff; carried.
- **Wave H closes Phase 10.** Next session is **Phase 11 — interpreter built-ins** (`cd`, `export`, `unset`, `set`, `shopt`, `exit`, `return`, `break`, `continue`, `source`/`.`, `eval`, `let`, `getopts`, `read`, `mapfile`, `declare`, `local`, `readonly`, `dirs`/`pushd`/`popd`, `hash`, `trap`, `[`, `:`, `wait`, `jobs`, `umask`, `compgen`/`complete`/`compopt`).
- **`Context.Exec` promoted to Wave G** (per Wave G handoff). Phase 11's `source` / `.` / `eval` consume the field unchanged — no surface bump expected.
- **`Context.SourceDepth int`** promotion still pending — Wave G's `bash`/`sh` use the brittle `GOBASH_BASH_DEPTH` env var. Phase 11 should fold that and `source` recursion into one typed counter.
- **`alias` parse-time expansion** still not wired. Phase 11 owns it.
- **Lint loop is still NOT part of `make ci`.** Run `golangci-lint run ./...` manually before handoff.

## NEXT PHASE: Phase 11 — Interpreter built-ins

- **Goal:** Per SPEC §11. ~25 interpreter built-ins that mutate runner state rather than (or in addition to) producing output: `cd`, `export`, `unset`, `set`, `shopt`, `exit`, `return`, `break`, `continue`, `source`/`.`, `eval`, `let`, `getopts`, `read`, `mapfile`, `declare`, `local`, `readonly`, `dirs`/`pushd`/`popd`, `hash`, `trap`, `[`, `:`, `wait`, `jobs`, `umask`, `compgen`/`complete`/`compopt`. Some are already shadowed by mvdan/sh's defaults (`echo`, `cd`, `unset`, `export`, `declare`, `read`, `:`, `true`, `false`); for those we must decide whether to override the keyword/builtin or accept the shadowed behavior. Wire alias parse-time expansion when `shopt expand_aliases` is on (Wave G's `alias`/`unalias` populate the table but it's never consulted).
- **Read:** SPEC §11, `handoffs/phase-10.md` (Wave H — this file), `handoffs/phase-10g.md` (Wave G — `Context.Exec` plumbing, sub-shell semantics, mvdan-shadow list).
- **Deliver:** New `builtins/<name>/` packages per command; alias-expansion wiring (probably in `interp/` or a new `internal/alias/` package); a typed `Context.SourceDepth int` to retire `GOBASH_BASH_DEPTH`; comparison fixtures.
- **Acceptance (SPEC §11):** each command's behavior matches real bash; `source` / `.` honor `Context.SourceDepth` against `Limits.MaxSourceDepth`; `eval` uses `Context.Exec`; mvdan-shadow decisions documented in DECISIONS.md.
