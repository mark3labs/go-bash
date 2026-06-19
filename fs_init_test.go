package gobash_test

import (
	"context"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	gbfs "github.com/mark3labs/go-bash/fs"
)

// defaultLayoutPaths mirrors the spec directory list. Kept as a
// test-local slice so a divergence between the production helper and
// The spec lights up here as a missing path rather than as a silent
// agreement with a buggy helper.
var defaultLayoutPaths = []string{
	"/",
	"/home",
	"/home/user",
	"/bin",
	"/usr",
	"/usr/bin",
	"/tmp",
	"/etc",
	"/dev",
	"/proc",
	"/proc/self",
}

// TestPhase7DefaultLayoutPresent asserts every the spec directory
// exists in the VFS after New(BashOptions{}), and that the two
// templated files /etc/hostname and /proc/self/status were written.
func TestPhase7DefaultLayoutPresent(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fs := b.FS()
	for _, p := range defaultLayoutPaths {
		fi, err := fs.Stat(p)
		if err != nil {
			t.Errorf("Stat(%q): %v", p, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%q is not a directory", p)
		}
	}
	gotHost, err := fs.ReadFile("/etc/hostname")
	if err != nil {
		t.Fatalf("ReadFile /etc/hostname: %v", err)
	}
	if string(gotHost) != "localhost\n" {
		t.Errorf("hostname=%q, want %q", gotHost, "localhost\n")
	}
	statusBytes, err := fs.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatalf("ReadFile /proc/self/status: %v", err)
	}
	status := string(statusBytes)
	// Sanity-check the structural fields and default ProcessInfo
	// values (PID=1, PPID=0, UID=1000, GID=1000).
	for _, want := range []string{
		"Name:\tbash\n",
		"Pid:\t1\n",
		"PPid:\t0\n",
		"Uid:\t1000\t1000\t1000\t1000\n",
		"Gid:\t1000\t1000\t1000\t1000\n",
	} {
		if !strings.Contains(status, want) {
			t.Errorf("/proc/self/status missing %q\nfull text:\n%s", want, status)
		}
	}
}

// TestPhase7DefaultEnvHomeAndPath asserts $HOME and $PATH are seeded
// from the default layout and reach scripts through Exec.
func TestPhase7DefaultEnvHomeAndPath(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := b.Exec(ctx, `echo "$HOME"; echo "$PATH"`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	want := "/home/user\n/usr/bin:/bin\n"
	if res.Stdout != want {
		t.Errorf("stdout=%q, want %q", res.Stdout, want)
	}
}

// TestPhase7UserHomeOverridesDefault confirms that a user-supplied
// Env["HOME"] beats the default-layout seed (and similarly for PATH).
func TestPhase7UserHomeOverridesDefault(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Env: map[string]string{
			"HOME": "/custom/home",
			"PATH": "/sbin",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	res, err := b.Exec(ctx, `echo "$HOME"; echo "$PATH"`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	want := "/custom/home\n/sbin\n"
	if res.Stdout != want {
		t.Errorf("stdout=%q, want %q", res.Stdout, want)
	}
}

// TestPhase7ProcSelfStatusInterpolatesProcessInfo confirms the
// templated file picks up custom ProcessInfo values rather than the
// defaults.
func TestPhase7ProcSelfStatusInterpolatesProcessInfo(t *testing.T) {
	info := gobash.ProcessInfo{PID: 42, PPID: 7, UID: 501, GID: 502}
	b, err := gobash.New(gobash.BashOptions{ProcessInfo: &info})
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.FS().ReadFile("/proc/self/status")
	if err != nil {
		t.Fatalf("ReadFile /proc/self/status: %v", err)
	}
	want := "Name:\tbash\n" +
		"Pid:\t42\n" +
		"PPid:\t7\n" +
		"Uid:\t501\t501\t501\t501\n" +
		"Gid:\t502\t502\t502\t502\n"
	if string(got) != want {
		t.Errorf("/proc/self/status mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestPhase7CwdSuppressesDefaultLayout asserts that providing a Cwd
// skips the default layout. /etc/hostname, /proc/self/status, and
// the §7 directories (other than the Cwd itself, which New still
// auto-creates) must NOT exist.
func TestPhase7CwdSuppressesDefaultLayout(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{Cwd: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	fs := b.FS()
	// The supplied cwd is the only directory that should exist.
	if fi, err := fs.Stat("/workspace"); err != nil {
		t.Fatalf("Stat /workspace: %v", err)
	} else if !fi.IsDir() {
		t.Errorf("/workspace should be a directory")
	}
	// None of the §7 sentinel files should exist.
	if _, err := fs.Stat("/etc/hostname"); err == nil {
		t.Errorf("/etc/hostname should not exist when Cwd was supplied")
	}
	if _, err := fs.Stat("/proc/self/status"); err == nil {
		t.Errorf("/proc/self/status should not exist when Cwd was supplied")
	}
	// /home/user is one of the §7 dirs; with Cwd supplied it should
	// be absent (the default-cwd /home/user branch in New also no
	// longer fires because b.cwd != "").
	if _, err := fs.Stat("/home/user"); err == nil {
		t.Errorf("/home/user should not exist when Cwd was supplied")
	}
	// $HOME / $PATH default seeds must not fire. PATH is the safe
	// canary: mvdan/sh does NOT auto-fill PATH from the host, whereas
	// it does auto-fill HOME from the host user when env is empty (see
	// the runner's Reset path). The default-layout seed sets PATH to
	// "/usr/bin:/bin" — when the layout is suppressed, PATH stays empty.
	res, err := b.Exec(context.Background(), `echo "PATH=$PATH"`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "PATH=\n" {
		t.Errorf("env stdout=%q, want %q", res.Stdout, "PATH=\n")
	}
}

// TestPhase7FilesSuppressDefaultLayout asserts that providing a
// Files map skips the default layout. The seeded file is present
// but the §7 dirs/sentinel files are not.
func TestPhase7FilesSuppressDefaultLayout(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/workspace/data.txt": {Content: []byte("seeded\n")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fs := b.FS()
	if got, err := fs.ReadFile("/workspace/data.txt"); err != nil {
		t.Fatalf("ReadFile seeded: %v", err)
	} else if string(got) != "seeded\n" {
		t.Errorf("seeded content=%q", got)
	}
	if _, err := fs.Stat("/etc/hostname"); err == nil {
		t.Errorf("/etc/hostname should not exist when Files was supplied")
	}
	if _, err := fs.Stat("/proc/self/status"); err == nil {
		t.Errorf("/proc/self/status should not exist when Files was supplied")
	}
	if _, err := fs.Stat("/home/user"); err == nil {
		t.Errorf("/home/user should not exist when Files was supplied")
	}
	if _, err := fs.Stat("/bin"); err == nil {
		t.Errorf("/bin should not exist when Files was supplied")
	}
}

// TestPhase7LayoutVisibleToScripts is an end-to-end sanity check:
// a script reads /etc/hostname through the standard `< file` form
// and observes the default layout content.
func TestPhase7LayoutVisibleToScripts(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.Exec(context.Background(),
		`read host < /etc/hostname; echo "$host"`,
		gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "localhost\n" {
		t.Errorf("stdout=%q, want %q", res.Stdout, "localhost\n")
	}
}
