package gobash_test

import (
	"testing"

	_ "github.com/mark3labs/go-bash/builtins"
	"github.com/mark3labs/go-bash/internal/cmpfixture"
)

// TestPhase12ProcInfoFixtures runs the spec process-info comparison
// fixtures. Each pins one acceptance criterion: $$ → procInfo.PID,
// $PPID → procInfo.PPID, $BASHPID per-subshell counter, and
// /proc/self/status byte-exact template content.
func TestPhase12ProcInfoFixtures(t *testing.T) {
	cmpfixture.RunDir(t, "internal/testdata/fixtures/procinfo")
}
