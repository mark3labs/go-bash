// Package yq implements the `yq` data-format swiss-army built-in
// (SPEC §10 Wave E). Reads YAML/JSON/XML/TOML/CSV, optionally applies a
// gojq filter, and emits any of the same formats.
//
// Flags:
//
//	-i FMT, --input FMT      input format (default yaml)
//	-o FMT, --output FMT     output format (default yaml)
//	-p FMT                   set both input and output
//	-r, --raw-output         emit strings without JSON/YAML quoting
//	-c, --compact            compact JSON output
//	--help                   show this help
//
// The filter is optional. When omitted, the document round-trips through
// the chosen formats unchanged (modulo lossy conversions, e.g. XML →
// JSON drops comments and PIs). All file I/O routes through
// Context.FS; stdin via Context.Stdin; stdout via Context.Stdout.
package yq

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	stdstrings "strings"

	"github.com/BurntSushi/toml"
	"github.com/goccy/go-yaml"
	"github.com/itchyny/gojq"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "yq [-i FMT] [-o FMT] [-r] [-c] [FILTER] [FILE...]"
const helpText = `Usage: yq [OPTIONS] [FILTER] [FILE...]
Convert and query YAML/JSON/XML/TOML/CSV. FILTER is a jq expression
(default: ".").

  -i, --input FMT     input format: yaml|json|xml|toml|csv (default yaml)
  -o, --output FMT    output format: yaml|json|xml|toml|csv (default yaml)
  -p FMT              set both input and output formats
  -r, --raw-output    emit strings without JSON/YAML quoting
  -c, --compact       compact JSON output
      --help          show this help`

// New returns the yq command.
func New() command.Command { return command.Define("yq", run) }

func run(ctx context.Context, args []string, c *command.Context) command.Result {
	inFmt := "yaml"
	outFmt := "yaml"
	rawOut := false
	compact := false
	var positional []string

	i := 1
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--help":
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		case a == "-i", a == "--input", a == "--input-format":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			inFmt = stdstrings.ToLower(args[i+1])
			i++
		case a == "-o", a == "--output", a == "--output-format":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			outFmt = stdstrings.ToLower(args[i+1])
			i++
		case a == "-p":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			inFmt = stdstrings.ToLower(args[i+1])
			outFmt = inFmt
			i++
		case stdstrings.HasPrefix(a, "--input="):
			inFmt = stdstrings.ToLower(stdstrings.TrimPrefix(a, "--input="))
		case stdstrings.HasPrefix(a, "--output="):
			outFmt = stdstrings.ToLower(stdstrings.TrimPrefix(a, "--output="))
		case a == "-r", a == "--raw-output":
			rawOut = true
		case a == "-c", a == "--compact", a == "--compact-output":
			compact = true
		case a == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case stdstrings.HasPrefix(a, "--"):
			return builtinutil.UsageError(c.Stderr, usage)
		case stdstrings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			ok := true
			for _, ch := range a[1:] {
				switch ch {
				case 'r':
					rawOut = true
				case 'c':
					compact = true
				default:
					ok = false
				}
			}
			if !ok {
				return builtinutil.UsageError(c.Stderr, usage)
			}
		default:
			positional = append(positional, a)
		}
	}

	if !knownFormat(inFmt) {
		return builtinutil.Errorf(c.Stderr, "yq", 2, "unknown input format %q", inFmt)
	}
	if !knownFormat(outFmt) {
		return builtinutil.Errorf(c.Stderr, "yq", 2, "unknown output format %q", outFmt)
	}

	filter := "."
	var files []string
	if len(positional) > 0 {
		// Heuristic: anything starting with `.` or `(` or `[` is a filter;
		// otherwise treat positional[0] as a file if it exists, else as a
		// filter. Simpler rule: first positional is filter when there are
		// no other positionals OR when it starts with a jq-ish prefix.
		if looksLikeFilter(positional[0]) || len(positional) > 1 {
			filter = positional[0]
			files = positional[1:]
		} else {
			files = positional
		}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}

	data, err := builtinutil.ReadAllInputs(c, files)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "yq", 2, "%v", err)
	}

	inputs, err := decode(inFmt, data)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "yq", 2, "%s decode: %v", inFmt, err)
	}

	q, err := gojq.Parse(filter)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "yq", 3, "filter: %v", err)
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "yq", 3, "filter: %v", err)
	}

	var results []any
	maxIter := c.Limits.MaxJqIterations
	tick := 0
	for _, in := range inputs {
		iter := code.RunWithContext(ctx, in)
		for {
			tick++
			if maxIter > 0 && tick > maxIter {
				return builtinutil.Errorf(c.Stderr, "yq", 5, "MaxJqIterations exceeded")
			}
			v, ok := iter.Next()
			if !ok {
				break
			}
			if e, isErr := v.(error); isErr {
				if he, ok := e.(*gojq.HaltError); ok && he.Value() == nil {
					return command.Result{ExitCode: 0}
				}
				return builtinutil.Errorf(c.Stderr, "yq", 5, "%v", e)
			}
			results = append(results, v)
		}
	}

	if err := encode(c.Stdout, outFmt, results, compact, rawOut); err != nil {
		return builtinutil.Errorf(c.Stderr, "yq", 2, "%s encode: %v", outFmt, err)
	}
	return command.Result{}
}

