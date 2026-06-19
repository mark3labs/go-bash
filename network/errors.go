package network

import "fmt"

// NetworkAccessDeniedError is returned when a URL does not match any
// allow-list entry (and DangerouslyAllowFullAccess is not set), or
// when an otherwise-allowed URL resolves only to private addresses
// while DenyPrivateRanges is enabled.
//
// SPEC §9.4.
type NetworkAccessDeniedError struct {
	URL    string
	Reason string
}

func (e *NetworkAccessDeniedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("network access denied: %s", e.URL)
	}
	return fmt.Sprintf("network access denied: %s: %s", e.URL, e.Reason)
}

// TooManyRedirectsError is returned when a request would exceed
// Config.MaxRedirects redirect hops.
//
// SPEC §9.4.
type TooManyRedirectsError struct {
	Max int
}

func (e *TooManyRedirectsError) Error() string {
	return fmt.Sprintf("too many redirects: max=%d", e.Max)
}

// RedirectNotAllowedError is returned when a redirect target fails
// allow-list re-validation. The two URLs are the source and the
// rejected destination, matching the TS error shape.
//
// SPEC §9.4.
type RedirectNotAllowedError struct {
	From string
	To   string
}

func (e *RedirectNotAllowedError) Error() string {
	return fmt.Sprintf("redirect not allowed: %s -> %s", e.From, e.To)
}

// MethodNotAllowedError is returned when the request's method is not
// in Config.AllowedMethods (and DangerouslyAllowFullAccess is false).
//
// SPEC §9.4.
type MethodNotAllowedError struct {
	Method string
	URL    string
}

func (e *MethodNotAllowedError) Error() string {
	return fmt.Sprintf("method not allowed: %s %s", e.Method, e.URL)
}

// ResponseTooLargeError is returned when a response body exceeds
// Config.MaxResponseSize bytes. The body is not buffered into
// Response.Body when this error fires.
//
// SPEC §9.4.
type ResponseTooLargeError struct {
	Max int64
}

func (e *ResponseTooLargeError) Error() string {
	return fmt.Sprintf("response too large: max=%d bytes", e.Max)
}
