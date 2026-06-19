// Package curl implements the `curl` built-in.
//
// This is NOT a real curl(1); it is a thin wrapper around the
// network.Doer plumbed through Context.Fetch. All HTTP/HTTPS traffic
// goes through that Doer — the curl built-in itself never touches the
// network stack. When Context.Fetch is nil (host did not configure
// BashOptions.Network or BashOptions.Fetch), curl emits the
// "network disabled" diagnostic and exits with code 2.
//
// Supported flags:
//
//	-X METHOD               request method
//	-H 'K: V'               extra header (repeatable)
//	-d DATA                 request body (URL-encoded by default)
//	--data-binary DATA      request body verbatim
//	--data-urlencode DATA   request body, URL-encoded
//	-F NAME=VALUE           multipart/form-data field (repeatable)
//	-o FILE                 write body to FILE (use - for stdout)
//	-O                      write body to the URL's basename
//	-s, --silent            no progress / no error output
//	-S, --show-error        re-enable error output when used with -s
//	-L, --location          follow redirects (Doer policy decides)
//	-i, --include           include response headers in the output
//	-I, --head              issue a HEAD request, print headers only
//	-k, --insecure          permit TLS errors (no-op; Doer governs)
//	-u USER:PASS            HTTP Basic auth
//	-A AGENT                User-Agent header
//	-e REFERER              Referer header
//	-f, --fail              exit 22 on HTTP >= 400 without body output
//	--max-time DUR          per-request time cap (seconds, optional unit)
//	-w FORMAT               write-out format string (subset)
//	-G                      issue a GET, append -d data as the query string
//	--help                  show this help and exit
//
// `-w` placeholders implemented: %{http_code}, %{size_download},
// %{url_effective}, %{content_type}, %{response_code} (alias),
// %{num_headers}, and literal \n / \t escapes.
package curl

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
	"github.com/mark3labs/go-bash/network"
)

const usage = "curl [OPTIONS] URL..."

const helpText = `Usage: curl [OPTIONS] URL...
Issue HTTP/HTTPS requests via the host-supplied network Doer.

Options:
  -X METHOD                request method (default GET)
  -H 'K: V'                extra header (repeatable)
  -d DATA                  request body (form-encoded by default)
  --data-binary DATA       request body verbatim
  --data-urlencode DATA    request body, URL-encoded
  -F NAME=VALUE            multipart/form-data field (repeatable)
  -o FILE                  write body to FILE (use - for stdout)
  -O                       write body to the URL's basename
  -s, --silent             suppress error output
  -S, --show-error         re-enable error output with -s
  -L, --location           follow redirects (Doer policy applies)
  -i, --include            include response headers in output
  -I, --head               issue a HEAD request, print headers only
  -k, --insecure           permit TLS errors (no-op)
  -u USER:PASS             HTTP Basic auth
  -A AGENT                 User-Agent header
  -e REFERER               Referer header
  -f, --fail               exit 22 on HTTP >= 400 without body output
  --max-time DUR           per-request time cap (e.g. 30, 10s, 1m)
  -w FORMAT                write-out format string
  -G                       issue GET, append -d data as query
  --help                   show this help and exit
`

// New returns the curl command.
func New() command.Command { return command.Define("curl", run) }

// opts captures the parsed argv for a single curl invocation.
type opts struct {
	method         string
	headers        []string // raw "K: V" entries
	data           []string // -d (form)
	dataBinary     []string // --data-binary
	dataURLEncode  []string // --data-urlencode
	forms          []string // -F NAME=VALUE
	output         string
	useURLBasename bool
	silent         bool
	showError      bool
	follow         bool
	includeHdrs    bool
	headOnly       bool
	insecure       bool
	userpass       string
	userAgent      string
	referer        string
	failOnErr      bool
	maxTime        time.Duration
	writeOut       string
	getMode        bool
	urls           []string
}

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	o, perr := parseArgs(args[1:])
	if perr != nil {
		if perr == errHelp {
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		}
		return builtinutil.UsageError(c.Stderr, usage)
	}
	if len(o.urls) == 0 {
		return builtinutil.Errorf(c.Stderr, "curl", 2, "no URL specified")
	}
	if c.Fetch == nil {
		return builtinutil.Errorf(c.Stderr, "curl", 2, "network disabled")
	}

	if o.maxTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.maxTime)
		defer cancel()
	}

	finalExit := 0
	for _, u := range o.urls {
		ec := doOne(ctx, o, u, c)
		if ec != 0 {
			finalExit = ec
		}
	}
	return command.Result{ExitCode: finalExit}
}

