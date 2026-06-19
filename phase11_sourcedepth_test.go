package gobash_test

import (
	"context"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	_ "github.com/mark3labs/go-bash/builtins"
	"github.com/mark3labs/go-bash/fs"
)

// TestSourceDepthEnforced verifies /bin/source bumps Context.SourceDepth
// against Limits.MaxSourceDepth across nested invocations.
func TestSourceDepthEnforced(t *testing.T) {
	// Build a script that sources itself recursively.
	files := map[string]fs.FileInit{
		"/recur.sh": {Content: []byte("/bin/source /recur.sh\n")},
	}
	maxDepth := 3
	b, err := gobash.New(gobash.BashOptions{
		Files:           files,
		ExecutionLimits: &gobash.ExecutionLimits{MaxSourceDepth: &maxDepth},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, _ := b.Exec(context.Background(), "/bin/source /recur.sh", gobash.ExecOptions{})
	if !strings.Contains(res.Stderr, "MaxSourceDepth") {
		t.Errorf("expected MaxSourceDepth diagnostic; stderr=%q", res.Stderr)
	}
}
