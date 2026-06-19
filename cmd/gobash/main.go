// Command gobash is the CLI entry point for the go-bash runtime.
//
// The argv surface is a strict superset of just-bash's CLI
// (vercel-labs/just-bash, packages/just-bash/src/cli/just-bash.ts):
// the overlapping flags (`-c`, `-e`/`--errexit`, `--root`, `--cwd`,
// `--allow-write`, `--json`, `-h`/`--help`, `-v`/`--version`,
// `--python`, `--javascript`, positional `[script-file]` /
// `[root-path]`, combined short flags like `-ec`) behave
// byte-identically. Go-side additions (`--no-network`,
// `--network-allow PREFIX`) are documented in --help and default
// off, so invocations that work against just-bash continue to work
// here.
//
// The CLI is intentionally kept small: argv parsing + wiring
// BashOptions/ExecOptions, plus the `--root` host directory mounted
// at `/home/user/project` via overlayfs (read-only by default).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	gobash "github.com/mark3labs/go-bash"
	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/mountfs"
	"github.com/mark3labs/go-bash/fs/overlayfs"
	"github.com/mark3labs/go-bash/network"
)

// version is the build version reported by --version. It defaults to
// "dev" and is overwritten at release time via the goreleaser
// ldflag -X main.version (see .goreleaser.yaml), matching the kit
// convention. The --version line keeps its "gobash <version>" shape
// so consumers parsing it by regex see a stable form.
var version = "dev"

// defaultMountPoint matches just-bash's OverlayFs default mount
// point. The host --root directory is exposed at this virtual path;
// the script's default cwd is the same path.
const defaultMountPoint = "/home/user/project"

// helpText is printed for -h/--help. The wording mirrors just-bash's
// printHelp() output so shell completions and screen-scrapers that
// rely on the just-bash text continue to work.
const helpText = `gobash - A secure bash environment for AI agents

Usage:
  gobash [options] [script-file]
  gobash -c 'script' [options]
  echo 'script' | gobash [options]

Options:
  -c <script>             Execute the script from command line argument
  -e, --errexit           Exit immediately if a command exits with non-zero status
  --root <path>           Root directory for OverlayFS (default: current directory)
  --cwd <path>            Working directory within the sandbox (default: project mount point)
  --allow-write           Allow write operations (default: read-only)
  --python                Enable python3/python commands (requires pythonexec build)
  --javascript            Enable js-exec command (requires jsexec build)
  --json                  Output results as JSON (stdout, stderr, exitCode)
  --no-network            Explicitly disable network (default; redundant)
  --network-allow PREFIX  Allow network requests to PREFIX (repeatable)
  -h, --help              Show this help message
  -v, --version           Show version

Security:
  - Reads from the real filesystem (read-only via OverlayFS)
  - Write operations are blocked by default (use --allow-write to enable)
  - Cannot escape the root directory
  - No network access unless --network-allow PREFIX is supplied

Filesystem:
  The root directory is mounted at /home/user/project in the virtual
  filesystem. The working directory starts at this mount point.
`

// cliOptions mirrors the just-bash CliOptions struct.
type cliOptions struct {
	script         string
	scriptSet      bool
	scriptFile     string
	root           string
	cwd            string
	cwdOverridden  bool
	errexit        bool
	allowWrite     bool
	python         bool
	javascript     bool
	json           bool
	help           bool
	version        bool
	noNetwork      bool
	networkAllowed []string
}

// parseError signals an argv-parsing failure. main converts it to
// stderr + exit-2 (matching just-bash's process.exit(1) on parse
// errors; we use 2 to follow the GNU convention for "command-line
// usage error", since the just-bash divergence is minor).
type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