// doOne issues one request per URL and writes the result. Returns the
// curl-style exit code for this URL.
func doOne(ctx context.Context, o *opts, rawURL string, c *command.Context) int {
	req, body, err := buildRequest(o, rawURL)
	if err != nil {
		if !o.silent || o.showError {
			_, _ = fmt.Fprintf(c.Stderr, "curl: %v\n", err)
		}
		return 3 // URL malformed
	}

	resp, err := c.Fetch.Do(ctx, req)
	if err != nil {
		if !o.silent || o.showError {
			_, _ = fmt.Fprintf(c.Stderr, "curl: %v\n", err)
		}
		return 6 // could not resolve host (curl-ish)
	}

	// -f / --fail: server returned >=400 ⇒ exit 22, suppress body.
	if o.failOnErr && resp.Status >= 400 {
		if !o.silent || o.showError {
			_, _ = fmt.Fprintf(c.Stderr, "curl: (22) The requested URL returned error: %d\n", resp.Status)
		}
		// Still honor -w even on failure, matches curl behavior.
		writeOut(o.writeOut, resp, c.Stdout, body)
		return 22
	}

	// Pick output sink. Default = c.Stdout.
	sink := c.Stdout
	if o.output != "" && o.output != "-" {
		// Write through c.FS so the sandbox contract holds.
		target := builtinutil.ResolvePath(c.Cwd, o.output)
		if err := c.FS.WriteFile(target, assembleBody(resp, o), 0o644); err != nil {
			if !o.silent || o.showError {
				_, _ = fmt.Fprintf(c.Stderr, "curl: %v\n", err)
			}
			return 23 // write error
		}
		writeOut(o.writeOut, resp, c.Stdout, body)
		return 0
	}
	if o.useURLBasename {
		target := urlBasename(rawURL)
		if target == "" {
			target = "index.html"
		}
		target = builtinutil.ResolvePath(c.Cwd, target)
		if err := c.FS.WriteFile(target, assembleBody(resp, o), 0o644); err != nil {
			if !o.silent || o.showError {
				_, _ = fmt.Fprintf(c.Stderr, "curl: %v\n", err)
			}
			return 23
		}
		writeOut(o.writeOut, resp, c.Stdout, body)
		return 0
	}

	if sink == nil {
		// No stdout writer configured — nothing to do but honor -w.
		writeOut(o.writeOut, resp, c.Stdout, body)
		return 0
	}
	_, _ = sink.Write(assembleBody(resp, o))
	writeOut(o.writeOut, resp, c.Stdout, body)
	return 0
}

// assembleBody returns the bytes to render: optional response-header
// block (-i / -I) followed by the body (omitted for -I).
func assembleBody(r *network.Response, o *opts) []byte {
	var buf bytes.Buffer
	if o.includeHdrs || o.headOnly {
		_, _ = fmt.Fprintf(&buf, "HTTP/1.1 %d %s\r\n", r.Status, r.StatusText)
		writeHeaders(&buf, r.Headers)
		buf.WriteString("\r\n")
	}
	if !o.headOnly {
		buf.Write(r.Body)
	}
	return buf.Bytes()
}

func writeHeaders(buf *bytes.Buffer, h http.Header) {
	// Sort headers for stable output. http.Header is map-typed.
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	// stable sort via insertion
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	for _, k := range keys {
		for _, v := range h[k] {
			_, _ = fmt.Fprintf(buf, "%s: %s\r\n", k, v)
		}
	}
}

