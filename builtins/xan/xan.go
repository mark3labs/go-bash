// Package xan implements the `xan` CSV toolkit built-in.
//
// Subset matching just-bash's port: subcommands `select`, `slice`,
// `filter`, `count`, `flatten`, `headers`, `stats`, `cat`, `to`, `from`.
// All file I/O routes through Context.FS; stdin via Context.Stdin;
// stdout via Context.Stdout.
//
// Filter expressions are a deliberate subset of xan's moonblade language:
// `COL OP VALUE` where OP is one of `==` / `!=` / `<` / `<=` / `>` / `>=`
// (numeric if both sides parse as float, else string) plus `=~` /
// `!~` (regex). COL is matched by header name or 1-based index.
package xan

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	stdstrings "strings"

	"github.com/mark3labs/go-bash/command"
	"github.com/mark3labs/go-bash/internal/builtinutil"
)

const usage = "xan SUBCOMMAND [OPTIONS] [FILE]"
const helpText = `Usage: xan SUBCOMMAND [OPTIONS] [FILE]
A small CSV toolkit (subset of medialab/xan).

Subcommands:
  select COLS [FILE]          keep only the named columns
  slice -s N -l N [FILE]      slice rows by offset/length
  filter EXPR [FILE]          keep rows where EXPR is true
  count [FILE]                print row count (excluding header)
  flatten [FILE]              print key:value blocks separated by blank lines
  headers [FILE]              list the column headers
  stats [FILE]                per-column count/min/max/mean
  cat [rows] FILE...          concatenate CSV files
  to FMT [FILE]               convert CSV to FMT (json|jsonl|tsv)
  from FMT [FILE]             convert FMT to CSV (json|jsonl|tsv)

      --help                  show this help`

// New returns the xan command.
func New() command.Command { return command.Define("xan", run) }

func run(_ context.Context, args []string, c *command.Context) command.Result {
	if len(args) < 2 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	for _, a := range args[1:] {
		if a == "--help" {
			builtinutil.PrintHelp(c.Stdout, helpText)
			return command.Result{ExitCode: 0}
		}
	}
	sub := args[1]
	rest := args[2:]
	switch sub {
	case "select":
		return runSelect(c, rest)
	case "slice":
		return runSlice(c, rest)
	case "filter":
		return runFilter(c, rest)
	case "count":
		return runCount(c, rest)
	case "flatten":
		return runFlatten(c, rest)
	case "headers":
		return runHeaders(c, rest)
	case "stats":
		return runStats(c, rest)
	case "cat":
		return runCat(c, rest)
	case "to":
		return runTo(c, rest)
	case "from":
		return runFrom(c, rest)
	default:
		return builtinutil.UsageError(c.Stderr, usage)
	}
}

// ---------- helpers ----------

func readCSV(c *command.Context, file string) (header []string, rows [][]string, err error) {
	r, closer, err := builtinutil.OpenInput(c, file)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if closer != nil {
			_ = closer.Close()
		}
	}()
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	all, err := cr.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, nil
	}
	return all[0], all[1:], nil
}

