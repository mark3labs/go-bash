package network

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestCompileAllowList_HappyPath(t *testing.T) {
	t.Parallel()
	entries := []AllowedURLEntry{
		{URL: "https://api.example.com/v1/"},
		{URL: "http://docs.example.com"},
		{URL: "https://EXAMPLE.com:443/path"}, // default port stripped
	}
	compiled, err := compileAllowList(entries)
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	if got, want := len(compiled), 3; got != want {
		t.Fatalf("compiled len = %d, want %d", got, want)
	}
	if got, want := compiled[0].origin, "https://api.example.com"; got != want {
		t.Errorf("entry[0].origin = %q, want %q", got, want)
	}
	if got, want := compiled[0].pathPrefix, "/v1/"; got != want {
		t.Errorf("entry[0].pathPrefix = %q, want %q", got, want)
	}
	// Default port + uppercase scheme/host both normalized.
	if got, want := compiled[2].origin, "https://example.com"; got != want {
		t.Errorf("entry[2].origin = %q, want %q", got, want)
	}
	// Empty path normalizes to "/".
	if got, want := compiled[1].pathPrefix, "/"; got != want {
		t.Errorf("entry[1].pathPrefix = %q, want %q", got, want)
	}
}

func TestCompileAllowList_RejectsMissingSchemeOrHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []AllowedURLEntry
		want    string
	}{
		{"empty URL", []AllowedURLEntry{{URL: ""}}, "empty URL"},
		{"no scheme", []AllowedURLEntry{{URL: "example.com/path"}}, "missing scheme"},
		{"no host", []AllowedURLEntry{{URL: "https:///path"}}, "missing host"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := compileAllowList(tc.entries)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestCompileAllowList_RejectsEncodedSlash(t *testing.T) {
	t.Parallel()
	cases := []string{
		"https://api.example.com/v1%2fadmin",
		"https://api.example.com/v1%2Fadmin",
		"https://api.example.com/v1%5cadmin",
		"https://api.example.com/v1%5Cadmin",
	}
	for _, urlStr := range cases {
		urlStr := urlStr
		t.Run(urlStr, func(t *testing.T) {
			t.Parallel()
			_, err := compileAllowList([]AllowedURLEntry{{URL: urlStr}})
			if err == nil {
				t.Fatalf("expected error for %q, got nil", urlStr)
			}
			if !strings.Contains(err.Error(), "ambiguous") {
				t.Errorf("error = %q, want 'ambiguous'", err.Error())
			}
		})
	}
}

func TestAllowEntry_Matches(t *testing.T) {
	t.Parallel()
	compiled, err := compileAllowList([]AllowedURLEntry{
		{URL: "https://api.example.com/v1/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	e := compiled[0]
	cases := []struct {
		url  string
		want bool
	}{
		{"https://api.example.com/v1/users", true},
		{"https://api.example.com/v1/", true},
		{"https://api.example.com/v2/users", false},
		{"https://api.example.com/", false},
		{"http://api.example.com/v1/users", false},  // wrong scheme
		{"https://other.example.com/v1/users", false}, // wrong host
		{"https://API.EXAMPLE.COM/v1/users", true},   // case-insensitive host
		{"https://api.example.com:443/v1/users", true}, // default port
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if got := e.matches(u); got != tc.want {
				t.Errorf("matches(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestConfig_WithDefaults(t *testing.T) {
	t.Parallel()
	c := (&Config{}).withDefaults()
	if c.MaxRedirects != DefaultMaxRedirects {
		t.Errorf("MaxRedirects = %d, want %d", c.MaxRedirects, DefaultMaxRedirects)
	}
	if c.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", c.Timeout, DefaultTimeout)
	}
	if c.MaxResponseSize != DefaultMaxResponseSize {
		t.Errorf("MaxResponseSize = %d, want %d", c.MaxResponseSize, DefaultMaxResponseSize)
	}
	if len(c.AllowedMethods) != 2 || c.AllowedMethods[0] != "GET" || c.AllowedMethods[1] != "HEAD" {
		t.Errorf("AllowedMethods = %v, want [GET HEAD]", c.AllowedMethods)
	}

	// Negative sentinels preserved.
	in := &Config{MaxRedirects: -1, Timeout: -1, MaxResponseSize: -1}
	out := in.withDefaults()
	if out.MaxRedirects != -1 || out.Timeout != -1 || out.MaxResponseSize != -1 {
		t.Errorf("negative sentinels mutated: %+v", out)
	}
}

func TestErrorMessages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want string
	}{
		{&NetworkAccessDeniedError{URL: "https://x"}, "network access denied: https://x"},
		{&NetworkAccessDeniedError{URL: "https://x", Reason: "private"}, "network access denied: https://x: private"},
		{&TooManyRedirectsError{Max: 5}, "too many redirects: max=5"},
		{&RedirectNotAllowedError{From: "a", To: "b"}, "redirect not allowed: a -> b"},
		{&MethodNotAllowedError{Method: "POST", URL: "u"}, "method not allowed: POST u"},
		{&ResponseTooLargeError{Max: 100}, "response too large: max=100 bytes"},
	}
	for _, tc := range cases {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestErrorsAsRoundTrip(t *testing.T) {
	t.Parallel()
	var nad *NetworkAccessDeniedError
	if !errors.As(&NetworkAccessDeniedError{URL: "x"}, &nad) || nad.URL != "x" {
		t.Errorf("errors.As did not unwrap")
	}
}

func TestNewSecureFetch_CompileErrorSurfacesOnDo(t *testing.T) {
	t.Parallel()
	d := NewSecureFetch(&Config{
		AllowedURLPrefixes: []AllowedURLEntry{{URL: "not-a-url"}}, // no scheme
	})
	req := mustReq(t, "GET", "https://api.example.com/")
	_, err := d.Do(context.Background(), req)
	if err == nil {
		t.Fatalf("expected compile error to surface on Do")
	}
	if !strings.Contains(err.Error(), "missing scheme") {
		t.Errorf("err = %v, want missing scheme", err)
	}
}
