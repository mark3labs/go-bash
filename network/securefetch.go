package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewSecureFetch returns a Doer that enforces cfg. Passing nil
// produces a fully-locked-down Doer that denies every URL — this is
// the policy applied when BashOptions.Network is nil and the host
// did not supply BashOptions.Fetch either.
//
// The returned Doer is safe for concurrent use by multiple goroutines.
//
// The spec
func NewSecureFetch(cfg *Config) Doer {
	resolved := (&Config{}).withDefaults()
	if cfg != nil {
		resolved = cfg.withDefaults()
	}
	compiled, compileErr := compileAllowList(resolved.AllowedURLPrefixes)
	sf := &secureFetch{
		cfg:        resolved,
		compiled:   compiled,
		compileErr: compileErr,
	}
	sf.client = sf.buildClient()
	return sf
}

type secureFetch struct {
	cfg        Config
	compiled   []allowEntry
	compileErr error // surfaced on first Do call
	client     *http.Client
}

// buildClient constructs the http.Client used for every Do call. We
// scope it to one Doer instance so the redirect policy (which needs
// access to sf.cfg) can be a closure.
func (sf *secureFetch) buildClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: sf.dialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{
		Transport:     &headerTransform{base: transport, sf: sf},
		CheckRedirect: sf.checkRedirect,
	}
	return client
}

// Do implements Doer. It validates the request URL and method against
// the allow-list, applies the per-request timeout (if any), runs the
// transport, reads the body (subject to MaxResponseSize), and packs
// everything into a Response value.
func (sf *secureFetch) Do(ctx context.Context, req *http.Request) (*Response, error) {
	if sf.compileErr != nil {
		return nil, sf.compileErr
	}
	if req == nil || req.URL == nil {
		return nil, errors.New("network: nil request or URL")
	}
	if err := sf.checkMethod(req.Method, req.URL.String()); err != nil {
		return nil, err
	}
	if err := sf.checkURL(req.URL); err != nil {
		return nil, err
	}

	// Per-request timeout: if positive, wrap ctx; otherwise pass through.
	if sf.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sf.cfg.Timeout)
		defer cancel()
	}
	req = req.Clone(ctx)

	httpResp, err := sf.client.Do(req)
	if err != nil {
		// Unwrap our typed errors when http.Client wraps them in *url.Error.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			if typed := unwrapTypedNetErr(urlErr.Err); typed != nil {
				return nil, typed
			}
		}
		return nil, err
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := sf.readBody(httpResp.Body)
	if err != nil {
		return nil, err
	}

	finalURL := req.URL.String()
	if httpResp.Request != nil && httpResp.Request.URL != nil {
		finalURL = httpResp.Request.URL.String()
	}

	return &Response{
		Status:     httpResp.StatusCode,
		StatusText: httpResp.Status[strings.IndexByte(httpResp.Status, ' ')+1:],
		Headers:    httpResp.Header,
		Body:       body,
		URL:        finalURL,
	}, nil
}