func writeCSV(w io.Writer, header []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if len(header) > 0 {
		if err := cw.Write(header); err != nil {
			return err
		}
	}
	for _, r := range rows {
		if err := cw.Write(r); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// resolveCol returns the 0-based column index for spec (either a header
// name or a 1-based numeric index). -1 if not found.
func resolveCol(header []string, spec string) int {
	for i, h := range header {
		if h == spec {
			return i
		}
	}
	if n, err := strconv.Atoi(spec); err == nil && n >= 1 && n <= len(header) {
		return n - 1
	}
	return -1
}

func parseColList(header []string, spec string) ([]int, error) {
	parts := stdstrings.Split(spec, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = stdstrings.TrimSpace(p)
		if p == "" {
			continue
		}
		i := resolveCol(header, p)
		if i < 0 {
			return nil, fmt.Errorf("unknown column %q", p)
		}
		out = append(out, i)
	}
	return out, nil
}

func popOneFile(args []string) (string, []string) {
	if len(args) == 0 {
		return "-", nil
	}
	return args[0], args[1:]
}

// ---------- subcommands ----------

func runSelect(c *command.Context, args []string) command.Result {
	if len(args) < 1 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	cols := args[0]
	file, extra := popOneFile(args[1:])
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	header, rows, err := readCSV(c, file)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	idx, err := parseColList(header, cols)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	newHeader := make([]string, len(idx))
	for i, j := range idx {
		newHeader[i] = header[j]
	}
	newRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		nr := make([]string, len(idx))
		for i, j := range idx {
			if j < len(r) {
				nr[i] = r[j]
			}
		}
		newRows = append(newRows, nr)
	}
	if err := writeCSV(c.Stdout, newHeader, newRows); err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	return command.Result{}
}

func runSlice(c *command.Context, args []string) command.Result {
	start := 0
	length := -1
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-s", a == "--start":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			start = n
			i++
		case a == "-l", a == "--length":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			length = n
			i++
		case a == "-e", a == "--end":
			if i+1 >= len(args) {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return builtinutil.UsageError(c.Stderr, usage)
			}
			length = n - start
			if length < 0 {
				length = 0
			}
			i++
		case a == "--":
			rest = append(rest, args[i+1:]...)
			i = len(args)
		case stdstrings.HasPrefix(a, "-") && a != "-":
			return builtinutil.UsageError(c.Stderr, usage)
		default:
			rest = append(rest, a)
		}
	}
	file, extra := popOneFile(rest)
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	header, rows, err := readCSV(c, file)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	if start > len(rows) {
		start = len(rows)
	}
	end := len(rows)
	if length >= 0 {
		end = start + length
		if end > len(rows) {
			end = len(rows)
		}
	}
	if err := writeCSV(c.Stdout, header, rows[start:end]); err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	return command.Result{}
}

func runFilter(c *command.Context, args []string) command.Result {
	if len(args) < 1 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	expr := args[0]
	file, extra := popOneFile(args[1:])
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	header, rows, err := readCSV(c, file)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	pred, err := compileFilter(header, expr)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		ok, err := pred(r)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
		}
		if ok {
			out = append(out, r)
		}
	}
	if err := writeCSV(c.Stdout, header, out); err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	return command.Result{}
}

func runCount(c *command.Context, args []string) command.Result {
	file, extra := popOneFile(args)
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	_, rows, err := readCSV(c, file)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	_, _ = fmt.Fprintf(c.Stdout, "%d\n", len(rows))
	return command.Result{}
}

func runFlatten(c *command.Context, args []string) command.Result {
	file, extra := popOneFile(args)
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	header, rows, err := readCSV(c, file)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	maxName := 0
	for _, h := range header {
		if len(h) > maxName {
			maxName = len(h)
		}
	}
	for i, r := range rows {
		if i > 0 {
			_, _ = fmt.Fprintln(c.Stdout)
		}
		for j, h := range header {
			val := ""
			if j < len(r) {
				val = r[j]
			}
			_, _ = fmt.Fprintf(c.Stdout, "%-*s  %s\n", maxName, h, val)
		}
	}
	return command.Result{}
}

func runHeaders(c *command.Context, args []string) command.Result {
	file, extra := popOneFile(args)
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	header, _, err := readCSV(c, file)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	for i, h := range header {
		_, _ = fmt.Fprintf(c.Stdout, "%d   %s\n", i+1, h)
	}
	return command.Result{}
}

