package curl_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/curl"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/fs/memfs"
	"github.com/mark3labs/go-bash/network"
)

// stubDoer records the last request and returns a canned response.
type stubDoer struct {
	resp    *network.Response
	err     error
	gotReq  *http.Request
	gotBody []byte
}

func (s *stubDoer) Do(_ context.Context, req *http.Request) (*network.Response, error) {
	s.gotReq = req
	if req.Body != nil {
		s.gotBody, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.resp == nil {
		return &network.Response{
			Status: 200, StatusText: "OK",
			Headers: http.Header{"Content-Type": []string{"text/plain"}},
			Body:    []byte("ok"),
			URL:     req.URL.String(),
		}, nil
	}
	return s.resp, nil
}

func runCmd(t *testing.T, c *command.Context, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	if c == nil {
		c = &command.Context{}
	}
	if c.FS == nil {
		c.FS = memfs.New()
	}
	c.Stdout = &o
	c.Stderr = &e
	res := curl.New().Execute(context.Background(), append([]string{"curl"}, args...), c)
	return o.String(), e.String(), res.ExitCode
}

func TestCurlNoFetch(t *testing.T) {
	_, e, code := runCmd(t, nil, "https://example.com/")
	if code != 2 {
		t.Errorf("code=%d want 2", code)
	}
	if !strings.Contains(e, "network disabled") {
		t.Errorf("stderr=%q", e)
	}
}

func TestCurlNoURL(t *testing.T) {
	c := &command.Context{Fetch: &stubDoer{}}
	_, e, code := runCmd(t, c)
	if code != 2 || !strings.Contains(e, "no URL") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestCurlGETBody(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	out, _, code := runCmd(t, c, "https://example.com/x")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "ok" {
		t.Errorf("out=%q", out)
	}
	if s.gotReq.Method != http.MethodGet {
		t.Errorf("method=%s", s.gotReq.Method)
	}
	if s.gotReq.URL.String() != "https://example.com/x" {
		t.Errorf("url=%s", s.gotReq.URL)
	}
}

func TestCurlDashXMethod(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, code := runCmd(t, c, "-X", "DELETE", "https://example.com/x")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if s.gotReq.Method != http.MethodDelete {
		t.Errorf("method=%s", s.gotReq.Method)
	}
}

func TestCurlDashH(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, _ = runCmd(t, c, "-H", "X-Foo: bar", "-H", "X-Bar: baz", "https://example.com/")
	if s.gotReq.Header.Get("X-Foo") != "bar" {
		t.Errorf("X-Foo=%q", s.gotReq.Header.Get("X-Foo"))
	}
	if s.gotReq.Header.Get("X-Bar") != "baz" {
		t.Errorf("X-Bar=%q", s.gotReq.Header.Get("X-Bar"))
	}
}

func TestCurlDashD(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, _ = runCmd(t, c, "-d", "a=1", "https://example.com/")
	if s.gotReq.Method != http.MethodPost {
		t.Errorf("method=%s want POST", s.gotReq.Method)
	}
	if string(s.gotBody) != "a=1" {
		t.Errorf("body=%q", s.gotBody)
	}
	if ct := s.gotReq.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Errorf("ct=%q", ct)
	}
}

func TestCurlDataBinary(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, _ = runCmd(t, c, "--data-binary", "{\"k\":1}", "-H", "Content-Type: application/json", "https://example.com/")
	if string(s.gotBody) != `{"k":1}` {
		t.Errorf("body=%q", s.gotBody)
	}
	if ct := s.gotReq.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("ct=%q", ct)
	}
}

func TestCurlDataURLEncode(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, _ = runCmd(t, c, "--data-urlencode", "q=hello world", "https://example.com/")
	if string(s.gotBody) != "q=hello+world" {
		t.Errorf("body=%q", s.gotBody)
	}
}

func TestCurlDashG(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, _ = runCmd(t, c, "-G", "-d", "q=cats", "https://example.com/search")
	if s.gotReq.Method != http.MethodGet {
		t.Errorf("method=%s", s.gotReq.Method)
	}
	if s.gotReq.URL.RawQuery != "q=cats" {
		t.Errorf("query=%q", s.gotReq.URL.RawQuery)
	}
}