// readBody reads at most MaxResponseSize+1 bytes from r. If the read
// returns more than MaxResponseSize bytes it fires ResponseTooLargeError.
// A negative MaxResponseSize disables the cap.
func (sf *secureFetch) readBody(r io.Reader) ([]byte, error) {
	if sf.cfg.MaxResponseSize < 0 {
		return io.ReadAll(r)
	}
	limit := sf.cfg.MaxResponseSize
	limited := io.LimitReader(r, limit+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > limit {
		return nil, &ResponseTooLargeError{Max: limit}
	}
	return buf, nil
}

// checkMethod validates the request method against AllowedMethods.
// DangerouslyAllowFullAccess bypasses the check.
func (sf *secureFetch) checkMethod(method, urlStr string) error {
	if sf.cfg.DangerouslyAllowFullAccess {
		return nil
	}
	if method == "" {
		method = http.MethodGet
	}
	for _, m := range sf.cfg.AllowedMethods {
		if strings.EqualFold(m, method) {
			return nil
		}
	}
	return &MethodNotAllowedError{Method: method, URL: urlStr}
}

// checkURL validates u against the allow-list. The check happens on
// every hop (initial request + each redirect), so a redirect into a
// non-allowed origin fails RedirectNotAllowedError from the
// CheckRedirect path; this method is the underlying yes/no.
func (sf *secureFetch) checkURL(u *url.URL) error {
	if sf.cfg.DangerouslyAllowFullAccess {
		return nil
	}
	if _, ok := findMatch(sf.compiled, u); !ok {
		return &NetworkAccessDeniedError{URL: u.String()}
	}
	return nil
}

// checkRedirect is the http.Client.CheckRedirect closure. It bounds
// the redirect count and re-validates the destination URL against
// the allow-list. The host's redirect cap maps directly to len(via)
// — http.Client passes us the chain of previously-followed requests.
func (sf *secureFetch) checkRedirect(req *http.Request, via []*http.Request) error {
	if sf.cfg.MaxRedirects < 0 {
		return http.ErrUseLastResponse
	}
	if len(via) > sf.cfg.MaxRedirects {
		return &TooManyRedirectsError{Max: sf.cfg.MaxRedirects}
	}
	// Re-validate method (in case the redirect changed it) and URL.
	if err := sf.checkMethod(req.Method, req.URL.String()); err != nil {
		return err
	}
	if sf.cfg.DangerouslyAllowFullAccess {
		return nil
	}
	if _, ok := findMatch(sf.compiled, req.URL); !ok {
		from := ""
		if n := len(via); n > 0 && via[n-1].URL != nil {
			from = via[n-1].URL.String()
		}
		return &RedirectNotAllowedError{From: from, To: req.URL.String()}
	}
	return nil
}

// dialContext is the transport's DialContext. When DenyPrivateRanges
// is enabled, every connection target is resolved via Config.DNSResolve
// (or net.DefaultResolver) and rejected if every address is in a
// private/loopback/link-local/multicast range.
//
// When DenyPrivateRanges is off we still go through this hook so
// host/port parsing stays consistent across modes.
func (sf *secureFetch) dialContext(ctx context.Context, network_, addr string) (net.Conn, error) {
	if !sf.cfg.DenyPrivateRanges {
		var d net.Dialer
		return d.DialContext(ctx, network_, addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	addrs, err := sf.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	publicAddrs := make([]string, 0, len(addrs))
	for _, a := range addrs {
		ip := net.ParseIP(a.Address)
		if ip == nil {
			continue
		}
		if isPrivateIP(ip) {
			continue
		}
		publicAddrs = append(publicAddrs, a.Address)
	}
	if len(publicAddrs) == 0 {
		return nil, &NetworkAccessDeniedError{
			URL:    fmt.Sprintf("%s://%s", "tcp", addr),
			Reason: "resolves to private/loopback/link-local address",
		}
	}
	var d net.Dialer
	// Try public addresses in order; the first to dial wins.
	var lastErr error
	for _, a := range publicAddrs {
		conn, err := d.DialContext(ctx, network_, net.JoinHostPort(a, port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (sf *secureFetch) resolve(ctx context.Context, host string) ([]DNSLookupResult, error) {
	// If host is already an IP literal, short-circuit.
	if ip := net.ParseIP(host); ip != nil {
		family := 4
		if ip.To4() == nil {
			family = 6
		}
		return []DNSLookupResult{{Address: ip.String(), Family: family}}, nil
	}
	if sf.cfg.DNSResolve != nil {
		return sf.cfg.DNSResolve(ctx, host)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	out := make([]DNSLookupResult, 0, len(ips))
	for _, ip := range ips {
		family := 4
		if ip.To4() == nil {
			family = 6
		}
		out = append(out, DNSLookupResult{Address: ip.String(), Family: family})
	}
	return out, nil
}

// isPrivateIP returns true for every IP range we want to block when
// DenyPrivateRanges is enabled: loopback, link-local, multicast,
// unspecified, and RFC1918 / RFC4193 private space.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}
	return false
}

// headerTransform is the per-request RoundTripper that applies
// Config.AllowedURLPrefixes[i].Transform entries to the outgoing
// request. It wraps the underlying transport so the rewrite happens
// AFTER http.Client has set its own default headers (Host, etc.).
type headerTransform struct {
	base http.RoundTripper
	sf   *secureFetch
}

func (h *headerTransform) RoundTrip(req *http.Request) (*http.Response, error) {
	entry, ok := findMatch(h.sf.compiled, req.URL)
	if ok {
		applyTransforms(req, entry.transforms)
	}
	return h.base.RoundTrip(req)
}

func applyTransforms(req *http.Request, transforms []RequestTransform) {
	for _, t := range transforms {
		for k, v := range t.Headers {
			// http.Header.Set already canonicalizes the key; passing
			// the raw input is correct and avoids the S1035 lint.
			req.Header.Set(k, v)
		}
	}
}

// unwrapTypedNetErr digs through wrapped errors to find one of our
// typed network errors. http.Client wraps every transport error in
// *url.Error, which masks the typed error from errors.As at the
// caller layer; we unwrap once here so SecureFetch.Do returns the
// raw typed error for direct comparison.
func unwrapTypedNetErr(err error) error {
	var (
		nad *NetworkAccessDeniedError
		tmr *TooManyRedirectsError
		rna *RedirectNotAllowedError
		mna *MethodNotAllowedError
		rtl *ResponseTooLargeError
	)
	switch {
	case errors.As(err, &nad):
		return nad
	case errors.As(err, &tmr):
		return tmr
	case errors.As(err, &rna):
		return rna
	case errors.As(err, &mna):
		return mna
	case errors.As(err, &rtl):
		return rtl
	}
	return nil
}