func runStats(c *command.Context, args []string) command.Result {
	file, extra := popOneFile(args)
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	header, rows, err := readCSV(c, file)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	out := csv.NewWriter(c.Stdout)
	if err := out.Write([]string{"field", "count", "type", "min", "max", "mean"}); err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	for j, h := range header {
		numCount := 0
		var nums []float64
		strCount := 0
		var minS, maxS string
		first := true
		for _, r := range rows {
			if j >= len(r) {
				continue
			}
			v := r[j]
			if v == "" {
				continue
			}
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				nums = append(nums, f)
				numCount++
			} else {
				strCount++
				if first || v < minS {
					minS = v
				}
				if first || v > maxS {
					maxS = v
				}
				first = false
			}
		}
		typ := "string"
		minStr, maxStr, meanStr := "", "", ""
		if numCount > 0 && strCount == 0 {
			typ = "number"
			minN, maxN, sum := nums[0], nums[0], 0.0
			for _, n := range nums {
				if n < minN {
					minN = n
				}
				if n > maxN {
					maxN = n
				}
				sum += n
			}
			minStr = strconv.FormatFloat(minN, 'f', -1, 64)
			maxStr = strconv.FormatFloat(maxN, 'f', -1, 64)
			meanStr = strconv.FormatFloat(sum/float64(len(nums)), 'f', -1, 64)
		} else {
			minStr = minS
			maxStr = maxS
		}
		total := numCount + strCount
		if err := out.Write([]string{
			h, strconv.Itoa(total), typ, minStr, maxStr, meanStr,
		}); err != nil {
			return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
		}
	}
	out.Flush()
	if err := out.Error(); err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	return command.Result{}
}

func runCat(c *command.Context, args []string) command.Result {
	// Accept optional `rows` subkeyword; default mode is rows.
	files := args
	if len(args) > 0 && (args[0] == "rows" || args[0] == "columns") {
		if args[0] == "columns" {
			return builtinutil.Errorf(c.Stderr, "xan", 2,
				"cat columns not implemented")
		}
		files = args[1:]
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	var outHeader []string
	var outRows [][]string
	for i, f := range files {
		h, rows, err := readCSV(c, f)
		if err != nil {
			return builtinutil.Errorf(c.Stderr, "xan", 1, "%s: %v", f, err)
		}
		if i == 0 {
			outHeader = h
			outRows = rows
			continue
		}
		// Re-map columns by header name.
		mapping := make([]int, len(outHeader))
		for k, name := range outHeader {
			mapping[k] = -1
			for ki, hn := range h {
				if hn == name {
					mapping[k] = ki
					break
				}
			}
		}
		for _, r := range rows {
			nr := make([]string, len(outHeader))
			for k, src := range mapping {
				if src >= 0 && src < len(r) {
					nr[k] = r[src]
				}
			}
			outRows = append(outRows, nr)
		}
	}
	if err := writeCSV(c.Stdout, outHeader, outRows); err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	return command.Result{}
}

func runTo(c *command.Context, args []string) command.Result {
	if len(args) < 1 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	fmtName := args[0]
	file, extra := popOneFile(args[1:])
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	header, rows, err := readCSV(c, file)
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	switch fmtName {
	case "json":
		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			out = append(out, rowToMap(header, r))
		}
		enc := json.NewEncoder(c.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
		}
	case "jsonl", "ndjson":
		enc := json.NewEncoder(c.Stdout)
		for _, r := range rows {
			if err := enc.Encode(rowToMap(header, r)); err != nil {
				return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
			}
		}
	case "tsv":
		if len(header) > 0 {
			_, _ = fmt.Fprintln(c.Stdout, stdstrings.Join(header, "\t"))
		}
		for _, r := range rows {
			_, _ = fmt.Fprintln(c.Stdout, stdstrings.Join(r, "\t"))
		}
	default:
		return builtinutil.Errorf(c.Stderr, "xan", 2,
			"to: unknown format %q (want json|jsonl|tsv)", fmtName)
	}
	return command.Result{}
}