func TestCurlDashO(t *testing.T) {
	s := &stubDoer{
		resp: &network.Response{
			Status: 200, StatusText: "OK",
			Headers: http.Header{}, Body: []byte("payload"),
		},
	}
	fs := memfs.New()
	c := &command.Context{Fetch: s, FS: fs, Cwd: "/tmp"}
	_ = fs.MkdirAll("/tmp", 0o755)
	_, _, code := runCmd(t, c, "-o", "out.txt", "https://example.com/")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, err := fs.ReadFile("/tmp/out.txt")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "payload" {
		t.Errorf("content=%q", data)
	}
}

func TestCurlBigO(t *testing.T) {
	s := &stubDoer{
		resp: &network.Response{
			Status: 200, StatusText: "OK",
			Headers: http.Header{}, Body: []byte("hello"),
		},
	}
	fs := memfs.New()
	_ = fs.MkdirAll("/tmp", 0o755)
	c := &command.Context{Fetch: s, FS: fs, Cwd: "/tmp"}
	_, _, code := runCmd(t, c, "-O", "https://example.com/path/file.txt")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, _ := fs.ReadFile("/tmp/file.txt")
	if string(data) != "hello" {
		t.Errorf("content=%q", data)
	}
}

func TestCurlInclude(t *testing.T) {
	s := &stubDoer{
		resp: &network.Response{
			Status: 200, StatusText: "OK",
			Headers: http.Header{"Content-Type": []string{"text/plain"}},
			Body:    []byte("body"),
		},
	}
	c := &command.Context{Fetch: s}
	out, _, _ := runCmd(t, c, "-i", "https://example.com/")
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("out=%q", out)
	}
	if !strings.Contains(out, "Content-Type: text/plain") {
		t.Errorf("out=%q", out)
	}
	if !strings.HasSuffix(out, "\r\nbody") {
		t.Errorf("out=%q", out)
	}
}

func TestCurlHead(t *testing.T) {
	s := &stubDoer{
		resp: &network.Response{
			Status: 200, StatusText: "OK",
			Headers: http.Header{"Content-Type": []string{"text/plain"}},
			Body:    []byte("ignored-body"),
		},
	}
	c := &command.Context{Fetch: s}
	out, _, _ := runCmd(t, c, "-I", "https://example.com/")
	if s.gotReq.Method != http.MethodHead {
		t.Errorf("method=%s", s.gotReq.Method)
	}
	if strings.Contains(out, "ignored-body") {
		t.Errorf("body leaked: out=%q", out)
	}
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("out=%q", out)
	}
}

func TestCurlUserBasicAuth(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, _ = runCmd(t, c, "-u", "alice:secret", "https://example.com/")
	got := s.gotReq.Header.Get("Authorization")
	if got != "Basic YWxpY2U6c2VjcmV0" {
		t.Errorf("authz=%q", got)
	}
}

func TestCurlUserAgentAndReferer(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, _ = runCmd(t, c, "-A", "agent/1", "-e", "https://ref/", "https://example.com/")
	if s.gotReq.Header.Get("User-Agent") != "agent/1" {
		t.Errorf("ua=%q", s.gotReq.Header.Get("User-Agent"))
	}
	if s.gotReq.Header.Get("Referer") != "https://ref/" {
		t.Errorf("ref=%q", s.gotReq.Header.Get("Referer"))
	}
}

func TestCurlFailFlag(t *testing.T) {
	s := &stubDoer{resp: &network.Response{Status: 404, StatusText: "Not Found", Headers: http.Header{}, Body: []byte("oops")}}
	c := &command.Context{Fetch: s}
	out, e, code := runCmd(t, c, "-f", "https://example.com/")
	if code != 22 {
		t.Errorf("code=%d want 22", code)
	}
	if strings.Contains(out, "oops") {
		t.Errorf("body leaked: %q", out)
	}
	if !strings.Contains(e, "22") {
		t.Errorf("stderr=%q", e)
	}
}

func TestCurlSilentNoErr(t *testing.T) {
	s := &stubDoer{resp: &network.Response{Status: 404, StatusText: "Not Found", Headers: http.Header{}, Body: []byte("x")}}
	c := &command.Context{Fetch: s}
	_, e, code := runCmd(t, c, "-s", "-f", "https://example.com/")
	if code != 22 {
		t.Errorf("code=%d", code)
	}
	if e != "" {
		t.Errorf("stderr=%q want empty (silent)", e)
	}
}

