package gobash_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	gbfs "github.com/mark3labs/go-bash/fs"
	"github.com/mark3labs/go-bash/fs/memfs"
)

// TestBashFSDefault confirms that New() with zero options yields a
// non-nil in-memory FS reachable via Bash.FS().
func TestBashFSDefault(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if b.FS() == nil {
		t.Fatal("Bash.FS() returned nil")
	}
	// Default home dir auto-created during New so mvdan/sh's pwd works.
	fi, err := b.FS().Stat("/home/user")
	if err != nil {
		t.Fatalf("Stat /home/user: %v", err)
	}
	if !fi.IsDir() {
		t.Errorf("/home/user should be a directory")
	}
}

// TestBashOptionsFilesRoundTrip is the SPEC §3.8 acceptance criterion
// for Phase 3: BashOptions.Files round-trips through Bash.FS().ReadFile.
func TestBashOptionsFilesRoundTrip(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/etc/motd":          {Content: []byte("welcome\n")},
			"/home/user/.bashrc": {Content: []byte("alias ll='ls -l'\n")},
			"/var/log":           {Dir: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.FS().ReadFile("/etc/motd")
	if err != nil {
		t.Fatalf("ReadFile motd: %v", err)
	}
	if string(got) != "welcome\n" {
		t.Errorf("motd = %q; want %q", got, "welcome\n")
	}
	got, err = b.FS().ReadFile("/home/user/.bashrc")
	if err != nil {
		t.Fatalf("ReadFile .bashrc: %v", err)
	}
	if string(got) != "alias ll='ls -l'\n" {
		t.Errorf(".bashrc = %q", got)
	}
	fi, err := b.FS().Stat("/var/log")
	if err != nil || !fi.IsDir() {
		t.Errorf("/var/log not a dir: fi=%+v err=%v", fi, err)
	}
}

// TestScriptRedirectWritesToVFS verifies that `>` redirection lands in
// the virtual filesystem rather than touching the host disk.
//
// We deliberately use the shell `read` builtin to confirm the read-side
// path also goes through the VFS — Phase 5 will introduce real commands
// like `cat`, but for Phase 3 we exercise only redirects + builtins.
func TestScriptRedirectWritesToVFS(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		`echo "vfs contents" > /home/user/canary; read line < /home/user/canary; echo "got=$line"`,
		gobash.ExecOptions{},
	)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "got=vfs contents\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "got=vfs contents\n")
	}
	got, err := b.FS().ReadFile("/home/user/canary")
	if err != nil {
		t.Fatalf("FS.ReadFile: %v", err)
	}
	if string(got) != "vfs contents\n" {
		t.Errorf("VFS contents = %q", got)
	}
}

// TestSeededFilesVisibleToScript verifies that a Files-seeded file is
// readable from inside the script via `<` redirect + `read`.
func TestSeededFilesVisibleToScript(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/data/value.txt": {Content: []byte("seeded-value\n")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		`read line < /data/value.txt; echo "$line"`,
		gobash.ExecOptions{},
	)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "seeded-value\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "seeded-value\n")
	}
}

// TestBashOptionsFSOverride confirms that passing FS overrides the
// default memfs construction.
func TestBashOptionsFSOverride(t *testing.T) {
	custom := memfs.New()
	if err := custom.WriteFile("/marker", []byte("custom-fs"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := gobash.New(gobash.BashOptions{FS: custom})
	if err != nil {
		t.Fatal(err)
	}
	if b.FS() != custom {
		t.Errorf("Bash.FS() did not return the custom FS")
	}
	got, err := b.FS().ReadFile("/marker")
	if err != nil || string(got) != "custom-fs" {
		t.Errorf("custom FS not in use: got=%q err=%v", got, err)
	}
}

// TestStatViaVFS confirms that `[ -f /file ]` style tests route through
// our overridden StatHandler.
func TestStatViaVFS(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/exists": {Content: []byte("y")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		`if [ -f /exists ]; then echo yes; else echo no; fi; if [ -f /missing ]; then echo yes; else echo no; fi`,
		gobash.ExecOptions{},
	)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "yes\nno\n" {
		t.Errorf("Stdout = %q; want yes\\nno\\n", res.Stdout)
	}
}

// TestScriptCannotReadHostFile is a security canary: even if the script
// asks for /etc/passwd, our VFS handler reports whatever the VFS holds
// (or ErrNotExist), never the host file.
//
// The test uses the `read < /etc/passwd` redirect which goes through
// the OpenHandler (no external command involved). The VFS is pre-seeded
// with a synthetic content, and we assert the script reads that synthetic
// value rather than the real one from disk.
func TestScriptCannotReadHostFile(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/etc/passwd": {Content: []byte("vfs-only\n")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		`read line < /etc/passwd; echo "$line"`,
		gobash.ExecOptions{},
	)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "vfs-only\n" {
		t.Errorf("Stdout = %q; want %q (host /etc/passwd may have leaked)", res.Stdout, "vfs-only\n")
	}
	if strings.Contains(res.Stdout, "root:") {
		t.Errorf("Stdout looks like host /etc/passwd: %q", res.Stdout)
	}
}

// TestRedirectToMissingDirError confirms that a redirect to a path
// whose parent does not exist surfaces as a script-level redirect
// failure (non-zero exit, stderr message).
func TestRedirectToMissingDirError(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		`echo x > /no/such/dir/file 2>/dev/null; echo "exit=$?"`,
		gobash.ExecOptions{},
	)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	// The script exit code is the last command (the echo), but the
	// failing redirect should have set $? to non-zero before the echo.
	if !strings.Contains(res.Stdout, "exit=") {
		t.Errorf("Stdout = %q; want exit= line", res.Stdout)
	}
	if strings.Contains(res.Stdout, "exit=0\n") {
		t.Errorf("redirect to missing dir reported success: %q", res.Stdout)
	}
}

// TestFilesNullByteRejected confirms BashOptions.Files paths containing
// a NUL byte are rejected at construction time.
func TestFilesNullByteRejected(t *testing.T) {
	_, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/etc/bad\x00path": {Content: []byte("x")},
		},
	})
	if err == nil {
		t.Fatal("expected null-byte rejection")
	}
	if !errors.Is(err, gbfs.ErrNullByte) {
		t.Errorf("err = %v; want ErrNullByte", err)
	}
}
