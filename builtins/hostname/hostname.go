// Package hostname implements the `hostname` built-in (SPEC §10 Wave A).
// Reads /etc/hostname from the VFS; defaults to "localhost" when the
// file is missing or empty. Setting the hostname is not supported in
// the sandboxed FS (matches just-bash).
package hostname

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "hostname [-s|-d|-f]"
const helpText = `Usage: hostname [OPTION]
Print the system hostname read from /etc/hostname.

  -s    short form (strip everything after first dot)
  -d    DNS domain (strip everything before first dot)
  -f    FQDN (default)`

// New returns the hostname command.
func New() command.Command {
	return command.Define("hostname", run)
}

func run(_ context.Context, args []string, c *command.Context) command.Result {
	mode := "full"
	for _, a := range args[1:] {
		switch a {
		case "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case "-s", "--short":
			mode = "short"
		case "-d", "--domain":
			mode = "domain"
		case "-f", "--fqdn", "--long":
			mode = "full"
		default:
			return builtinutil.UsageError(c.Stderr, usage)
		}
	}
	name := readHostname(c)
	switch mode {
	case "short":
		if i := strings.Index(name, "."); i >= 0 {
			name = name[:i]
		}
	case "domain":
		if i := strings.Index(name, "."); i >= 0 {
			name = name[i+1:]
		} else {
			name = ""
		}
	}
	if c.Stdout != nil {
		_, _ = io.WriteString(c.Stdout, name)
		_, _ = fmt.Fprintln(c.Stdout)
	}
	return command.Result{ExitCode: 0}
}

func readHostname(c *command.Context) string {
	if c.FS == nil {
		return "localhost"
	}
	f, err := c.FS.OpenFile("/etc/hostname", 0, 0)
	if err != nil {
		return "localhost"
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return "localhost"
	}
	data = bytes.TrimRight(data, "\r\n \t")
	if len(data) == 0 {
		return "localhost"
	}
	return string(data)
}

func init() {
	command.RegisterBuiltin(New())
}