func TestCurlSilentShowError(t *testing.T) {
	s := &stubDoer{resp: &network.Response{Status: 404, StatusText: "Not Found", Headers: http.Header{}, Body: []byte("x")}}
	c := &command.Context{Fetch: s}
	_, e, _ := runCmd(t, c, "-s", "-S", "-f", "https://example.com/")
	if !strings.Contains(e, "22") {
		t.Errorf("stderr=%q want 22 message via -S", e)
	}
}

func TestCurlWriteOut(t *testing.T) {
	s := &stubDoer{
		resp: &network.Response{Status: 201, StatusText: "Created", Headers: http.Header{"Content-Type": []string{"x/y"}}, Body: []byte("abcde"), URL: "https://example.com/final"},
	}
	c := &command.Context{Fetch: s}
	out, _, _ := runCmd(t, c, "-w", `%{http_code} %{size_download} %{content_type} %{url_effective}\n`, "https://example.com/")
	if !strings.HasSuffix(out, "201 5 x/y https://example.com/final\n") {
		t.Errorf("out=%q", out)
	}
}

func TestCurlForm(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, _ = runCmd(t, c, "-F", "name=alice", "-F", "color=blue", "https://example.com/")
	if s.gotReq.Method != http.MethodPost {
		t.Errorf("method=%s", s.gotReq.Method)
	}
	if !strings.HasPrefix(s.gotReq.Header.Get("Content-Type"), "multipart/form-data;") {
		t.Errorf("ct=%q", s.gotReq.Header.Get("Content-Type"))
	}
	if !bytes.Contains(s.gotBody, []byte(`name="name"`)) || !bytes.Contains(s.gotBody, []byte("alice")) {
		t.Errorf("body=%q", s.gotBody)
	}
}

func TestCurlMaxTime(t *testing.T) {
	// Parser-level check: --max-time must accept "5" and "200ms".
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, code := runCmd(t, c, "--max-time", "5", "https://example.com/")
	if code != 0 {
		t.Errorf("code=%d", code)
	}
	_, _, code = runCmd(t, c, "--max-time", "200ms", "https://example.com/")
	if code != 0 {
		t.Errorf("code=%d", code)
	}
}

func TestCurlInsecureNoOp(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, code := runCmd(t, c, "-k", "https://example.com/")
	if code != 0 {
		t.Errorf("code=%d", code)
	}
}

func TestCurlFollowFlagNoOp(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, code := runCmd(t, c, "-L", "https://example.com/")
	if code != 0 {
		t.Errorf("code=%d", code)
	}
}

func TestCurlDoerError(t *testing.T) {
	s := &stubDoer{err: &network.NetworkAccessDeniedError{URL: "https://nope"}}
	c := &command.Context{Fetch: s}
	_, e, code := runCmd(t, c, "https://nope")
	if code != 6 {
		t.Errorf("code=%d want 6", code)
	}
	if !strings.Contains(e, "network access denied") {
		t.Errorf("stderr=%q", e)
	}
}

func TestCurlHelp(t *testing.T) {
	out, _, code := runCmd(t, nil, "--help")
	if code != 0 || !strings.Contains(out, "Usage: curl") {
		t.Errorf("help out=%q code=%d", out, code)
	}
}

func TestCurlUnknownOption(t *testing.T) {
	c := &command.Context{Fetch: &stubDoer{}}
	_, e, code := runCmd(t, c, "--bogus")
	if code != 2 || !strings.Contains(e, "usage:") {
		t.Errorf("code=%d stderr=%q", code, e)
	}
}

func TestCurlDashDash(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	_, _, code := runCmd(t, c, "--", "https://example.com/?q=1")
	if code != 0 {
		t.Errorf("code=%d", code)
	}
	if s.gotReq.URL.String() != "https://example.com/?q=1" {
		t.Errorf("url=%s", s.gotReq.URL)
	}
}

func TestCurlMultipleURLs(t *testing.T) {
	s := &stubDoer{}
	c := &command.Context{Fetch: s}
	out, _, code := runCmd(t, c, "https://a.example/", "https://b.example/")
	if code != 0 {
		t.Errorf("code=%d", code)
	}
	if out != "okok" {
		t.Errorf("out=%q", out)
	}
}
