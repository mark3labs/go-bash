// Package tree implements the `tree` built-in.
//
// Flags: -L LEVEL, -d (dirs only), -a (all), -J (JSON), -C (colors).
package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	iofs "io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "tree [-a -d -J -C -L LEVEL] [PATH]"
const helpText = `Usage: tree [OPTION]... [PATH]
List contents of directories in a tree-like format.

  -a            All files; do not ignore entries starting with .
  -d            List directories only
  -L LEVEL      Descend only level directories deep
  -J            Output JSON
  -C            Turn colorization on (no-op)`

// New returns the tree command.
func New() command.Command { return command.Define("tree", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	all := false
	dirsOnly := false
	jsonOut := false
	maxDepth := -1
	var root string
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-a":
			all = true
		case a == "-d":
			dirsOnly = true
		case a == "-J":
			jsonOut = true
		case a == "-C":
			// no-op
		case a == "-L":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "tree", 1, "Invalid level, must be > 0: %s", args[i])
			}
			maxDepth = n
		case strings.HasPrefix(a, "-L"):
			n, err := strconv.Atoi(a[2:])
			if err != nil || n < 1 {
				return builtinutil.Errorf(c.Stderr, "tree", 1, "Invalid level: %s", a[2:])
			}
			maxDepth = n
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			if root == "" {
				root = a
			}
		}
	}
	if root == "" {
		root = "."
	}
	if c.FS == nil {
		return builtinutil.Errorf(c.Stderr, "tree", 1, "no filesystem")
	}
	rootAbs := builtinutil.ResolvePath(c.Cwd, root)
	if jsonOut {
		return runJSON(c, root, rootAbs, all, dirsOnly, maxDepth)
	}
	return runText(c, root, rootAbs, all, dirsOnly, maxDepth)
}

func runText(c *command.Context, displayRoot, rootAbs string, all, dirsOnly bool, maxDepth int) command.Result {
	if c.Stdout == nil {
		return command.Result{}
	}
	_, _ = fmt.Fprintln(c.Stdout, displayRoot)
	dirs, files, err := walkDir(c, rootAbs, all, dirsOnly, 1, maxDepth, c.Stdout, "")
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tree", 1, "%v", err)
	}
	if dirsOnly {
		_, _ = fmt.Fprintf(c.Stdout, "\n%d directories\n", dirs)
	} else {
		_, _ = fmt.Fprintf(c.Stdout, "\n%d directories, %d files\n", dirs, files)
	}
	return command.Result{}
}

func walkDir(c *command.Context, p string, all, dirsOnly bool, depth, maxDepth int, w io.Writer, prefix string) (int, int, error) {
	entries, err := c.FS.ReadDir(p)
	if err != nil {
		return 0, 0, err
	}
	// Filter and sort.
	filtered := make([]iofs.DirEntry, 0, len(entries))
	for _, e := range entries {
		if !all && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if dirsOnly && !e.IsDir() {
			continue
		}
		filtered = append(filtered, e)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name() < filtered[j].Name() })
	dirs := 0
	files := 0
	for i, e := range filtered {
		last := i == len(filtered)-1
		branch := "├── "
		nextPrefix := prefix + "│   "
		if last {
			branch = "└── "
			nextPrefix = prefix + "    "
		}
		_, _ = fmt.Fprintf(w, "%s%s%s\n", prefix, branch, e.Name())
		if e.IsDir() {
			dirs++
			if maxDepth < 0 || depth < maxDepth {
				sub := path.Join(p, e.Name())
				d, f, err := walkDir(c, sub, all, dirsOnly, depth+1, maxDepth, w, nextPrefix)
				if err == nil {
					dirs += d
					files += f
				}
			}
		} else {
			files++
		}
	}
	return dirs, files, nil
}

type jsonNode struct {
	Type     string      `json:"type"`
	Name     string      `json:"name"`
	Contents []*jsonNode `json:"contents,omitempty"`
}

func runJSON(c *command.Context, displayRoot, rootAbs string, all, dirsOnly bool, maxDepth int) command.Result {
	root := &jsonNode{Type: "directory", Name: displayRoot}
	if err := buildJSON(c, rootAbs, root, all, dirsOnly, 1, maxDepth); err != nil {
		return builtinutil.Errorf(c.Stderr, "tree", 1, "%v", err)
	}
	out := []any{root, map[string]any{"type": "report"}}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "tree", 1, "%v", err)
	}
	if c.Stdout != nil {
		_, _ = c.Stdout.Write(data)
		_, _ = io.WriteString(c.Stdout, "\n")
	}
	return command.Result{}
}

func buildJSON(c *command.Context, p string, parent *jsonNode, all, dirsOnly bool, depth, maxDepth int) error {
	entries, err := c.FS.ReadDir(p)
	if err != nil {
		return err
	}
	filtered := make([]iofs.DirEntry, 0, len(entries))
	for _, e := range entries {
		if !all && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if dirsOnly && !e.IsDir() {
			continue
		}
		filtered = append(filtered, e)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name() < filtered[j].Name() })
	for _, e := range filtered {
		child := &jsonNode{Name: e.Name()}
		if e.IsDir() {
			child.Type = "directory"
		} else {
			child.Type = "file"
		}
		parent.Contents = append(parent.Contents, child)
		if e.IsDir() && (maxDepth < 0 || depth < maxDepth) {
			_ = buildJSON(c, path.Join(p, e.Name()), child, all, dirsOnly, depth+1, maxDepth)
		}
	}
	return nil
}

func init() { command.RegisterBuiltin(New()) }
