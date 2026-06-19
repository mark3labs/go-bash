// Package fs defines the virtual filesystem interface used by go-bash and
// utility functions for path manipulation.
//
// All paths exchanged with FileSystem implementations are POSIX-style
// forward-slash paths. Paths containing a NUL byte ("\x00") are rejected
// at the API boundary — this matches POSIX and bash semantics and closes
// a class of injection attacks that exploit C-string truncation in lower
// layers.
package fs

import (
	"errors"
	"strings"
)

// MaxSymlinkDepth bounds the number of symlink hops any resolver in the
// FileSystem layer will follow before returning a loop error. Matches the
// Linux kernel's MAXSYMLINKS / ELOOP threshold.
const MaxSymlinkDepth = 40

// ErrNullByte is returned by Validate when the input path contains a NUL
// byte. Callers should not unwrap this — match via errors.Is.
var ErrNullByte = errors.New("path contains null byte")

// ErrEmptyPath is returned by Validate when the input is the empty string.
var ErrEmptyPath = errors.New("empty path")

// Validate enforces the path-level invariants every FileSystem
// implementation must check before doing any I/O. It is intentionally
// cheap and pure so it can be called at every public entry point.
func Validate(path string) error {
	if path == "" {
		return ErrEmptyPath
	}
	if strings.IndexByte(path, 0) >= 0 {
		return ErrNullByte
	}
	return nil
}

// Clean is a POSIX-style path cleaner. It collapses repeated slashes,
// resolves "." and ".." textually (without touching the filesystem),
// and removes a trailing slash except on the root path "/".
//
// Empty input returns ".". A purely-relative path that resolves to the
// empty string after cleaning returns ".".
func Clean(path string) string {
	if path == "" {
		return "."
	}
	abs := strings.HasPrefix(path, "/")
	parts := strings.Split(path, "/")

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		switch p {
		case "", ".":
			// drop
		case "..":
			if len(out) > 0 && out[len(out)-1] != ".." {
				out = out[:len(out)-1]
			} else if !abs {
				out = append(out, "..")
			}
			// On an absolute path, ".." at root is a no-op.
		default:
			out = append(out, p)
		}
	}
	joined := strings.Join(out, "/")
	if abs {
		return "/" + joined
	}
	if joined == "" {
		return "."
	}
	return joined
}

// Join concatenates path elements with "/" and Cleans the result. Empty
// elements are skipped. Join() == ".".
func Join(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return "."
	}
	return Clean(strings.Join(nonEmpty, "/"))
}

// Resolve returns the cleaned absolute form of path against base. If path
// is already absolute, base is ignored. If base is empty and path is
// relative, the result is just Clean(path).
func Resolve(base, path string) string {
	if strings.HasPrefix(path, "/") {
		return Clean(path)
	}
	if base == "" {
		return Clean(path)
	}
	return Clean(base + "/" + path)
}

// Dirname returns everything up to (but not including) the last "/" in
// path. For "/a/b" it returns "/a"; for "a" it returns "."; for "/" it
// returns "/".
func Dirname(path string) string {
	p := Clean(path)
	if p == "/" {
		return "/"
	}
	i := strings.LastIndexByte(p, '/')
	switch {
	case i < 0:
		return "."
	case i == 0:
		return "/"
	default:
		return p[:i]
	}
}

// Basename returns the final path component of path. For "/" it returns
// "/"; for "/a/b" it returns "b"; for "a" it returns "a".
func Basename(path string) string {
	p := Clean(path)
	if p == "/" {
		return "/"
	}
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return p
	}
	return p[i+1:]
}

// IsWithinRoot reports whether path lies inside root (after cleaning),
// treating root as a prefix boundary. Both inputs are Cleaned before
// comparison so the answer is purely lexical — callers that need a
// symlink-resolved check must canonicalize first.
func IsWithinRoot(root, path string) bool {
	r := Clean(root)
	p := Clean(path)
	if r == "/" {
		return strings.HasPrefix(p, "/")
	}
	if p == r {
		return true
	}
	return strings.HasPrefix(p, r+"/")
}

// SplitParts returns the cleaned path's components (without any leading
// slash). For "/a/b/c" it returns ["a", "b", "c"]; for "/" it returns nil.
// Helper used by tree-walking implementations.
func SplitParts(path string) []string {
	p := Clean(path)
	if p == "/" || p == "." {
		return nil
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
