package network

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// allowEntry is the compiled form of an AllowedURLEntry: origin
// pre-normalized and path prefix validated.
type allowEntry struct {
	origin     string // scheme://host[:port]; default ports stripped
	pathPrefix string // raw path prefix (must already start with "/")
	transforms []RequestTransform
}

// compileAllowList parses each entry and returns a slice of compiled
// matchers. An entry fails compilation if it lacks scheme or host or
// if its raw path contains an ambiguous %2f / %5c sequence.
//
// SPEC §9.2.
func compileAllowList(entries []AllowedURLEntry) ([]allowEntry, error) {
	out := make([]allowEntry, 0, len(entries))
	for i, e := range entries {
		ce, err := compileEntry(e)
		if err != nil {
			return nil, fmt.Errorf("network: AllowedURLPrefixes[%d]: %w", i, err)
		}
		out = append(out, ce)
	}
	return out, nil
}

func compileEntry(e AllowedURLEntry) (allowEntry, error) {
	if e.URL == "" {
		return allowEntry{}, errors.New("empty URL")
	}
	// Reject %2f / %5c BEFORE url.Parse so the parser doesn't decode
	// them. The ambiguity is exactly that "/v1%2fadmin" might match a
	// "/v1/" entry or not depending on decoding; we side-step the
	// question by rejecting at compile time.
	if containsEncodedSlash(e.URL) {
		return allowEntry{}, fmt.Errorf("path contains ambiguous %%2f or %%5c: %q", e.URL)
	}
	u, err := url.Parse(e.URL)
	if err != nil {
		return allowEntry{}, fmt.Errorf("parse %q: %w", e.URL, err)
	}
	if u.Scheme == "" {
		return allowEntry{}, fmt.Errorf("missing scheme: %q", e.URL)
	}
	if u.Host == "" {
		return allowEntry{}, fmt.Errorf("missing host: %q", e.URL)
	}
	origin := normalizeOrigin(u)
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return allowEntry{
		origin:     origin,
		pathPrefix: path,
		transforms: e.Transform,
	}, nil
}

// containsEncodedSlash detects %2f / %5c (case-insensitive) anywhere
// in s. The check is intentionally on the raw input, not after
// url.Parse: url.Parse leaves percent-encoded reserved characters
// intact in u.RawPath, but we also want to catch entries where the
// encoded form appears in the userinfo, host, or query — anywhere a
// future code path might interpret it as a separator.
func containsEncodedSlash(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c")
}

// normalizeOrigin returns scheme://host[:port] with default ports
// stripped. Both scheme and host are lowercased — RFC 3986 §3.1
// declares them case-insensitive, and the TS matcher normalizes the
// same way.
func normalizeOrigin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" && !isDefaultPort(scheme, port) {
		return scheme + "://" + host + ":" + port
	}
	return scheme + "://" + host
}

func isDefaultPort(scheme, port string) bool {
	switch scheme {
	case "http", "ws":
		return port == "80"
	case "https", "wss":
		return port == "443"
	}
	return false
}

// matches reports whether candidate is permitted by entry. The
// candidate URL is normalized the same way as the entry origin, then
// the path is checked as a literal prefix.
func (a allowEntry) matches(candidate *url.URL) bool {
	if normalizeOrigin(candidate) != a.origin {
		return false
	}
	cPath := candidate.EscapedPath()
	if cPath == "" {
		cPath = "/"
	}
	return strings.HasPrefix(cPath, a.pathPrefix)
}

// findMatch returns the first allowEntry that matches candidate, or
// (allowEntry{}, false) if none do. First-match wins so the caller
// can order entries by specificity.
func findMatch(entries []allowEntry, candidate *url.URL) (allowEntry, bool) {
	for _, e := range entries {
		if e.matches(candidate) {
			return e, true
		}
	}
	return allowEntry{}, false
}