// parseArgs ports the just-bash parseArgs() function verbatim,
// extended with the go-bash-specific flags. Errors return a
// *parseError so the caller can route them to stderr without
// printing a stack trace.
//
//nolint:gocyclo // the argv parser is intrinsically branchy; splitting it would obscure the just-bash line-by-line correspondence.
func parseArgs(args []string, hostCwd string) (*cliOptions, error) {
	opts := &cliOptions{
		root: hostCwd,
		cwd:  defaultMountPoint,
	}
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			opts.help = true
			i++
		case arg == "-v" || arg == "--version":
			opts.version = true
			i++
		case arg == "-c":
			if i+1 >= len(args) {
				return nil, &parseError{"Error: -c requires a script argument"}
			}
			opts.script = args[i+1]
			opts.scriptSet = true
			i += 2
		case arg == "-e" || arg == "--errexit":
			opts.errexit = true
			i++
		case arg == "--root":
			if i+1 >= len(args) {
				return nil, &parseError{"Error: --root requires a path argument"}
			}
			abs, err := filepath.Abs(args[i+1])
			if err != nil {
				return nil, &parseError{fmt.Sprintf("Error: --root: %v", err)}
			}
			opts.root = abs
			i += 2
		case arg == "--cwd":
			if i+1 >= len(args) {
				return nil, &parseError{"Error: --cwd requires a path argument"}
			}
			opts.cwd = args[i+1]
			opts.cwdOverridden = true
			i += 2
		case arg == "--json":
			opts.json = true
			i++
		case arg == "--allow-write":
			opts.allowWrite = true
			i++
		case arg == "--python":
			opts.python = true
			i++
		case arg == "--javascript":
			opts.javascript = true
			i++
		case arg == "--no-network":
			opts.noNetwork = true
			i++
		case arg == "--network-allow":
			if i+1 >= len(args) {
				return nil, &parseError{"Error: --network-allow requires a PREFIX argument"}
			}
			opts.networkAllowed = append(opts.networkAllowed, args[i+1])
			i += 2
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 2:
			// Combined short options like `-ec`. -c, when present,
			// must be last so the next argv element is its value.
			flags := arg[1:]
			for j, flag := range flags {
				switch flag {
				case 'e':
					opts.errexit = true
				case 'h':
					opts.help = true
				case 'v':
					opts.version = true
				case 'c':
					// -c must be last in the combined flag block.
					if j != len(flags)-1 {
						return nil, &parseError{"Error: -c must be the last flag in a combined short option"}
					}
					if i+1 >= len(args) {
						return nil, &parseError{"Error: -c requires a script argument"}
					}
					opts.script = args[i+1]
					opts.scriptSet = true
					i++
				default:
					return nil, &parseError{fmt.Sprintf("Error: Unknown option: -%c", flag)}
				}
			}
			i++
		case strings.HasPrefix(arg, "-"):
			return nil, &parseError{fmt.Sprintf("Error: Unknown option: %s", arg)}
		default:
			// Positional argument — first is script file, second
			// is root (matching just-bash). A second positional is
			// only treated as root when --root was not explicitly
			// supplied (i.e. root is still hostCwd).
			switch {
			case opts.scriptFile == "" && !opts.scriptSet:
				opts.scriptFile = arg
			case opts.scriptFile != "" && opts.root == hostCwd:
				abs, err := filepath.Abs(arg)
				if err != nil {
					return nil, &parseError{fmt.Sprintf("Error: root path: %v", err)}
				}
				opts.root = abs
			}
			i++
		}
	}
	return opts, nil
}

// IO bundles the four streams Run interacts with. main wires this
// from os.{Stdin,Stdout,Stderr,Args} + os.Getwd; tests substitute
// in-memory readers/writers.
type IO struct {
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	HostCwd string

	// StdinIsTTY reports whether the runtime stdin is connected to
	// a terminal. When true and no -c / script-file is supplied,
	// the CLI prints help and exits non-zero (matching just-bash).
	// When false, the CLI reads stdin as the script.
	StdinIsTTY bool
}

