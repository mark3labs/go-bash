package builtinutil

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
)

// OpenInput returns an io.Reader for the given operand. The literal
// "-" or "" means c.Stdin (returning a discard-on-nil reader).
// Otherwise the path is resolved against c.Cwd and opened via c.FS.
// The caller is responsible for closing any returned io.Closer.
func OpenInput(c *command.Context, name string) (io.Reader, io.Closer, error) {
	if name == "" || name == "-" {
		if c.Stdin == nil {
			return strings.NewReader(""), nil, nil
		}
		return c.Stdin, nil, nil
	}
	if c.FS == nil {
		return nil, nil, fmt.Errorf("no filesystem")
	}
	abs := ResolvePath(c.Cwd, name)
	f, err := c.FS.OpenFile(abs, 0 /* O_RDONLY */, 0)
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}

// ReadAllInputs reads each operand sequentially (with the same "-"
// convention as OpenInput) and returns the concatenated bytes. If
// names is empty, stdin is read.
func ReadAllInputs(c *command.Context, names []string) ([]byte, error) {
	if len(names) == 0 {
		names = []string{"-"}
	}
	var out []byte
	for _, n := range names {
		r, closer, err := OpenInput(c, n)
		if err != nil {
			return nil, err
		}
		data, rerr := io.ReadAll(r)
		if closer != nil {
			_ = closer.Close()
		}
		if rerr != nil {
			return nil, rerr
		}
		out = append(out, data...)
	}
	return out, nil
}

// ScanLines returns a *bufio.Scanner sized to handle long lines
// (16 MiB max). The scanner uses bufio.ScanLines (no trailing \n in
// the returned token).
func ScanLines(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return sc
}