// looksLikeFilter returns true when s looks like a jq expression rather
// than a file path. It is intentionally conservative — when both
// interpretations are plausible (e.g. "foo"), we treat the first
// positional as the filter only if there are additional positionals.
func looksLikeFilter(s string) bool {
	if s == "." || s == "" {
		return true
	}
	first := s[0]
	switch first {
	case '.', '(', '[', '{', '$':
		return true
	}
	return false
}

func knownFormat(f string) bool {
	switch f {
	case "yaml", "yml", "json", "xml", "toml", "csv":
		return true
	}
	return false
}

// ---------- decode ----------

func decode(format string, data []byte) ([]any, error) {
	switch format {
	case "yaml", "yml":
		return decodeYAML(data)
	case "json":
		return decodeJSON(data)
	case "xml":
		v, err := decodeXML(data)
		if err != nil {
			return nil, err
		}
		return []any{v}, nil
	case "toml":
		v, err := decodeTOML(data)
		if err != nil {
			return nil, err
		}
		return []any{v}, nil
	case "csv":
		v, err := decodeCSV(data)
		if err != nil {
			return nil, err
		}
		return []any{v}, nil
	}
	return nil, fmt.Errorf("unknown format %q", format)
}

func decodeYAML(data []byte) ([]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
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
		out = append(out, normalize(v))
	}
	return out, nil
}

func decodeJSON(data []byte) ([]any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
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
		out = append(out, normalize(v))
	}
	return out, nil
}

func decodeTOML(data []byte) (any, error) {
	var v map[string]any
	if err := toml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return normalize(v), nil
}

