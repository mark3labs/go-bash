package network

import (
	"context"
	"time"
)

// Default values applied by NewSecureFetch when Config fields are
// zero. Exported so tests and host code can refer to the same
// constants without re-deriving them.
const (
	DefaultMaxRedirects   = 20
	DefaultTimeout        = 30 * time.Second
	DefaultMaxResponseSize = 10 * 1024 * 1024 // 10 MiB
)

// DefaultAllowedMethods is the methods set applied when
// Config.AllowedMethods is nil. GET and HEAD are the safest defaults
// matching the just-bash TS table.
var DefaultAllowedMethods = []string{"GET", "HEAD"}

// Config configures SecureFetch. The zero Config denies every URL —
// to enable any traffic at all, set AllowedURLPrefixes or
// DangerouslyAllowFullAccess.
//
// The spec
type Config struct {
	// AllowedURLPrefixes whitelists specific origins (scheme+host[+port])
	// and optionally a path prefix and per-entry header transforms. An
	// incoming URL matches when its origin equals the entry's origin
	// AND its path begins with the entry's path. Each entry is
	// re-validated on every redirect hop.
	AllowedURLPrefixes []AllowedURLEntry

	// AllowedMethods restricts which HTTP methods the Doer will issue.
	// Nil means DefaultAllowedMethods (GET, HEAD). Case-insensitive.
	AllowedMethods []string

	// DangerouslyAllowFullAccess turns SecureFetch into a pass-through.
	// All URLs and methods are permitted. DenyPrivateRanges is still
	// honored. Intended for development sandboxes — do not enable in
	// production hosts.
	DangerouslyAllowFullAccess bool

	// MaxRedirects caps the redirect chain. The initial request is hop
	// zero; the limit is the number of additional Location hops the
	// client will follow. Zero means DefaultMaxRedirects (20). A
	// negative value disables redirect following entirely (the first
	// 3xx returns to the caller).
	MaxRedirects int

	// Timeout is the per-request deadline (initial + redirects). Zero
	// means DefaultTimeout (30s). A negative value disables the
	// per-request timeout (caller's ctx is still honored).
	Timeout time.Duration

	// MaxResponseSize caps the response body in bytes. Zero means
	// DefaultMaxResponseSize (10 MiB). A negative value disables the
	// cap; SecureFetch will still read the whole body into memory.
	MaxResponseSize int64

	// DenyPrivateRanges resolves every connection target via DNSResolve
	// (or net.DefaultResolver if nil) and rejects any address in a
	// private, loopback, link-local, or multicast range. Combine with
	// DangerouslyAllowFullAccess to permit arbitrary public hosts
	// while still blocking SSRF into the host's local network.
	DenyPrivateRanges bool

	// DNSResolve is an injection point for tests and custom resolvers.
	// When nil, SecureFetch falls back to net.DefaultResolver.LookupIP.
	// When non-nil, it is the sole source of address records used for
	// the DenyPrivateRanges check.
	DNSResolve func(ctx context.Context, host string) ([]DNSLookupResult, error)
}

// AllowedURLEntry is one allow-list slot. URL provides the
// scheme://host[:port][/path] prefix; Transform lists header
// rewrites applied on every matching request hop.
//
// The spec
type AllowedURLEntry struct {
	// URL is the prefix to allow. Must include scheme and host. The
	// path component is treated as a literal prefix — entry URL
	// "https://api.example.com/v1/" matches request URLs whose path
	// starts with "/v1/" on that origin. %2f / %5c in the path are
	// rejected at compile time because they create ambiguity with
	// real / and \ characters.
	URL string

	// Transform is the list of header rewrites applied when this entry
	// matches the outgoing request. Multiple entries with overlapping
	// origins all contribute their headers, last-write-wins per header
	// name.
	Transform []RequestTransform
}

// RequestTransform is a header overlay applied to a matching request.
// Headers replace any existing header of the same name.
//
// The spec
type RequestTransform struct {
	// Headers is the name → value overlay. Header names are
	// canonicalized via http.CanonicalHeaderKey before application.
	Headers map[string]string
}

// DNSLookupResult is one resolved address from Config.DNSResolve.
// Family is the AF_* constant: 4 for AF_INET, 6 for AF_INET6.
//
// The spec
type DNSLookupResult struct {
	// Address is the textual representation (192.0.2.1 or 2001:db8::1).
	Address string

	// Family is 4 (AF_INET) or 6 (AF_INET6). Unknown families cause
	// the address to be skipped during private-range checks.
	Family int
}

// withDefaults returns a Config with zero-valued fields replaced by
// the package defaults. Negative sentinels (MaxRedirects=-1,
// Timeout<0, MaxResponseSize<0) are preserved verbatim so callers can
// opt out of each cap individually.
func (c *Config) withDefaults() Config {
	out := Config{}
	if c != nil {
		out = *c
	}
	if out.MaxRedirects == 0 {
		out.MaxRedirects = DefaultMaxRedirects
	}
	if out.Timeout == 0 {
		out.Timeout = DefaultTimeout
	}
	if out.MaxResponseSize == 0 {
		out.MaxResponseSize = DefaultMaxResponseSize
	}
	if len(out.AllowedMethods) == 0 {
		out.AllowedMethods = append([]string(nil), DefaultAllowedMethods...)
	}
	return out
}
