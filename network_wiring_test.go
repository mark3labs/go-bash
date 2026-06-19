package gobash_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/network"
)

// fetchProbeCommand is a sample command that records the Fetch field
// it receives on Context, optionally issues a request, and writes a
// summary to c.Stdout. The test asserts on the captured state.
func fetchProbeCommand(captured **command.Context) command.Command {
	return command.Define("fetch-probe", func(ctx context.Context, args []string, c *command.Context) command.Result {
		*captured = c
		if c.Fetch == nil {
			_, _ = io.WriteString(c.Stdout, "no-fetch\n")
			return command.Result{}
		}
		req, err := http.NewRequest("GET", args[1], nil)
		if err != nil {
			return command.Result{Stderr: err.Error() + "\n", ExitCode: 1}
		}
		resp, err := c.Fetch.Do(ctx, req)
		if err != nil {
			return command.Result{Stderr: err.Error() + "\n", ExitCode: 1}
		}
		_, _ = io.WriteString(c.Stdout, "status="+statusString(resp.Status)+"\n")
		_, _ = io.WriteString(c.Stdout, "body="+string(resp.Body)+"\n")
		return command.Result{}
	})
}

func statusString(code int) string {
	switch code {
	case 200:
		return "200"
	case 404:
		return "404"
	default:
		return "other"
	}
}

func TestPhase9FetchNilByDefault(t *testing.T) {
	t.Parallel()
	var captured *command.Context
	b, err := gobash.New(gobash.BashOptions{
		CustomCommands: []command.Command{fetchProbeCommand(&captured)},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), "fetch-probe", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "no-fetch\n" {
		t.Errorf("stdout = %q, want no-fetch\\n", res.Stdout)
	}
	if captured == nil {
		t.Fatal("command never ran")
	}
	if captured.Fetch != nil {
		t.Errorf("captured.Fetch = %v, want nil", captured.Fetch)
	}
}

func TestPhase9CustomFetchOverride(t *testing.T) {
	t.Parallel()
	// A trivial stub Doer that returns a fixed response.
	stub := &stubDoer{
		resp: &network.Response{
			Status: 200,
			Body:   []byte("from-stub"),
		},
	}
	var captured *command.Context
	b, err := gobash.New(gobash.BashOptions{
		Fetch:          stub,
		CustomCommands: []command.Command{fetchProbeCommand(&captured)},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), "fetch-probe https://anywhere/", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !strings.Contains(res.Stdout, "status=200") || !strings.Contains(res.Stdout, "body=from-stub") {
		t.Errorf("stdout = %q, want status=200 and body=from-stub", res.Stdout)
	}
	if stub.called != 1 {
		t.Errorf("stub.called = %d, want 1", stub.called)
	}
	if captured.Fetch != stub {
		t.Errorf("captured.Fetch != stub Doer")
	}
}

func TestPhase9NetworkConfigBuildsSecureFetch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "real")
	}))
	t.Cleanup(srv.Close)

	var captured *command.Context
	b, err := gobash.New(gobash.BashOptions{
		Network: &network.Config{
			AllowedURLPrefixes: []network.AllowedURLEntry{{URL: srv.URL}},
		},
		CustomCommands: []command.Command{fetchProbeCommand(&captured)},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), "fetch-probe "+srv.URL+"/", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !strings.Contains(res.Stdout, "status=200") || !strings.Contains(res.Stdout, "body=real") {
		t.Errorf("stdout = %q, want status=200 and body=real", res.Stdout)
	}
	if captured.Fetch == nil {
		t.Errorf("captured.Fetch is nil; expected SecureFetch")
	}
}

func TestPhase9NetworkDeniesUnlistedURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "real")
	}))
	t.Cleanup(srv.Close)

	var captured *command.Context
	b, err := gobash.New(gobash.BashOptions{
		Network: &network.Config{
			AllowedURLPrefixes: []network.AllowedURLEntry{{URL: srv.URL + "/api/"}},
		},
		CustomCommands: []command.Command{fetchProbeCommand(&captured)},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), "fetch-probe "+srv.URL+"/forbidden", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "network access denied") {
		t.Errorf("stderr = %q, want 'network access denied'", res.Stderr)
	}
}

func TestPhase9FetchOverridesNetwork(t *testing.T) {
	t.Parallel()
	// Both Fetch (stub) and Network (real config) set — Fetch wins.
	stub := &stubDoer{
		resp: &network.Response{Status: 200, Body: []byte("stub-wins")},
	}
	var captured *command.Context
	b, err := gobash.New(gobash.BashOptions{
		Fetch: stub,
		Network: &network.Config{
			AllowedURLPrefixes: []network.AllowedURLEntry{{URL: "https://nope"}},
		},
		CustomCommands: []command.Command{fetchProbeCommand(&captured)},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), "fetch-probe https://api.example.com/", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !strings.Contains(res.Stdout, "body=stub-wins") {
		t.Errorf("stdout = %q, want body=stub-wins", res.Stdout)
	}
	if captured.Fetch != stub {
		t.Errorf("captured.Fetch != stub; Fetch did not override Network")
	}
}

// stubDoer is a simple Doer for plumbing tests.
type stubDoer struct {
	called int
	resp   *network.Response
	err    error
}

func (s *stubDoer) Do(ctx context.Context, req *http.Request) (*network.Response, error) {
	s.called++
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

// Sanity: typed errors unwrap through Exec when surfaced via stderr.
func TestPhase9TypedErrorsRoutable(t *testing.T) {
	t.Parallel()
	stub := &stubDoer{err: &network.NetworkAccessDeniedError{URL: "https://x"}}
	cmd := command.Define("probe", func(ctx context.Context, args []string, c *command.Context) command.Result {
		req, _ := http.NewRequest("GET", "https://x", nil)
		_, err := c.Fetch.Do(ctx, req)
		var nad *network.NetworkAccessDeniedError
		if !errors.As(err, &nad) {
			return command.Result{Stderr: "wrong err: " + err.Error() + "\n", ExitCode: 2}
		}
		return command.Result{Stdout: "matched\n"}
	})
	b, err := gobash.New(gobash.BashOptions{
		Fetch:          stub,
		CustomCommands: []command.Command{cmd},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(), "probe", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "matched\n" {
		t.Errorf("stdout = %q, want matched", res.Stdout)
	}
}
