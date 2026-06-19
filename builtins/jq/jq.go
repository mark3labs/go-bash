// Package jq implements the `jq` built-in (SPEC §10 Wave D).
//
// Wraps github.com/itchyny/gojq. Honored flags: --raw-input/-R,
// --raw-output/-r, --slurp/-s, --compact/-c, --null-input/-n,
// --arg KEY VAL, --argjson KEY JSON.
//
// MaxJqIterations is honored via a tick counter wrapped around the
// gojq.Code.Run iterator.
package jq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdstrings "strings"

	"github.com/itchyny/gojq"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "jq [OPTIONS] FILTER [FILE...]"
const helpText = `Usage: jq [OPTION]... FILTER [FILE]...
Apply FILTER (a jq program) to each JSON input.

  -c, --compact         compact output (no pretty-printing)
  -r, --raw-output      output strings without JSON quoting
  -R, --raw-input       read raw strings instead of JSON
  -s, --slurp           read inputs into one array
  -n, --null-input      use null as the single input
      --arg KEY VAL     bind $KEY to the string VAL
      --argjson KEY VAL bind $KEY to the JSON value VAL
      --help            show this help`

// New returns the jq command.
func New() command.Command { return command.Define("jq", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	compact := false
	rawOutput := false
	rawInput := false
	slurp := false
	nullInput := false
	var argNames []string
	var argVals []any
	var filter string
	filterSet := false
	var files []string

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-c", a == "--compact-output", a == "--compact":
			compact = true
		case a == "-r", a == "--raw-output":
			rawOutput = true
		case a == "-R", a == "--raw-input":
			rawInput = true
		case a == "-s", a == "--slurp":
			slurp = true
		case a == "-n", a == "--null-input":
			nullInput = true
		case a == "--arg":
			if i+2 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			argNames = append(argNames, "$"+args[i+1])
			argVals = append(argVals, args[i+2])
			i += 2
		case a == "--argjson":
			if i+2 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			var v any
			if err := json.Unmarshal([]byte(args[i+2]), &v); err != nil {
				return builtinutil.Errorf(c.Stderr, "jq", 2, "--argjson %s: %v", args[i+1], err)
			}
			argNames = append(argNames, "$"+args[i+1])
			argVals = append(argVals, v)
			i += 2
		case a == "--":
			i++
			if !filterSet {
				if i < len(args) {
					filter = args[i]
					filterSet = true
					i++
				}
			}
			files = append(files, args[i:]...)
			goto run
		case stdstrings.HasPrefix(a, "--"):
			return builtinutil.UsageError(c.Stderr, usage)
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			// Bundled short flags: -cs, -Rs, -rs, -nc, etc.
			ok := true
			for _, ch := range a[1:] {
				switch ch {
				case 'c':
					compact = true
				case 'r':
					rawOutput = true
				case 'R':
					rawInput = true
				case 's':
					slurp = true
				case 'n':
					nullInput = true
				default:
					ok = false
				}
			}
			if !ok {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			if !filterSet {
				filter = a
				filterSet = true
			} else {
				files = append(files, a)
			}
		}
	}
run:
	if !filterSet {
		return builtinutil.UsageError(c.Stderr, usage)
	}

	q, err := gojq.Parse(filter)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "jq", 3, "%v", err)
	}
	code, err := gojq.Compile(q, gojq.WithVariables(argNames))
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "jq", 3, "%v", err)
	}

	// Collect input values.
	var inputs []any
	switch {
	case nullInput:
		inputs = []any{nil}
	case rawInput && slurp:
		// One string = whole input.
		data, err := readAll(c, files)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "jq", 2, "%v", err)
		}
		inputs = []any{string(data)}
	case rawInput:
		data, err := readAll(c, files)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "jq", 2, "%v", err)
		}
		s := stdstrings.TrimSuffix(string(data), "\n")
		if s == "" {
			inputs = nil
		} else {
			for _, line := range stdstrings.Split(s, "\n") {
				inputs = append(inputs, line)
			}
		}
	case slurp:
		vals, err := decodeAll(c, files)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "jq", 2, "%v", err)
		}
		inputs = []any{vals}
	default:
		vals, err := decodeAll(c, files)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "jq", 2, "%v", err)
		}
		inputs = vals
	}

	maxIter := c.Limits.MaxJqIterations
	tick := 0

	hadValue := false
	for _, in := range inputs {
		iter := code.RunWithContext(ctx, in, argVals...)
		for {
			tick++
			if maxIter > 0 && tick > maxIter {
				return builtinutil.Errorf(c.Stderr, "jq", 5, "MaxJqIterations exceeded")
			}
			v, ok := iter.Next()
			if !ok {
				break
			}
			if e, isErr := v.(error); isErr {
				if he, ok := e.(*gojq.HaltError); ok && he.Value() == nil {
					return command.Result{ExitCode: 0}
				}
				return builtinutil.Errorf(c.Stderr, "jq", 5, "%v", e)
			}
			hadValue = true
			if err := emit(c.Stdout, v, compact, rawOutput); err != nil {
				return builtinutil.Errorf(c.Stderr, "jq", 2, "%v", err)
			}
		}
	}
	_ = hadValue
	return command.Result{ExitCode: 0}
}

func emit(w io.Writer, v any, compact, rawOutput bool) error {
	if rawOutput {
		if s, ok := v.(string); ok {
			_, err := fmt.Fprintln(w, s)
			return err
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if !compact {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return err
	}
	// json.Encoder adds a trailing \n; that's fine.
	_, err := w.Write(buf.Bytes())
	return err
}

func readAll(c *command.Context, files []string) ([]byte, error) {
	return builtinutil.ReadAllInputs(c, files)
}

func decodeAll(c *command.Context, files []string) ([]any, error) {
	data, err := readAll(c, files)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out []any
	for {
		var v any
		err := dec.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, fromNumber(v))
	}
	return out, nil
}

// fromNumber converts json.Number values to float64/int. gojq accepts
// any numeric type but expects native Go numerics for arithmetic.
func fromNumber(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		f, _ := t.Float64()
		return f
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = fromNumber(vv)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = fromNumber(vv)
		}
		return out
	default:
		return t
	}
}

func init() { command.RegisterBuiltin(New()) }