func decodeCSV(data []byte) (any, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return []any{}, nil
	}
	header := all[0]
	out := make([]any, 0, len(all)-1)
	for _, row := range all[1:] {
		m := make(map[string]any, len(header))
		for i, h := range header {
			if i < len(row) {
				m[h] = row[i]
			} else {
				m[h] = ""
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// normalize coerces decoder-specific types (json.Number, map[any]any
// from go-yaml's any-key paths) into JSON-canonical Go shapes that
// gojq understands.
func normalize(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		f, _ := t.Float64()
		return f
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[fmt.Sprint(k)] = normalize(vv)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = normalize(vv)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, vv := range t {
			out[i] = normalize(vv)
		}
		return out
	default:
		return t
	}
}

// ---------- encode ----------

func encode(w io.Writer, format string, vals []any, compact, raw bool) error {
	switch format {
	case "json":
		return encodeJSON(w, vals, compact, raw)
	case "yaml", "yml":
		return encodeYAML(w, vals, raw)
	case "toml":
		return encodeTOML(w, vals)
	case "xml":
		return encodeXML(w, vals)
	case "csv":
		return encodeCSV(w, vals)
	}
	return fmt.Errorf("unknown format %q", format)
}

func encodeJSON(w io.Writer, vals []any, compact, raw bool) error {
	for _, v := range vals {
		if raw {
			if s, ok := v.(string); ok {
				if _, err := fmt.Fprintln(w, s); err != nil {
					return err
				}
				continue
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
		if _, err := w.Write(buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func encodeYAML(w io.Writer, vals []any, raw bool) error {
	for i, v := range vals {
		if raw {
			if s, ok := v.(string); ok {
				if _, err := fmt.Fprintln(w, s); err != nil {
					return err
				}
				continue
			}
		}
		if i > 0 {
			if _, err := io.WriteString(w, "---\n"); err != nil {
				return err
			}
		}
		b, err := yaml.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

func encodeTOML(w io.Writer, vals []any) error {
	if len(vals) == 0 {
		return nil
	}
	if len(vals) > 1 {
		return fmt.Errorf("toml output supports a single document")
	}
	m, ok := vals[0].(map[string]any)
	if !ok {
		return fmt.Errorf("toml output requires a top-level object, got %T", vals[0])
	}
	enc := toml.NewEncoder(w)
	return enc.Encode(m)
}

func encodeXML(w io.Writer, vals []any) error {
	for _, v := range vals {
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("xml output requires top-level object, got %T", v)
		}
		if len(m) != 1 {
			return fmt.Errorf("xml output requires exactly one root element, got %d", len(m))
		}
		keys := mapKeys(m)
		root := keys[0]
		if _, err := io.WriteString(w, xml.Header); err != nil {
			return err
		}
		if err := writeXMLElem(w, root, m[root], 0); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func writeXMLElem(w io.Writer, name string, v any, depth int) error {
	indent := stdstrings.Repeat("  ", depth)
	switch t := v.(type) {
	case nil:
		_, err := fmt.Fprintf(w, "%s<%s/>", indent, name)
		return err
	case map[string]any:
		var attrs []string
		var text string
		var childKeys []string
		for _, k := range mapKeys(t) {
			switch {
			case stdstrings.HasPrefix(k, "@"):
				attrs = append(attrs, fmt.Sprintf(` %s=%q`,
					stdstrings.TrimPrefix(k, "@"), fmt.Sprint(t[k])))
			case k == "#text":
				text = fmt.Sprint(t[k])
			default:
				childKeys = append(childKeys, k)
			}
		}
		if len(childKeys) == 0 && text == "" {
			_, err := fmt.Fprintf(w, "%s<%s%s/>", indent, name, stdstrings.Join(attrs, ""))
			return err
		}
		if _, err := fmt.Fprintf(w, "%s<%s%s>", indent, name, stdstrings.Join(attrs, "")); err != nil {
			return err
		}
		if text != "" {
			if _, err := io.WriteString(w, xmlEscape(text)); err != nil {
				return err
			}
		}
		if len(childKeys) > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
			for _, k := range childKeys {
				cv := t[k]
				if arr, ok := cv.([]any); ok {
					for _, av := range arr {
						if err := writeXMLElem(w, k, av, depth+1); err != nil {
							return err
						}
						if _, err := io.WriteString(w, "\n"); err != nil {
							return err
						}
					}
				} else {
					if err := writeXMLElem(w, k, cv, depth+1); err != nil {
						return err
					}
					if _, err := io.WriteString(w, "\n"); err != nil {
						return err
					}
				}
			}
			if _, err := fmt.Fprintf(w, "%s", indent); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintf(w, "</%s>", name)
		return err
	case []any:
		// Implicit list: emit one element per item.
		for i, av := range t {
			if i > 0 {
				if _, err := io.WriteString(w, "\n"); err != nil {
					return err
				}
			}
			if err := writeXMLElem(w, name, av, depth); err != nil {
				return err
			}
		}
		return nil
	default:
		_, err := fmt.Fprintf(w, "%s<%s>%s</%s>", indent, name, xmlEscape(fmt.Sprint(t)), name)
		return err
	}
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func encodeCSV(w io.Writer, vals []any) error {
	if len(vals) == 0 {
		return nil
	}
	// Accept either a single value that is a list of maps, or a stream
	// of maps (one per value).
	var rows []map[string]any
	if len(vals) == 1 {
		if arr, ok := vals[0].([]any); ok {
			for _, e := range arr {
				m, ok := e.(map[string]any)
				if !ok {
					return fmt.Errorf("csv output requires list of objects")
				}
				rows = append(rows, m)
			}
		} else if m, ok := vals[0].(map[string]any); ok {
			rows = append(rows, m)
		} else {
			return fmt.Errorf("csv output requires object or array of objects")
		}
	} else {
		for _, v := range vals {
			m, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("csv output requires objects")
			}
			rows = append(rows, m)
		}
	}
	keySet := map[string]struct{}{}
	for _, r := range rows {
		for k := range r {
			keySet[k] = struct{}{}
		}
	}
	header := make([]string, 0, len(keySet))
	for k := range keySet {
		header = append(header, k)
	}
	sort.Strings(header)
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := make([]string, len(header))
		for i, h := range header {
			rec[i] = scalarToString(r[h])
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func scalarToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------- XML decode ----------

type xmlNode struct {
	name     string
	attrs    map[string]string
	text     string
	children []*xmlNode
}

func decodeXML(data []byte) (any, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var root *xmlNode
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if start, ok := tok.(xml.StartElement); ok {
			n, err := decodeXMLElem(dec, start)
			if err != nil {
				return nil, err
			}
			root = n
			break
		}
	}
	if root == nil {
		return nil, nil
	}
	return map[string]any{root.name: nodeValue(root)}, nil
}

func decodeXMLElem(dec *xml.Decoder, start xml.StartElement) (*xmlNode, error) {
	n := &xmlNode{name: start.Name.Local, attrs: map[string]string{}}
	for _, a := range start.Attr {
		n.attrs[a.Name.Local] = a.Value
	}
	var textBuf bytes.Buffer
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := decodeXMLElem(dec, t)
			if err != nil {
				return nil, err
			}
			n.children = append(n.children, child)
		case xml.CharData:
			textBuf.Write(t)
		case xml.EndElement:
			n.text = stdstrings.TrimSpace(textBuf.String())
			return n, nil
		}
	}
}

func nodeValue(n *xmlNode) any {
	if len(n.children) == 0 && len(n.attrs) == 0 {
		return n.text
	}
	out := make(map[string]any)
	for k, v := range n.attrs {
		out["@"+k] = v
	}
	if n.text != "" {
		out["#text"] = n.text
	}
	// Group children by name; collapse multiples to []any.
	grouped := make(map[string][]any)
	order := []string{}
	for _, ch := range n.children {
		if _, seen := grouped[ch.name]; !seen {
			order = append(order, ch.name)
		}
		grouped[ch.name] = append(grouped[ch.name], nodeValue(ch))
	}
	for _, name := range order {
		vs := grouped[name]
		if len(vs) == 1 {
			out[name] = vs[0]
		} else {
			out[name] = vs
		}
	}
	return out
}

func init() { command.RegisterBuiltin(New()) }
