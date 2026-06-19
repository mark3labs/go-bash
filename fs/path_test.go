package fs

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		in      string
		wantErr error
	}{
		{"", ErrEmptyPath},
		{"a\x00b", ErrNullByte},
		{"/etc/passwd", nil},
		{"a/b/c", nil},
	}
	for _, tt := range tests {
		err := Validate(tt.in)
		if !errors.Is(err, tt.wantErr) {
			t.Errorf("Validate(%q) err = %v; want %v", tt.in, err, tt.wantErr)
		}
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "."},
		{"/", "/"},
		{"/a/b/", "/a/b"},
		{"/a/./b", "/a/b"},
		{"/a/b/../c", "/a/c"},
		{"/a/b/../..", "/"},
		{"/..", "/"},
		{"/../../etc", "/etc"},
		{"a/b/../c", "a/c"},
		{"a/..", "."},
		{"./a", "a"},
		{"a//b", "a/b"},
	}
	for _, tt := range tests {
		if got := Clean(tt.in); got != tt.want {
			t.Errorf("Clean(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		parts []string
		want  string
	}{
		{[]string{"/a", "b"}, "/a/b"},
		{[]string{"/a/", "/b"}, "/a/b"},
		{[]string{"a", "b", "c"}, "a/b/c"},
		{[]string{}, "."},
		{[]string{"", "", ""}, "."},
		{[]string{"/", "etc", "passwd"}, "/etc/passwd"},
	}
	for _, tt := range tests {
		if got := Join(tt.parts...); got != tt.want {
			t.Errorf("Join(%v) = %q; want %q", tt.parts, got, tt.want)
		}
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		base, path, want string
	}{
		{"/home/user", "file", "/home/user/file"},
		{"/home/user", "/etc/passwd", "/etc/passwd"},
		{"/a/b", "../c", "/a/c"},
		{"", "x/y", "x/y"},
	}
	for _, tt := range tests {
		if got := Resolve(tt.base, tt.path); got != tt.want {
			t.Errorf("Resolve(%q,%q) = %q; want %q", tt.base, tt.path, got, tt.want)
		}
	}
}

func TestDirnameBasename(t *testing.T) {
	tests := []struct{ in, dir, base string }{
		{"/", "/", "/"},
		{"/a/b/c", "/a/b", "c"},
		{"/a", "/", "a"},
		{"a", ".", "a"},
		{"a/b", "a", "b"},
	}
	for _, tt := range tests {
		if got := Dirname(tt.in); got != tt.dir {
			t.Errorf("Dirname(%q) = %q; want %q", tt.in, got, tt.dir)
		}
		if got := Basename(tt.in); got != tt.base {
			t.Errorf("Basename(%q) = %q; want %q", tt.in, got, tt.base)
		}
	}
}

func TestIsWithinRoot(t *testing.T) {
	tests := []struct {
		root, path string
		want       bool
	}{
		{"/sandbox", "/sandbox/a/b", true},
		{"/sandbox", "/sandbox", true},
		{"/sandbox", "/sandbox2", false},
		{"/sandbox", "/etc", false},
		{"/sandbox", "/sandbox/../etc", false},
		{"/", "/anything", true},
	}
	for _, tt := range tests {
		if got := IsWithinRoot(tt.root, tt.path); got != tt.want {
			t.Errorf("IsWithinRoot(%q,%q) = %v; want %v", tt.root, tt.path, got, tt.want)
		}
	}
}
