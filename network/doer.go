package network

import (
	"context"
	"net/http"
)

// Doer is the gobash analog of an http.Client. Every network-touching
// built-in (Phase 10's curl, future fetch / wget) talks to a Doer,
// never directly to net/http. Hosts may supply their own
// implementation via BashOptions.Fetch to mock network traffic in
// tests or to route through a custom transport (proxy, mTLS, etc.).
//
// Implementations MUST respect ctx cancellation and MUST NOT mutate
// the supplied req. The returned Response is owned by the caller.
type Doer interface {
	Do(ctx context.Context, req *http.Request) (*Response, error)
}

// Response is the value type returned by Doer.Do. It is a snapshot:
// Body has already been read (subject to MaxResponseSize), so the
// caller does not need to Close anything. URL records the final URL
// after redirect resolution so commands like curl -w can report it.
type Response struct {
	// Status is the numeric HTTP status code (200, 404, etc.).
	Status int

	// StatusText is the HTTP status line text ("OK", "Not Found", ...).
	StatusText string

	// Headers is the response header set. Multi-value headers retain
	// every value. Callers MUST NOT mutate this map.
	Headers http.Header

	// Body is the response body bytes, capped at Config.MaxResponseSize.
	// If the server attempted to send more, Doer.Do returns
	// *ResponseTooLargeError and Response is nil.
	Body []byte

	// URL is the final URL after any redirect chain. For a non-redirected
	// response it equals the request URL string.
	URL string
}
