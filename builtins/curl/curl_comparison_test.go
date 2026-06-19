package curl_test

import (
	"context"
	"net/http"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	_ "github.com/mark3labs/go-bash/builtins/curl"
	"github.com/mark3labs/go-bash/internal/cmpfixture"
	"github.com/mark3labs/go-bash/network"
)

// fixtureDoer returns a canned 200 response for any request. It is
// injected via BashOptions.Fetch so the comparison harness never
// touches the host network (GOBASH_TEST_NO_NETWORK=1 in CI).
type fixtureDoer struct{}

func (fixtureDoer) Do(_ context.Context, req *http.Request) (*network.Response, error) {
	return &network.Response{
		Status:     200,
		StatusText: "OK",
		Headers: http.Header{
			"Content-Type": []string{"text/plain"},
		},
		Body: []byte("hello from stub\n"),
		URL:  req.URL.String(),
	}, nil
}

// TestComparisonFixtures runs every JSON fixture under
// internal/testdata/fixtures/curl/. The standard cmpfixture.RunDir
// helper doesn't know about BashOptions.Fetch, so we re-implement the
// loop in-package with a stub Doer plumbed through.
func TestComparisonFixtures(t *testing.T) {
	const dir = "../../internal/testdata/fixtures/curl"
	entries, err := readJSONNames(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, name := range entries {
		name := name
		t.Run(name, func(t *testing.T) {
			fx, err := cmpfixture.Load(dir + "/" + name + ".json")
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			b, err := gobash.New(gobash.BashOptions{
				Cwd:   fx.Cwd,
				Env:   fx.Env,
				Fetch: fixtureDoer{},
			})
			if err != nil {
				t.Fatalf("gobash.New: %v", err)
			}
			res, err := b.Exec(context.Background(), fx.Script, gobash.ExecOptions{})
			if err != nil {
				t.Fatalf("Exec: %v", err)
			}
			if res.Stdout != fx.Expected.Stdout {
				t.Errorf("stdout mismatch\n got: %q\nwant: %q", res.Stdout, fx.Expected.Stdout)
			}
			if res.Stderr != fx.Expected.Stderr {
				t.Errorf("stderr mismatch\n got: %q\nwant: %q", res.Stderr, fx.Expected.Stderr)
			}
			if res.ExitCode != fx.Expected.ExitCode {
				t.Errorf("exitCode mismatch\n got: %d\nwant: %d", res.ExitCode, fx.Expected.ExitCode)
			}
		})
	}
}