// buildRequest constructs the *http.Request for a single URL.
func buildRequest(o *opts, rawURL string) (*http.Request, []byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid URL: %v", err)
	}

	method := o.method
	hasBody := len(o.data) > 0 || len(o.dataBinary) > 0 || len(o.dataURLEncode) > 0 || len(o.forms) > 0

	if o.getMode && hasBody {
		// -G: append data as the query string and force GET.
		q := u.Query()
		for _, d := range o.data {
			parseQueryInto(q, d, false)
		}
		for _, d := range o.dataBinary {
			parseQueryInto(q, d, false)
		}
		for _, d := range o.dataURLEncode {
			parseQueryInto(q, d, true)
		}
		u.RawQuery = q.Encode()
		hasBody = false
		if method == "" {
			method = http.MethodGet
		}
	}

	if o.headOnly {
		method = http.MethodHead
	}
	if method == "" {
		if hasBody {
			method = http.MethodPost
		} else {
			method = http.MethodGet
		}
	}

	var (
		body        io.Reader
		bodyBytes   []byte
		contentType string
	)
	switch {
	case len(o.forms) > 0:
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for _, f := range o.forms {
			eq := strings.IndexByte(f, '=')
			if eq < 0 {
				return nil, nil, fmt.Errorf("invalid -F form spec %q", f)
			}
			name := f[:eq]
			value := f[eq+1:]
			if err := mw.WriteField(name, value); err != nil {
				return nil, nil, err
			}
		}
		_ = mw.Close()
		bodyBytes = buf.Bytes()
		contentType = mw.FormDataContentType()
		body = bytes.NewReader(bodyBytes)
	case hasBody:
		var buf bytes.Buffer
		isForm := len(o.data) > 0 || len(o.dataURLEncode) > 0
		first := true
		writeJoin := func(s string) {
			if !first {
				buf.WriteByte('&')
			}
			buf.WriteString(s)
			first = false
		}
		for _, d := range o.data {
			writeJoin(d)
		}
		for _, d := range o.dataURLEncode {
			writeJoin(encodeData(d))
		}
		for _, d := range o.dataBinary {
			if !first {
				buf.WriteByte('&')
			}
			buf.WriteString(d)
			first = false
		}
		bodyBytes = buf.Bytes()
		body = bytes.NewReader(bodyBytes)
		if isForm {
			contentType = "application/x-www-form-urlencoded"
		}
	}

	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, h := range o.headers {
		k, v, ok := splitHeader(h)
		if !ok {
			return nil, nil, fmt.Errorf("invalid header %q", h)
		}
		req.Header.Set(k, v)
	}
	if o.userAgent != "" {
		req.Header.Set("User-Agent", o.userAgent)
	}
	if o.referer != "" {
		req.Header.Set("Referer", o.referer)
	}
	if o.userpass != "" {
		token := base64.StdEncoding.EncodeToString([]byte(o.userpass))
		req.Header.Set("Authorization", "Basic "+token)
	}
	return req, bodyBytes, nil
}

// splitHeader parses "Key: Value" / "Key:" (drop) / "Key;" (set empty).
func splitHeader(h string) (string, string, bool) {
	if idx := strings.IndexByte(h, ':'); idx > 0 {
		k := strings.TrimSpace(h[:idx])
		v := strings.TrimSpace(h[idx+1:])
		return k, v, k != ""
	}
	if idx := strings.IndexByte(h, ';'); idx > 0 {
		k := strings.TrimSpace(h[:idx])
		return k, "", k != ""
	}
	return "", "", false
}

// encodeData URL-encodes the value half of a "name=value" pair (or
// the whole string if there is no '='). Matches curl's
// --data-urlencode semantics for the common forms.
func encodeData(d string) string {
	if eq := strings.IndexByte(d, '='); eq >= 0 {
		return d[:eq+1] + url.QueryEscape(d[eq+1:])
	}
	return url.QueryEscape(d)
}

// parseQueryInto splits "k=v" into the url.Values map; if encode is
// true the value is taken verbatim (it is already URL-encoded by the
// caller when needed), otherwise we let url.Values.Encode handle it.
func parseQueryInto(q url.Values, raw string, urlEncoded bool) {
	eq := strings.IndexByte(raw, '=')
	if eq < 0 {
		q.Add(raw, "")
		return
	}
	k := raw[:eq]
	v := raw[eq+1:]
	if urlEncoded {
		// caller has already escaped; decode for url.Values then
		// re-encode happens in q.Encode().
		dec, err := url.QueryUnescape(v)
		if err == nil {
			v = dec
		}
	}
	q.Add(k, v)
}

// urlBasename returns the last path segment of the URL, suitable for
// -O. Empty or trailing-slash paths return "".
func urlBasename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	p := u.Path
	if p == "" || strings.HasSuffix(p, "/") {
		return ""
	}
	return path.Base(p)
}

