package builtinutil

import (
	"strings"

	gbfs "github.com/mark3labs/go-bash/fs"
)

// ResolvePath joins a possibly-relative path against the dispatch
// Cwd and returns the cleaned absolute form. Empty cwd is treated as
// "/" so paths still resolve to a usable absolute form.
func ResolvePath(cwd, p string) string {
	if cwd == "" {
		cwd = "/"
	}
	if strings.HasPrefix(p, "/") {
		return gbfs.Clean(p)
	}
	return gbfs.Clean(cwd + "/" + p)
}
