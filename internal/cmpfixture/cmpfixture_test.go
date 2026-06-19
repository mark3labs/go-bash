package cmpfixture_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/go-bash/internal/cmpfixture"
)

// TestRunFixture_HappyPath constructs an in-memory fixture, runs it,
// and asserts the harness greenlights a matching result. The script
// uses only Phase 5 surface (echo + mvdan/sh's builtin) so the
// fixture passes even before the Phase 10 built-ins register.
func TestRunFixture_HappyPath(t *testing.T) {
	fx := &cmpfixture.Fixture{
		Script: "echo hello",
		Expected: cmpfixture.Expected{
			Stdout:   "hello\n",
			ExitCode: 0,
		},
	}
	cmpfixture.RunFixture(t, fx)
}

// TestRunFixture_StderrAndExit asserts non-zero exit + stderr both
// route through the assertion path.
func TestRunFixture_StderrAndExit(t *testing.T) {
	fx := &cmpfixture.Fixture{
		Script: "echo oops 1>&2; exit 3",
		Expected: cmpfixture.Expected{
			Stderr:   "oops\n",
			ExitCode: 3,
		},
	}
	cmpfixture.RunFixture(t, fx)
}

// TestRunFixture_FilesAndCwd asserts Files seed the VFS and Cwd
// applies at Exec time. Uses [ -f ... ] (an mvdan/sh internal
// builtin) to avoid depending on cat/ls/etc. before those built-ins
// are registered.
func TestRunFixture_FilesAndCwd(t *testing.T) {
	fx := &cmpfixture.Fixture{
		Script: "[ -f /etc/greeting ] && echo present",
		Files: map[string]string{
			"/etc/greeting": "hi from fixture\n",
		},
		Expected: cmpfixture.Expected{
			Stdout: "present\n",
		},
	}
	cmpfixture.RunFixture(t, fx)
}

// TestLoad_RoundTrip writes a JSON fixture to disk and reloads it.
func TestLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fx.json")
	content := []byte(`{"script":"echo hi","expected":{"stdout":"hi\n","exitCode":0}}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	fx, err := cmpfixture.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if fx.Script != "echo hi" {
		t.Errorf("script = %q", fx.Script)
	}
	if fx.Expected.Stdout != "hi\n" {
		t.Errorf("stdout = %q", fx.Expected.Stdout)
	}
}

// TestRunDir runs every .json fixture in a dir as a subtest.
func TestRunDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"one.json": `{"script":"echo one","expected":{"stdout":"one\n","exitCode":0}}`,
		"two.json": `{"script":"echo two","expected":{"stdout":"two\n","exitCode":0}}`,
		// non-JSON file should be skipped.
		"readme.txt": `not a fixture`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmpfixture.RunDir(t, dir)
}