// writeOut renders curl's -w FORMAT string to stdout. Supports a
// pragmatic subset of the placeholders (curl's full list is huge).
func writeOut(format string, r *network.Response, stdout io.Writer, _ []byte) {
	if format == "" || stdout == nil || r == nil {
		return
	}
	out := format
	out = strings.ReplaceAll(out, `\n`, "\n")
	out = strings.ReplaceAll(out, `\t`, "\t")
	out = strings.ReplaceAll(out, `\r`, "\r")
	subs := map[string]string{
		"%{http_code}":     strconv.Itoa(r.Status),
		"%{response_code}": strconv.Itoa(r.Status),
		"%{size_download}": strconv.Itoa(len(r.Body)),
		"%{url_effective}": r.URL,
		"%{content_type}":  r.Headers.Get("Content-Type"),
		"%{num_headers}":   strconv.Itoa(countHeaderValues(r.Headers)),
	}
	for k, v := range subs {
		out = strings.ReplaceAll(out, k, v)
	}
	_, _ = io.WriteString(stdout, out)
}

func countHeaderValues(h http.Header) int {
	n := 0
	for _, vs := range h {
		n += len(vs)
	}
	return n
}

// errHelp is the sentinel returned by parseArgs when --help is seen.
var errHelp = fmt.Errorf("help")

func parseArgs(args []string) (*opts, error) {
	o := &opts{}
	i := 0
	requireArg := func(flag string) (string, error) {
		if i+1 >= len(args) {
			return "", fmt.Errorf("%s requires an argument", flag)
		}
		i++
		return args[i], nil
	}
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			return nil, errHelp
		case a == "--":
			i++
			o.urls = append(o.urls, args[i:]...)
			return o, nil
		case a == "-X" || a == "--request":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.method = v
		case strings.HasPrefix(a, "--request="):
			o.method = strings.TrimPrefix(a, "--request=")
		case a == "-H" || a == "--header":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.headers = append(o.headers, v)
		case strings.HasPrefix(a, "--header="):
			o.headers = append(o.headers, strings.TrimPrefix(a, "--header="))
		case a == "-d" || a == "--data" || a == "--data-ascii":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.data = append(o.data, v)
		case strings.HasPrefix(a, "--data="):
			o.data = append(o.data, strings.TrimPrefix(a, "--data="))
		case a == "--data-binary":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.dataBinary = append(o.dataBinary, v)
		case strings.HasPrefix(a, "--data-binary="):
			o.dataBinary = append(o.dataBinary, strings.TrimPrefix(a, "--data-binary="))
		case a == "--data-urlencode":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.dataURLEncode = append(o.dataURLEncode, v)
		case strings.HasPrefix(a, "--data-urlencode="):
			o.dataURLEncode = append(o.dataURLEncode, strings.TrimPrefix(a, "--data-urlencode="))
		case a == "-F" || a == "--form":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.forms = append(o.forms, v)
		case a == "-o" || a == "--output":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.output = v
		case a == "-O" || a == "--remote-name":
			o.useURLBasename = true
		case a == "-s" || a == "--silent":
			o.silent = true
		case a == "-S" || a == "--show-error":
			o.showError = true
		case a == "-L" || a == "--location":
			o.follow = true
		case a == "-i" || a == "--include":
			o.includeHdrs = true
		case a == "-I" || a == "--head":
			o.headOnly = true
		case a == "-k" || a == "--insecure":
			o.insecure = true
		case a == "-u" || a == "--user":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.userpass = v
		case a == "-A" || a == "--user-agent":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.userAgent = v
		case a == "-e" || a == "--referer":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.referer = v
		case a == "-f" || a == "--fail":
			o.failOnErr = true
		case a == "--max-time":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			d, err := parseDuration(v)
			if err != nil {
				return nil, err
			}
			o.maxTime = d
		case strings.HasPrefix(a, "--max-time="):
			d, err := parseDuration(strings.TrimPrefix(a, "--max-time="))
			if err != nil {
				return nil, err
			}
			o.maxTime = d
		case a == "-w" || a == "--write-out":
			v, err := requireArg(a)
			if err != nil {
				return nil, err
			}
			o.writeOut = v
		case a == "-G" || a == "--get":
			o.getMode = true
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return nil, fmt.Errorf("unknown option %q", a)
		default:
			o.urls = append(o.urls, a)
		}
	}
	return o, nil
}

// parseDuration accepts curl's --max-time form: a bare number is
// interpreted as seconds (possibly fractional); "5s" / "1m" / "2h" /
// "200ms" are accepted via time.ParseDuration. Negative values are
// rejected.
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		if v < 0 {
			return 0, fmt.Errorf("negative duration")
		}
		return time.Duration(v * float64(time.Second)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("negative duration")
	}
	return d, nil
}

func init() { command.RegisterBuiltin(New()) }