// Run parses argv and runs the requested script. The returned int is
// the process exit code. All output is written through io.Stdout /
// io.Stderr; nothing in this function calls os.Exit so tests can
// assert behavior in-process.
func Run(ctx context.Context, args []string, ioS IO) int {
	opts, err := parseArgs(args, ioS.HostCwd)
	if err != nil {
		_, _ = fmt.Fprintln(ioS.Stderr, err.Error())
		return 2
	}
	if opts.help {
		_, _ = fmt.Fprint(ioS.Stdout, helpText)
		return 0
	}
	if opts.version {
		_, _ = fmt.Fprintf(ioS.Stdout, "gobash %s\n", version)
		return 0
	}

	// Resolve script source. Priority matches just-bash:
	// -c > positional script file > stdin (if not TTY) > help+exit-1.
	var script string
	switch {
	case opts.scriptSet:
		script = opts.script
	case opts.scriptFile != "":
		// Read via the host OS; just-bash reads via the OverlayFS
		// (so script files in the mounted dir are visible). For
		// the Go port we match that by reading through the
		// constructed FS below. Defer to post-FS-setup.
	case !ioS.StdinIsTTY:
		data, err := io.ReadAll(ioS.Stdin)
		if err != nil {
			_, _ = fmt.Fprintln(ioS.Stderr, "Error: read stdin:", err)
			return 1
		}
		script = string(data)
	default:
		_, _ = fmt.Fprint(ioS.Stdout, helpText)
		return 1
	}

	// Build the filesystem. mountfs with a memfs base + overlayfs
	// mounted at /home/user/project mirrors just-bash's OverlayFs
	// mount-point semantics exactly: the host --root directory is
	// exposed at the mount point, reads fall through to the host
	// (or to the overlay if shadowed), writes land in the in-memory
	// overlay (or are rejected when --allow-write is off).
	overlay, err := overlayfs.New(overlayfs.Options{
		Root:     opts.root,
		ReadOnly: !opts.allowWrite,
	})
	if err != nil {
		_, _ = fmt.Fprintln(ioS.Stderr, "Error: overlay:", err)
		return 1
	}
	base, err := mountfs.New(mountfs.Options{
		Base: newMemBaseFS(),
		Mounts: []mountfs.Mount{
			{Path: defaultMountPoint, FileSystem: overlay},
		},
	})
	if err != nil {
		_, _ = fmt.Fprintln(ioS.Stderr, "Error: mountfs:", err)
		return 1
	}

	// Read scriptFile through the constructed FS so files under
	// --root are reachable via their virtual path.
	if opts.scriptFile != "" {
		virtual := opts.scriptFile
		if !strings.HasPrefix(virtual, "/") {
			virtual = defaultMountPoint + "/" + virtual
		}
		data, err := base.ReadFile(gbfs.Clean(virtual))
		if err != nil {
			_, _ = fmt.Fprintln(ioS.Stderr, "Error: Cannot read script file:", opts.scriptFile)
			return 1
		}
		script = string(data)
	}

	// Empty script is a no-op (matching just-bash).
	if strings.TrimSpace(script) == "" {
		if opts.json {
			emitJSON(ioS.Stdout, "", "", 0)
		}
		return 0
	}

	// Resolve cwd. cwdOverridden lets the user pin an arbitrary
	// virtual path (e.g. /tmp); otherwise we default to the mount
	// point. Normalize to absolute so MkdirAll doesn't refuse.
	cwd := opts.cwd
	if !opts.cwdOverridden {
		cwd = defaultMountPoint
	}
	if !strings.HasPrefix(cwd, "/") {
		cwd = "/" + cwd
	}
	cwd = gbfs.Clean(cwd)

	// Build the network policy. --no-network is the explicit form
	// (default behavior). --network-allow PREFIX (repeatable) builds
	// an AllowedURLPrefixes entry per prefix; without it, fetch is
	// nil and curl reports "network disabled".
	var netCfg *network.Config
	if !opts.noNetwork && len(opts.networkAllowed) > 0 {
		entries := make([]network.AllowedURLEntry, 0, len(opts.networkAllowed))
		for _, p := range opts.networkAllowed {
			entries = append(entries, network.AllowedURLEntry{URL: p})
		}
		netCfg = &network.Config{AllowedURLPrefixes: entries}
	}

	b, err := gobash.New(gobash.BashOptions{
		FS:      base,
		Cwd:     cwd,
		Network: netCfg,
	})
	if err != nil {
		_, _ = fmt.Fprintln(ioS.Stderr, "Error:", err)
		return 1
	}
	// --errexit prepends `set -e` (matching just-bash).
	if opts.errexit {
		script = "set -e\n" + script
	}

	// In --json mode, leave Stdout/Stderr nil so Bash.Exec captures
	// them into the returned ExecResult fields (which we then pack
	// into the envelope). In stream mode, pass our writers so the
	// script's output flows through immediately.
	execOpts := gobash.ExecOptions{Stdin: ioS.Stdin}
	if !opts.json {
		execOpts.Stdout = ioS.Stdout
		execOpts.Stderr = ioS.Stderr
	}
	res, err := b.Exec(ctx, script, execOpts)
	if err != nil {
		// Parse / limit / cancellation errors. Match just-bash's
		// shape: one-line error + exit 1.
		msg := err.Error()
		var iferr *iofs.PathError
		if errors.As(err, &iferr) {
			msg = "fs error: " + iferr.Error()
		}
		if opts.json {
			emitJSON(ioS.Stdout, "", msg+"\n", 1)
		} else {
			_, _ = fmt.Fprintln(ioS.Stderr, msg)
		}
		return 1
	}

	if opts.json {
		emitJSON(ioS.Stdout, res.Stdout, res.Stderr, res.ExitCode)
	}
	return res.ExitCode
}

// emitJSON writes the result envelope as a single line.
func emitJSON(w io.Writer, stdout, stderr string, exitCode int) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exitCode"`
	}{Stdout: stdout, Stderr: stderr, ExitCode: exitCode})
}

// newMemBaseFS returns an empty in-memory FS used as the base of
// the mountfs. The /bin stubs are NOT pre-seeded here — registry
// dispatch resolves /bin/X by basename via interp.lookupCommand, so
// the stub files only matter for scripts that stat() them. The CLI
// currently treats their absence as acceptable; a follow-up could
// pre-seed them by importing the package-private applyDefaultLayout.
func newMemBaseFS() gbfs.FileSystem {
	// Lazy import: building memfs is trivial.
	return newMemFS()
}

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Error: getwd:", err)
		os.Exit(1)
	}
	isTTY := isCharDevice(os.Stdin)
	code := Run(context.Background(), os.Args[1:], IO{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		HostCwd:    cwd,
		StdinIsTTY: isTTY,
	})
	os.Exit(code)
}
