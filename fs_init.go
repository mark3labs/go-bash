package gobash

import (
	"fmt"
	"strings"

	"github.com/mark3labs/go-bash/command"
	gbfs "github.com/mark3labs/go-bash/fs"
)

// SPEC.md §7 — Filesystem Init & Default Layout.
//
// When New(BashOptions{}) is called with no Cwd and no Files, gobash
// populates the in-memory FS with the canonical layout below, plus a
// pair of templated files under /etc and /proc, plus one no-op
// executable stub per registered command under /bin. The stub list is
// derived from the command registry (Phase 8). Until that registry
// lands, defaultBinStubs returns nil and the /bin directory stays
// empty.
//
// $HOME and $PATH are seeded into the initial env (only when the
// caller didn't already supply them) by New itself. Keeping that step
// out of applyDefaultLayout keeps the FS helper pure: anyone in
// later phases who wants to rebuild the layout on a different FS
// (e.g. a sandbox subshell with its own VFS) can call it without
// touching the env map.
//
// Cited surface: SPEC §7. Layout list, /etc/hostname content, and
// /proc/self/status template match the just-bash TS port one-to-one
// (Reference: vercel-labs/just-bash, src/fs/init.ts).

// defaultLayoutDirs is the canonical directory list (SPEC §7, in
// creation order). MkdirAll is idempotent on every FileSystem
// implementation in fs/, so re-entry is safe.
var defaultLayoutDirs = []string{
	"/",
	"/home",
	"/home/user",
	"/bin",
	"/usr",
	"/usr/bin",
	"/tmp",
	"/etc",
	"/dev",
	"/proc",
	"/proc/self",
}

// defaultHostname is the literal /etc/hostname content per SPEC §7.
// The trailing newline matches real bash's hostname(1) expectations.
const defaultHostname = "localhost\n"

// procSelfStatusTemplate is the /proc/self/status text from SPEC §11
// (referenced by §7). Tab-separated, ProcessInfo-interpolated. The
// four Uid/Gid columns mirror Linux's real /proc/self/status format
// (real, effective, saved-set, filesystem); gobash uses ProcessInfo's
// single UID/GID for all four since the virtualized identity model
// has no concept of set-uid transitions.
const procSelfStatusTemplate = "Name:\tbash\n" +
	"Pid:\t%d\n" +
	"PPid:\t%d\n" +
	"Uid:\t%d\t%d\t%d\t%d\n" +
	"Gid:\t%d\t%d\t%d\t%d\n"

// defaultBinStubs returns the list of command names that should be
// materialized as no-op executables under /bin. The list is derived
// from the supplied registry (registry.Names() is already sorted, so
// the resulting /bin contents are reproducible across runs).
//
// Each returned name N produces a file at /bin/N with mode 0o755 and
// the binStubBody sentinel content. The bytes are never executed by
// gobash — the runtime dispatch resolves /bin/N invocations by
// basename through the command registry (see interp.lookupCommand).
// The file existence is purely so that which(1), command -v, [ -x ],
// and absolute-path-arg-matching see the binary.
func defaultBinStubs(reg *command.Registry) []string {
	if reg == nil {
		return nil
	}
	names := reg.Names()
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, string(n))
	}
	return out
}

// binStubBody is the no-op sentinel written into each /bin/N stub.
// Real bash would shebang into the host interpreter; gobash never
// reads the bytes itself, so a comment-only payload is sufficient
// and safe even if the host accidentally tries to exec it.
const binStubBody = "#!/bin/sh\n# gobash builtin stub — handled by registry dispatch\nexit 0\n"

// applyDefaultLayout writes the SPEC §7 default layout into target,
// using info to template /proc/self/status and reg to derive the
// /bin/X stub set. Returns the first FS error encountered, if any.
// Callers may choose to ignore the returned error (matching the
// "best effort" nature of the layout when the supplied FS is
// read-only); New does exactly that today.
func applyDefaultLayout(target gbfs.FileSystem, info ProcessInfo, reg *command.Registry) error {
	for _, d := range defaultLayoutDirs {
		if err := target.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := target.WriteFile("/etc/hostname", []byte(defaultHostname), 0o644); err != nil {
		return err
	}
	status := renderProcSelfStatus(info)
	if err := target.WriteFile("/proc/self/status", []byte(status), 0o644); err != nil {
		return err
	}
	for _, name := range defaultBinStubs(reg) {
		// Skip names that aren't valid file basenames (`.` and `..`
		// in particular — Phase 11 registers `.` as a builtin name).
		// Other names (including `:`, `[`) are valid filenames on
		// every memfs/realfs backend we ship.
		if name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
			continue
		}
		if err := target.WriteFile("/bin/"+name, []byte(binStubBody), 0o755); err != nil {
			// Best-effort: a single bad name shouldn't sink the rest
			// of the layout. Phase 11 introduces `:` and `[` which
			// some FS impls may reject; don't propagate.
			continue
		}
	}
	return nil
}

// renderProcSelfStatus formats the /proc/self/status template against
// the supplied ProcessInfo. Exposed (package-private) so tests can
// pin the exact byte sequence without re-implementing the template.
func renderProcSelfStatus(p ProcessInfo) string {
	return fmt.Sprintf(procSelfStatusTemplate,
		p.PID,
		p.PPID,
		p.UID, p.UID, p.UID, p.UID,
		p.GID, p.GID, p.GID, p.GID,
	)
}
