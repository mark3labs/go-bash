// Package cmpfixture is the comparison-test harness for go-bash.
// Each fixture is a JSON file describing a script, optional VFS
// files, optional env, optional cwd, and the expected stdout /
// stderr / exit code. Run loads one fixture and asserts a *Bash
// instance produces the expected output byte-for-byte.
//
// The fixture format is a near-direct port of just-bash's
// src/comparison-tests/ shape (SPEC §19.1). Future enhancements
// (Phase 19) will add the RECORD_FIXTURES re-recording path and the
// locked-fixture skip logic; for Phase 10 we only need the read-side
// loader so Wave A built-ins can ship with at least one fixture each.
//
// Cited surface: SPEC §19.1, §19.2.
package cmpfixture

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/fs"
)

// Fixture is the on-disk shape of a comparison-test JSON file. Field
// names match the just-bash format (`script`, `files`, `env`, `cwd`,
// `locked`, `expected`) so a future bulk-import script (SPEC §19.4)
// can copy files in without rewriting keys.
type Fixture struct {
	// Script is the bash source the fixture exercises. Required.
	Script string `json:"script"`

	// Files seeds the in-memory filesystem before Exec runs. The
	// value is the file content as a string (UTF-8). Phase 10 does
	// not yet need binary content; if Phase 14+ needs it, add a
	// `filesBase64` companion field.
	Files map[string]string `json:"files,omitempty"`

	// Env seeds the per-Bash environment. Merged on top of the
	// default ProcessInfo-derived env.
	Env map[string]string `json:"env,omitempty"`

	// Cwd overrides the default working directory. Empty leaves the
	// gobash.New default (/home/user when Files is empty, / otherwise).
	Cwd string `json:"cwd,omitempty"`

	// Locked marks a fixture whose expected block has been hand-edited
	// (typically to paper over a TS-vs-Go divergence). The Phase 19
	// recorder skips locked fixtures unless RECORD_FIXTURES=force.
	// Phase 10's loader treats locked fixtures identically to unlocked
	// ones — we still run and assert against `expected`.
	Locked bool `json:"locked,omitempty"`

	// Expected is the recorded result of running Script under real
	// bash (or, for hand-locked fixtures, the negotiated expected
	// shape). Run asserts BashExecResult equals this block.
	Expected Expected `json:"expected"`
}

// Expected is the result triple a fixture pins. ExitCode is the
// script's exit status; Stdout and Stderr are byte-exact.
type Expected struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

// Load reads a fixture from disk. Returns the parsed Fixture or an
// error if the file is missing or malformed.
func Load(path string) (*Fixture, error) {
	data, err := os.ReadFile(path) //nolint:gosec // fixture paths come from test code, not user input
	if err != nil {
		return nil, err
	}
	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Run loads the fixture at path and asserts a *Bash configured from
// its files/env/cwd produces the expected stdout / stderr / exitCode
// when Exec(fixture.Script) runs. The harness fails the test via
// t.Errorf for any divergence and t.Fatalf for setup errors.
func Run(t *testing.T, path string) {
	t.Helper()
	fx, err := Load(path)
	if err != nil {
		t.Fatalf("cmpfixture: load %s: %v", path, err)
	}
	RunFixture(t, fx)
}

// RunFixture runs an in-memory fixture (useful for tests that
// generate a fixture programmatically). Caller is responsible for
// passing a non-nil fx.
func RunFixture(t *testing.T, fx *Fixture) {
	t.Helper()
	files := make(map[string]fs.FileInit, len(fx.Files))
	for p, content := range fx.Files {
		files[p] = fs.FileInit{Content: []byte(content)}
	}
	b, err := gobash.New(gobash.BashOptions{
		Cwd:   fx.Cwd,
		Env:   fx.Env,
		Files: files,
	})
	if err != nil {
		t.Fatalf("cmpfixture: gobash.New: %v", err)
	}
	res, err := b.Exec(context.Background(), fx.Script, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("cmpfixture: Exec error: %v", err)
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
}

// RunDir runs every *.json fixture in dir as a subtest. Files are
// sorted by name for deterministic test ordering; non-JSON entries
// and subdirectories are skipped. Empty dir is not an error — the
// caller should pass t.Skip for "no fixtures yet" cases.
func RunDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cmpfixture: read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(strings.TrimSuffix(name, ".json"), func(t *testing.T) {
			Run(t, filepath.Join(dir, name))
		})
	}
}
