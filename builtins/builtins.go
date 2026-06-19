// Package builtins is the meta-package that side-effect-imports every
// Phase 10 built-in. Each command's init() calls
// command.RegisterBuiltin, populating the package-level slice that
// gobash.New consumes when constructing a *Bash.
//
// Hosts that want the full default registry should not import this
// package directly — the gobash root package blank-imports it via
// builtins_init.go. Hosts that want a CUSTOM subset (e.g. only `echo`
// and `printf`) should set BashOptions.Commands to filter.
//
// Hosts that want NO built-ins at all must construct a *Bash and
// then drop entries from Bash.Registry() manually; the blank import
// here is not conditional.
//
// Cited surface: SPEC §8.3 ("Lazy loading" — Go has no dynamic
// import, so we replace it with side-effect registration).
package builtins

import (
	// Phase 10 Wave A: trivial commands.
	_ "github.com/mark3labs/go-bash/builtins/basename"
	_ "github.com/mark3labs/go-bash/builtins/clear"
	_ "github.com/mark3labs/go-bash/builtins/dirname"
	_ "github.com/mark3labs/go-bash/builtins/echo"
	_ "github.com/mark3labs/go-bash/builtins/expr"
	_ "github.com/mark3labs/go-bash/builtins/falsecmd"
	_ "github.com/mark3labs/go-bash/builtins/hostname"
	_ "github.com/mark3labs/go-bash/builtins/printf"
	_ "github.com/mark3labs/go-bash/builtins/pwd"
	_ "github.com/mark3labs/go-bash/builtins/seq"
	_ "github.com/mark3labs/go-bash/builtins/sleep"
	_ "github.com/mark3labs/go-bash/builtins/truecmd"
	_ "github.com/mark3labs/go-bash/builtins/which"
	_ "github.com/mark3labs/go-bash/builtins/whoami"

	// Phase 10 Wave B: file operations.
	_ "github.com/mark3labs/go-bash/builtins/chmod"
	_ "github.com/mark3labs/go-bash/builtins/cp"
	_ "github.com/mark3labs/go-bash/builtins/du"
	_ "github.com/mark3labs/go-bash/builtins/file"
	_ "github.com/mark3labs/go-bash/builtins/ln"
	_ "github.com/mark3labs/go-bash/builtins/ls"
	_ "github.com/mark3labs/go-bash/builtins/mkdir"
	_ "github.com/mark3labs/go-bash/builtins/mv"
	_ "github.com/mark3labs/go-bash/builtins/readlink"
	_ "github.com/mark3labs/go-bash/builtins/rm"
	_ "github.com/mark3labs/go-bash/builtins/rmdir"
	_ "github.com/mark3labs/go-bash/builtins/split"
	_ "github.com/mark3labs/go-bash/builtins/stat"
	_ "github.com/mark3labs/go-bash/builtins/touch"
	_ "github.com/mark3labs/go-bash/builtins/tree"

	// Phase 10 Wave C: text processing.
	_ "github.com/mark3labs/go-bash/builtins/base64"
	_ "github.com/mark3labs/go-bash/builtins/cat"
	_ "github.com/mark3labs/go-bash/builtins/column"
	_ "github.com/mark3labs/go-bash/builtins/comm"
	_ "github.com/mark3labs/go-bash/builtins/cut"
	_ "github.com/mark3labs/go-bash/builtins/diff"
	_ "github.com/mark3labs/go-bash/builtins/expand"
	_ "github.com/mark3labs/go-bash/builtins/find"
	_ "github.com/mark3labs/go-bash/builtins/fold"
	_ "github.com/mark3labs/go-bash/builtins/head"
	_ "github.com/mark3labs/go-bash/builtins/join"
	_ "github.com/mark3labs/go-bash/builtins/md5sum"
	_ "github.com/mark3labs/go-bash/builtins/nl"
	_ "github.com/mark3labs/go-bash/builtins/od"
	_ "github.com/mark3labs/go-bash/builtins/paste"
	_ "github.com/mark3labs/go-bash/builtins/rev"
	_ "github.com/mark3labs/go-bash/builtins/sha1sum"
	_ "github.com/mark3labs/go-bash/builtins/sha256sum"
	_ "github.com/mark3labs/go-bash/builtins/sort"
	_ "github.com/mark3labs/go-bash/builtins/strings"
	_ "github.com/mark3labs/go-bash/builtins/tac"
	_ "github.com/mark3labs/go-bash/builtins/tail"
	_ "github.com/mark3labs/go-bash/builtins/tee"
	_ "github.com/mark3labs/go-bash/builtins/tr"
	_ "github.com/mark3labs/go-bash/builtins/unexpand"
	_ "github.com/mark3labs/go-bash/builtins/uniq"
	_ "github.com/mark3labs/go-bash/builtins/wc"
	_ "github.com/mark3labs/go-bash/builtins/xargs"

	// Phase 10 Wave D: pattern engines.
	_ "github.com/mark3labs/go-bash/builtins/awk"
	_ "github.com/mark3labs/go-bash/builtins/egrep"
	_ "github.com/mark3labs/go-bash/builtins/fgrep"
	_ "github.com/mark3labs/go-bash/builtins/grep"
	_ "github.com/mark3labs/go-bash/builtins/jq"
	_ "github.com/mark3labs/go-bash/builtins/rg"
	_ "github.com/mark3labs/go-bash/builtins/sed"

	// Phase 10 Wave E: data formats.
	_ "github.com/mark3labs/go-bash/builtins/sqlite3"
	_ "github.com/mark3labs/go-bash/builtins/xan"
	_ "github.com/mark3labs/go-bash/builtins/yq"

	// Phase 10 Wave F: archive / compression.
	_ "github.com/mark3labs/go-bash/builtins/gunzip"
	_ "github.com/mark3labs/go-bash/builtins/gzip"
	_ "github.com/mark3labs/go-bash/builtins/tar"
	_ "github.com/mark3labs/go-bash/builtins/zcat"

	// Phase 10 Wave G: environment / shell.
	_ "github.com/mark3labs/go-bash/builtins/alias"
	_ "github.com/mark3labs/go-bash/builtins/bash"
	_ "github.com/mark3labs/go-bash/builtins/date"
	_ "github.com/mark3labs/go-bash/builtins/env"
	_ "github.com/mark3labs/go-bash/builtins/help"
	_ "github.com/mark3labs/go-bash/builtins/history"
	_ "github.com/mark3labs/go-bash/builtins/printenv"
	_ "github.com/mark3labs/go-bash/builtins/sh"
	_ "github.com/mark3labs/go-bash/builtins/time"
	_ "github.com/mark3labs/go-bash/builtins/timeout"
	_ "github.com/mark3labs/go-bash/builtins/unalias"

	// Phase 10 Wave H: network.
	_ "github.com/mark3labs/go-bash/builtins/curl"
	_ "github.com/mark3labs/go-bash/builtins/htmltomarkdown"

	// Phase 11: interpreter built-ins. Many command names below are
	// shadowed by mvdan/sh's native built-ins (cd, export, unset,
	// declare, read, :, true, false, pwd, set, shopt, exit, return,
	// break, continue, source, ., eval, hash, trap, readarray,
	// mapfile, dirs, pushd, popd, [, test, wait, getopts). For
	// shadowed entries the registered version is reachable only via
	// /bin/<name>; see DECISIONS.md (Phase 11) for the per-command
	// decision (override vs. accept shadow).
	_ "github.com/mark3labs/go-bash/builtins/breakcmd"
	_ "github.com/mark3labs/go-bash/builtins/cd"
	_ "github.com/mark3labs/go-bash/builtins/colon"
	_ "github.com/mark3labs/go-bash/builtins/compgen"
	_ "github.com/mark3labs/go-bash/builtins/complete"
	_ "github.com/mark3labs/go-bash/builtins/compopt"
	_ "github.com/mark3labs/go-bash/builtins/continuecmd"
	_ "github.com/mark3labs/go-bash/builtins/declare"
	_ "github.com/mark3labs/go-bash/builtins/dirs"
	_ "github.com/mark3labs/go-bash/builtins/eval"
	_ "github.com/mark3labs/go-bash/builtins/exit"
	_ "github.com/mark3labs/go-bash/builtins/export"
	_ "github.com/mark3labs/go-bash/builtins/getopts"
	_ "github.com/mark3labs/go-bash/builtins/hash"
	_ "github.com/mark3labs/go-bash/builtins/jobs"
	_ "github.com/mark3labs/go-bash/builtins/let"
	_ "github.com/mark3labs/go-bash/builtins/local"
	_ "github.com/mark3labs/go-bash/builtins/mapfile"
	_ "github.com/mark3labs/go-bash/builtins/popd"
	_ "github.com/mark3labs/go-bash/builtins/pushd"
	_ "github.com/mark3labs/go-bash/builtins/read"
	_ "github.com/mark3labs/go-bash/builtins/readonly"
	_ "github.com/mark3labs/go-bash/builtins/returncmd"
	_ "github.com/mark3labs/go-bash/builtins/set"
	_ "github.com/mark3labs/go-bash/builtins/shopt"
	_ "github.com/mark3labs/go-bash/builtins/source"
	_ "github.com/mark3labs/go-bash/builtins/test"
	_ "github.com/mark3labs/go-bash/builtins/trap"
	_ "github.com/mark3labs/go-bash/builtins/umask"
	_ "github.com/mark3labs/go-bash/builtins/unset"
	_ "github.com/mark3labs/go-bash/builtins/wait"
)