func runFrom(c *command.Context, args []string) command.Result {
	if len(args) < 1 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	fmtName := args[0]
	file, extra := popOneFile(args[1:])
	if len(extra) > 0 {
		return builtinutil.UsageError(c.Stderr, usage)
	}
	data, err := builtinutil.ReadAllInputs(c, []string{file})
	if err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	var header []string
	var rows [][]string
	switch fmtName {
	case "json":
		var arr []map[string]any
		if err := json.Unmarshal(data, &arr); err != nil {
			return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
		}
		header, rows = mapsToRows(arr)
	case "jsonl", "ndjson":
		dec := json.NewDecoder(stdstrings.NewReader(string(data)))
		var arr []map[string]any
		for {
			var m map[string]any
			err := dec.Decode(&m)
			if err == io.EOF {
				break
			}
			if err != nil {
				return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
			}
			arr = append(arr, m)
		}
		header, rows = mapsToRows(arr)
	case "tsv":
		lines := stdstrings.Split(stdstrings.TrimRight(string(data), "\n"), "\n")
		if len(lines) == 0 {
			break
		}
		header = stdstrings.Split(lines[0], "\t")
		for _, l := range lines[1:] {
			rows = append(rows, stdstrings.Split(l, "\t"))
		}
	default:
		return builtinutil.Errorf(c.Stderr, "xan", 2,
			"from: unknown format %q (want json|jsonl|tsv)", fmtName)
	}
	if err := writeCSV(c.Stdout, header, rows); err != nil {
		return builtinutil.Errorf(c.Stderr, "xan", 1, "%v", err)
	}
	return command.Result{}
}

// ---------- filter expression ----------

type rowPredicate func(row []string) (bool, error)

var filterRE = regexp.MustCompile(
	`^\s*(.+?)\s*(==|!=|<=|>=|=~|!~|<|>)\s*(.+?)\s*$`)

func compileFilter(header []string, expr string) (rowPredicate, error) {
	m := filterRE.FindStringSubmatch(expr)
	if m == nil {
		return nil, fmt.Errorf("filter: cannot parse %q", expr)
	}
	colSpec, op, valSpec := m[1], m[2], m[3]
	idx := resolveCol(header, colSpec)
	if idx < 0 {
		return nil, fmt.Errorf("filter: unknown column %q", colSpec)
	}
	val := unquote(valSpec)
	if op == "=~" || op == "!~" {
		re, err := regexp.Compile(val)
		if err != nil {
			return nil, fmt.Errorf("filter: bad regex %q: %v", val, err)
		}
		return func(r []string) (bool, error) {
			cur := ""
			if idx < len(r) {
				cur = r[idx]
			}
			m := re.MatchString(cur)
			if op == "!~" {
				m = !m
			}
			return m, nil
		}, nil
	}
	rhsNum, rhsIsNum := parseFloat(val)
	return func(r []string) (bool, error) {
		cur := ""
		if idx < len(r) {
			cur = r[idx]
		}
		if rhsIsNum {
			if lhsNum, ok := parseFloat(cur); ok {
				return cmpFloat(lhsNum, rhsNum, op), nil
			}
		}
		return cmpString(cur, val, op), nil
	}, nil
}

func parseFloat(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil
}

func cmpFloat(a, b float64, op string) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	}
	return false
}

func cmpString(a, b, op string) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	}
	return false
}

func unquote(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func rowToMap(header, row []string) map[string]any {
	out := make(map[string]any, len(header))
	for i, h := range header {
		v := ""
		if i < len(row) {
			v = row[i]
		}
		out[h] = v
	}
	return out
}

func mapsToRows(arr []map[string]any) ([]string, [][]string) {
	keySet := map[string]struct{}{}
	for _, m := range arr {
		for k := range m {
			keySet[k] = struct{}{}
		}
	}
	header := make([]string, 0, len(keySet))
	for k := range keySet {
		header = append(header, k)
	}
	sort.Strings(header)
	rows := make([][]string, 0, len(arr))
	for _, m := range arr {
		r := make([]string, len(header))
		for i, h := range header {
			if v, ok := m[h]; ok {
				r[i] = anyToString(v)
			}
		}
		rows = append(rows, r)
	}
	return header, rows
}

func anyToString(v any) string {
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
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func init() { command.RegisterBuiltin(New()) }
