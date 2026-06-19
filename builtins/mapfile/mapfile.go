// Package mapfile implements the `mapfile` / `readarray` shell
// built-ins. Reads stdin line-by-line into an "array"
// represented as c.Env entries `<NAME>_<INDEX>`. The runner-side
// array semantics are not propagated (mvdan/sh's native mapfile does
// that); /bin/mapfile and /bin/readarray here are best-effort.
package mapfile

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "mapfile [-t] [-n COUNT] [-O ORIGIN] [-s SKIP] [ARRAY]"

// New returns the mapfile command.
func New() command.Command { return command.Define("mapfile", run) }

// NewReadarray returns the readarray command (alias for mapfile).
func NewReadarray() command.Command { return command.Define("readarray", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	trim := false
	count := -1
	origin := 0
	skip := 0
	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, "Usage: "+usage+"\n")
			return command.Result{ExitCode: 0}
		case a == "--":
			i++
			goto done
		case a == "-t":
			trim = true
		case a == "-n":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			count = n
		case a == "-O":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			origin = n
		case a == "-s":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			skip = n
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			goto done
		}
	}
done:
	arrayName := "MAPFILE"
	if i < len(args) {
		arrayName = args[i]
	}
	if c.Stdin == nil {
		return command.Result{ExitCode: 0}
	}
	scanner := bufio.NewScanner(c.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	idx := origin
	added := 0
	skipped := 0
	for scanner.Scan() {
		if skipped < skip {
			skipped++
			continue
		}
		if count >= 0 && added >= count {
			break
		}
		line := scanner.Text()
		if !trim {
			line += "\n"
		}
		if c.Env != nil {
			c.Env[fmt.Sprintf("%s_%d", arrayName, idx)] = line
		}
		idx++
		added++
	}
	if err := scanner.Err(); err != nil {
		return builtinutil.Errorf(c.Stderr, "mapfile", 1, "%v", err)
	}
	return command.Result{ExitCode: 0}
}

func init() {
	command.RegisterBuiltin(New())
	command.RegisterBuiltin(NewReadarray())
}
