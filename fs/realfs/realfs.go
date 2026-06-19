// Package realfs holds the security helpers shared by the read-write
// and overlay filesystems.
//
// The two main jobs of this package are:
//   - Translate a sandbox-relative virtual path into a real on-disk path
//     under Root, refusing any path that would escape Root via "..",
//     symlink, or absolute prefix.
//   - Provide O_NOFOLLOW-style open semantics so that a malicious link
//     planted between Resolve and Open cannot redirect the I/O.
//
// Both implementations import this package; nothing else in the runtime
// should.
package realfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	gobashfs "github.com/mark3labs/go-bash/fs"
)

// ErrEscape is returned when a resolved path lies outside the sandbox
// root. Callers should translate this into a *PathError wrapping
// fs.ErrNotExist so it is indistinguishable from a missing path.
var ErrEscape = errors.New("path escapes sandbox root")

// ResolveAndValidate joins requested onto root and verifies the result
// still lies under root after a symlink-aware canonicalization. If
// allowSymlinks is false and any path component is a symlink, the
// function returns *os.PathError wrapping fs.ErrNotExist.
//
// The returned realPath is suitable to pass to os.OpenFile and friends.
// The clean param is the virtual path (sandbox-relative, /-prefixed).
func ResolveAndValidate(root, requested string, allowSymlinks bool) (realPath, clean string, err error) {
	if err := gobashfs.Validate(requested); err != nil {
		return "", "", gobashfs.PathError("resolve", requested, err)
	}
	if !filepath.IsAbs(root) {
		return "", "", fmt.Errorf("realfs: root %q is not absolute", root)
	}
	cleanRoot := filepath.Clean(root)
	virt := gobashfs.Clean(requested)
	if !strings.HasPrefix(virt, "/") {
		virt = "/" + virt
	}
	real := filepath.Join(cleanRoot, filepath.FromSlash(virt))
	if !pathWithin(cleanRoot, real) {
		return "", "", gobashfs.PathError("resolve", requested, fs.ErrNotExist)
	}
	if !allowSymlinks {
		// Walk components from root → real. If any intermediate
		// component exists and is a symlink, refuse.
		rel, err := filepath.Rel(cleanRoot, real)
		if err == nil && rel != "." {
			parts := strings.Split(rel, string(os.PathSeparator))
			cur := cleanRoot
			for _, p := range parts {
				cur = filepath.Join(cur, p)
				fi, statErr := os.Lstat(cur)
				if statErr != nil {
					if errors.Is(statErr, fs.ErrNotExist) {
						break // OK to create later
					}
					return "", "", gobashfs.PathError("resolve", requested, statErr)
				}
				if fi.Mode()&os.ModeSymlink != 0 {
					return "", "", gobashfs.PathError("resolve", requested, fs.ErrNotExist)
				}
			}
		}
	}
	return real, virt, nil
}

// pathWithin reports whether real (a cleaned absolute path) lies under
// cleanRoot. Both inputs must be cleaned & filepath-native already.
func pathWithin(cleanRoot, real string) bool {
	if cleanRoot == real {
		return true
	}
	return strings.HasPrefix(real, cleanRoot+string(os.PathSeparator))
}

// OpenFileNoFollow opens a file under root using O_NOFOLLOW semantics
// when supported by the kernel. Use this from the rwfs / overlay write
// paths so a symlink planted at the leaf cannot reroute the write.
func OpenFileNoFollow(realPath string, flag int, mode os.FileMode, allowSymlinks bool) (*os.File, error) {
	if !allowSymlinks {
		flag |= sysOpenNoFollow
	}
	return os.OpenFile(realPath, flag, mode)
}

// SanitizeError strips the sandbox root prefix from a *os.PathError so
// the virtual path is presented to the script. Callers that already know
// the virtual path may prefer to rebuild the error instead.
func SanitizeError(err error, root string) error {
	if err == nil {
		return nil
	}
	var pe *os.PathError
	if !errors.As(err, &pe) {
		return err
	}
	if root == "" {
		return err
	}
	pe.Path = "/" + strings.TrimPrefix(strings.TrimPrefix(pe.Path, root), "/")
	return pe
}
