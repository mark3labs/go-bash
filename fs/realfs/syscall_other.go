//go:build !unix

package realfs

// sysOpenNoFollow is a no-op on non-Unix platforms — there is no portable
// equivalent and the rwfs / overlay implementations fall back to the
// post-open Lstat check in ResolveAndValidate.
const sysOpenNoFollow = 0
