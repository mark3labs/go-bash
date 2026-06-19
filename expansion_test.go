package gobash_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	gbfs "github.com/mark3labs/go-bash/fs"
)

// helper: run a script with default options, return stdout/stderr/exit.
func runScript(t *testing.T, b *gobash.Bash, src string) gobash.BashExecResult {
	t.Helper()
	res, err := b.Exec(context.Background(), src, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec(%q): %v", src, err)
	}
	return res
}

func newBash(t *testing.T) *gobash.Bash {
	t.Helper()
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// -------- §6.1 Variables / parameter expansion --------

func TestPhase6ParamExpansionBasic(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		// $VAR / ${VAR}
		{`X=hi; echo "$X-${X}"`, "hi-hi\n"},
		// ${VAR:-default}
		{`unset X; echo "${X:-fallback}"`, "fallback\n"},
		// ${VAR:=assign}
		{`unset X; echo "${X:=assigned}"; echo "$X"`, "assigned\nassigned\n"},
		// ${VAR:+alt}
		{`X=set; echo "${X:+alternate}"; unset X; echo "${X:+alternate}"`, "alternate\n\n"},
		// ${VAR:offset:length}
		{`X=abcdef; echo "${X:1:3}"`, "bcd\n"},
		// ${#VAR}
		{`X=hello; echo "${#X}"`, "5\n"},
		// ${VAR/pat/repl} (first only)
		{`X=aXbXc; echo "${X/X/Z}"`, "aZbXc\n"},
		// ${VAR//pat/repl} (all)
		{`X=aXbXc; echo "${X//X/Z}"`, "aZbZc\n"},
		// ${VAR#pat}, ${VAR##pat}
		{`X=a.b.c; echo "${X#*.}"; echo "${X##*.}"`, "b.c\nc\n"},
		// ${VAR%pat}, ${VAR%%pat}
		{`X=a.b.c; echo "${X%.*}"; echo "${X%%.*}"`, "a.b\na\n"},
		// case conversions ${VAR^^}, ${VAR^}, ${VAR,,}, ${VAR,}
		{`X=hello; echo "${X^^}"; echo "${X^}"`, "HELLO\nHello\n"},
		{`X=HELLO; echo "${X,,}"; echo "${X,}"`, "hello\nhELLO\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

func TestPhase6ParamExpansionErrorOp(t *testing.T) {
	// ${VAR:?error} → non-zero exit + stderr message when unset.
	b := newBash(t)
	res, err := b.Exec(context.Background(),
		`unset X; echo "${X:?was unset}"`,
		gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("ExitCode = 0; want non-zero (unset with :?)")
	}
	if !strings.Contains(res.Stderr, "was unset") {
		t.Errorf("Stderr = %q; expected to contain 'was unset'", res.Stderr)
	}
}

// TestPhase6ParamExpansionAnchorDivergence locks in the known mvdan/sh
// divergence: ${VAR/#pat/repl} and ${VAR/%pat/repl} are silently treated
// as no-op replacements instead of anchored ones. Real bash anchors at
// the string start (#) or end (%). The fix lives in a later phase via
// pre-Parse rewriting; this test pins the current behavior so any
// upstream mvdan/sh fix is detected.
func TestPhase6ParamExpansionAnchorDivergence(t *testing.T) {
	b := newBash(t)
	res := runScript(t, b, `X=abcabc; echo "${X/#abc/XX}"`)
	// Real bash would print "XXabc\n"; mvdan/sh leaves the string
	// untouched. Recorded in DECISIONS.md and tracked for a later
	// pre-Parse rewriter that lands the actual fix.
	if res.Stdout != "abcabc\n" {
		t.Errorf("divergence regression: Stdout = %q (mvdan/sh used to no-op anchor)", res.Stdout)
	}
}

func TestPhase6ParamExpansionArrays(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		{`arr=(a b c); echo "${arr[1]}"`, "b\n"},
		{`arr=(a b c); echo "${arr[@]}"`, "a b c\n"},
		{`arr=(a b c); echo "${arr[*]}"`, "a b c\n"},
		{`arr=(a b c); echo "${#arr[@]}"`, "3\n"},
		{`arr=(a b c); for k in "${!arr[@]}"; do echo "$k"; done`, "0\n1\n2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

// -------- §6.2 Positional parameters --------

func TestPhase6Positional(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		{`set -- one two three; echo "$1-$2-$3"`, "one-two-three\n"},
		{`set -- a b c; echo "$#"`, "3\n"},
		{`set -- a b c; echo "$@"`, "a b c\n"},
		{`set -- a b c; shift; echo "$1-$2"`, "b-c\n"},
		{`set -- a b c d; shift 2; echo "$1-$2"`, "c-d\n"},
		// "$@" expands per element; "$*" joins with IFS first char.
		// mvdan/sh divergence: IFS is honored at command-arg expansion
		// time (echo "$*") but NOT at assignment time (out="$*"). We
		// exercise the arg form here. See DECISIONS.md.
		{`IFS=,; set -- a b c; echo "$*"`, "a,b,c\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

// -------- §6.4 Brace expansion --------

func TestPhase6BraceExpansion(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		// comma form
		{`echo {a,b,c}`, "a b c\n"},
		// sequence form
		{`echo {1..5}`, "1 2 3 4 5\n"},
		// sequence with step
		{`echo {1..10..2}`, "1 3 5 7 9\n"},
		// char range
		{`echo {a..e}`, "a b c d e\n"},
		// reversed range
		{`echo {5..1}`, "5 4 3 2 1\n"},
		// prefix/suffix
		{`echo pre{1,2}suf`, "pre1suf pre2suf\n"},
		// nested
		{`echo {a,b{1,2}}`, "a b1 b2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

func TestPhase6BraceExpansionCapTrips(t *testing.T) {
	// MaxBraceExpansionResults default is 10000. {1..20000} → 20001
	// elements, which must fail with *ExecutionLimitError before any
	// runtime allocation.
	b := newBash(t)
	_, err := b.Exec(context.Background(), `echo {1..20000}`, gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected MaxBraceExpansionResults error; got nil")
	}
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) {
		t.Fatalf("err = %T %v; want *ExecutionLimitError", err, err)
	}
	if ele.Limit != "MaxBraceExpansionResults" {
		t.Errorf("Limit = %q; want MaxBraceExpansionResults", ele.Limit)
	}
}

func TestPhase6BraceExpansionCapNestedTrips(t *testing.T) {
	// Nested product: {1..100}{a,b,c,d,e}{1..30} = 100*5*30 = 15000.
	b := newBash(t)
	_, err := b.Exec(context.Background(),
		`x={1..100}{a,b,c,d,e}{1..30}`, gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected MaxBraceExpansionResults error; got nil")
	}
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) || ele.Limit != "MaxBraceExpansionResults" {
		t.Fatalf("err = %v; want MaxBraceExpansionResults", err)
	}
}

func TestPhase6BraceExpansionAbsurdSequenceSaturates(t *testing.T) {
	// {1..2000000000} would OOM if expanded; the static analysis must
	// short-circuit via the cap+1 saturation path.
	b := newBash(t)
	_, err := b.Exec(context.Background(),
		`echo {1..2000000000}`, gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected MaxBraceExpansionResults error; got nil")
	}
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) || ele.Limit != "MaxBraceExpansionResults" {
		t.Fatalf("err = %v; want MaxBraceExpansionResults", err)
	}
}

// -------- §6.5 Globs / §6.6 Pathname expansion --------

func TestPhase6GlobBasic(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/work/a.txt":      {Content: []byte("a")},
			"/work/b.txt":      {Content: []byte("b")},
			"/work/c.log":      {Content: []byte("c")},
			"/work/sub/d.txt":  {Content: []byte("d")},
		},
		Cwd: "/work",
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		src  string
		want string
	}{
		// *.txt → a.txt b.txt
		{`for f in *.txt; do echo "$f"; done`, "a.txt\nb.txt\n"},
		// ? — single char (no a-z.txt match because more than one
		// char before .txt). Use [ab].txt instead.
		{`for f in [ab].txt; do echo "$f"; done`, "a.txt\nb.txt\n"},
		// !-class
		{`for f in [!c]*.txt; do echo "$f"; done`, "a.txt\nb.txt\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

func TestPhase6GlobMaxGlobOperations(t *testing.T) {
	// Seed enough directories that a recursive ** glob would exceed
	// MaxGlobOperations=5. We override the limit to a tiny value and
	// trigger several ReadDirs.
	files := map[string]gbfs.FileInit{}
	for i := 0; i < 20; i++ {
		files[fmt.Sprintf("/work/d%d", i)] = gbfs.FileInit{Dir: true}
		files[fmt.Sprintf("/work/d%d/x.txt", i)] = gbfs.FileInit{Content: []byte("x")}
	}
	low := 3
	b, err := gobash.New(gobash.BashOptions{
		Files:           files,
		Cwd:             "/work",
		ExecutionLimits: &gobash.ExecutionLimits{MaxGlobOperations: &low},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(context.Background(),
		`shopt -s globstar; for f in **/*.txt; do :; done`,
		gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected MaxGlobOperations error; got nil")
	}
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) || ele.Limit != "MaxGlobOperations" {
		t.Fatalf("err = %v; want MaxGlobOperations", err)
	}
}

// -------- §6.7 Arithmetic --------

func TestPhase6Arithmetic(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		{`echo $((1+2))`, "3\n"},
		{`echo $((10-3))`, "7\n"},
		{`echo $((4*5))`, "20\n"},
		{`echo $((20/3))`, "6\n"},
		{`echo $((20%3))`, "2\n"},
		{`echo $((2**10))`, "1024\n"},
		{`echo $((1<<4))`, "16\n"},
		{`echo $((16>>2))`, "4\n"},
		{`echo $((5&3))`, "1\n"},
		{`echo $((5|3))`, "7\n"},
		{`echo $((5^3))`, "6\n"},
		// comparison + logical
		{`echo $((3 > 2))`, "1\n"},
		{`echo $((3 < 2))`, "0\n"},
		{`echo $((1 && 0))`, "0\n"},
		{`echo $((1 || 0))`, "1\n"},
		// ternary
		{`echo $(( 1 ? 42 : 0 ))`, "42\n"},
		// assignments
		{`i=0; i=$((i+1)); i=$((i+1)); echo $i`, "2\n"},
		// compound
		{`i=5; ((i+=10)); echo $i`, "15\n"},
		// pre/post increment
		{`i=5; echo $((++i)); echo $i; echo $((i++)); echo $i`, "6\n6\n6\n7\n"},
		// (( cmd ))
		{`if (( 5 > 3 )); then echo yes; else echo no; fi`, "yes\n"},
		// arithmetic context in array index
		{`arr=(a b c d e); i=2; echo "${arr[$((i+1))]}"`, "d\n"},
		// let assignment (mvdan/sh quirk: `let "x = expr"` with spaces
		// around the assignment does NOT persist; using the no-space
		// form does. See DECISIONS.md.)
		{`let x=4*3; echo $x`, "12\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

func TestPhase6ArithmeticDivByZero(t *testing.T) {
	// mvdan/sh writes "division by zero" to stderr but currently
	// exits 0. Real bash exits non-zero. We pin the visible-error
	// behavior here; a future phase will patch the exit code via an
	// arithmetic-error hook. See DECISIONS.md.
	b := newBash(t)
	res, err := b.Exec(context.Background(), `echo $((1/0))`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !strings.Contains(res.Stderr, "division by zero") {
		t.Errorf("Stderr = %q; expected to contain 'division by zero'", res.Stderr)
	}
}

// -------- §6.8 Conditionals --------

func TestPhase6TestSingleBracket(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		{`if [ "$X" = "" ]; then echo empty; else echo nonempty; fi`, "empty\n"},
		{`if [ 5 -gt 3 ]; then echo gt; fi`, "gt\n"},
		{`if [ 5 -lt 3 ]; then echo lt; else echo nlt; fi`, "nlt\n"},
		{`if [ -z "" ]; then echo z; fi`, "z\n"},
		{`if [ -n "abc" ]; then echo n; fi`, "n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

func TestPhase6TestDoubleBracket(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		{`if [[ "abc" == abc ]]; then echo eq; fi`, "eq\n"},
		{`if [[ "abc" != xyz ]]; then echo neq; fi`, "neq\n"},
		// pattern match
		{`if [[ "abc123" == abc* ]]; then echo pat; fi`, "pat\n"},
		// regex
		{`if [[ "hello42world" =~ ^hello([0-9]+)world$ ]]; then echo "${BASH_REMATCH[1]}"; fi`, "42\n"},
		// short circuit && / ||
		{`if [[ 1 -eq 1 && 2 -eq 2 ]]; then echo and; fi`, "and\n"},
		{`if [[ 1 -eq 0 || 2 -eq 2 ]]; then echo or; fi`, "or\n"},
		// lex compare
		{`if [[ a < b ]]; then echo lt; fi`, "lt\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

func TestPhase6TestFileFlags(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/work/file.txt":  {Content: []byte("data")},
			"/work/empty.txt": {Content: []byte{}},
			"/work/sub":       {Dir: true},
		},
		Cwd: "/work",
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		src  string
		want string
	}{
		{`[[ -e /work/file.txt ]] && echo e`, "e\n"},
		{`[[ -f /work/file.txt ]] && echo f`, "f\n"},
		{`[[ -d /work/sub ]] && echo d`, "d\n"},
		{`[[ -s /work/file.txt ]] && echo s`, "s\n"},
		{`[[ ! -s /work/empty.txt ]] && echo empty`, "empty\n"},
		{`[[ -e /work/missing ]] || echo missing`, "missing\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

// -------- §6.9 Redirections --------

func TestPhase6Redirections(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		Files: map[string]gbfs.FileInit{
			"/work": {Dir: true},
		},
		Cwd: "/work",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Truncating > and append >>
	res := runScript(t, b, `echo first > out.txt; echo second >> out.txt; read l1 < out.txt; read l2 << "EOF"
inline
EOF
echo "$l1|$l2"`)
	if res.Stdout != "first|inline\n" {
		t.Errorf("Stdout = %q", res.Stdout)
	}
	// here-string <<<
	res = runScript(t, b, `read x <<< "from hstr"; echo "$x"`)
	if res.Stdout != "from hstr\n" {
		t.Errorf("here-string Stdout = %q", res.Stdout)
	}
	// stderr -> stdout merge 2>&1
	res = runScript(t, b, `(echo to_out; echo to_err 1>&2) 2>&1`)
	if !strings.Contains(res.Stdout, "to_out") || !strings.Contains(res.Stdout, "to_err") {
		t.Errorf("2>&1 merge Stdout = %q", res.Stdout)
	}
	// &> form
	res = runScript(t, b, `(echo combined; echo also 1>&2) &> merged.txt; read x < merged.txt; echo $x`)
	if !strings.HasPrefix(res.Stdout, "combined") && !strings.HasPrefix(res.Stdout, "also") {
		t.Errorf("&> Stdout = %q (expected at least one merged line)", res.Stdout)
	}
}

func TestPhase6HeredocStripsTabs(t *testing.T) {
	b := newBash(t)
	src := "cat() { while IFS= read -r l; do echo \"$l\"; done; }\n" +
		"cat <<-EOF\n\thello\n\tworld\nEOF\n"
	// Note: our cat shim above is a function that reads stdin until EOF.
	res := runScript(t, b, src)
	if res.Stdout != "hello\nworld\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "hello\nworld\n")
	}
}

func TestPhase6ProcessSubstitution(t *testing.T) {
	b := newBash(t)
	// mvdan/sh supports process substitution via /dev/fd or named pipes
	// depending on platform. Pipe the output of a subshell into a
	// reader via < <(...).
	res := runScript(t, b,
		`while IFS= read -r line; do echo "got:$line"; done < <(echo one; echo two)`)
	if res.Stdout != "got:one\ngot:two\n" {
		t.Errorf("Stdout = %q; want %q", res.Stdout, "got:one\ngot:two\n")
	}
}

// -------- §6.10 Control flow --------

func TestPhase6ControlFlow(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		// if/elif/else
		{`if [ 1 -gt 2 ]; then echo a; elif [ 1 -lt 2 ]; then echo b; else echo c; fi`, "b\n"},
		// for x in ...
		{`for x in a b c; do echo $x; done`, "a\nb\nc\n"},
		// for ((init; cond; post))
		{`for ((i=0; i<3; i++)); do echo $i; done`, "0\n1\n2\n"},
		// while
		{`i=0; while [ $i -lt 3 ]; do echo $i; i=$((i+1)); done`, "0\n1\n2\n"},
		// until
		{`i=0; until [ $i -ge 3 ]; do echo $i; i=$((i+1)); done`, "0\n1\n2\n"},
		// case
		{`case foo in foo) echo a;; *) echo b;; esac`, "a\n"},
		{`case bar in foo) echo a;; *) echo b;; esac`, "b\n"},
		// break
		{`for i in 1 2 3 4 5; do if [ $i -eq 3 ]; then break; fi; echo $i; done`, "1\n2\n"},
		// break N
		{`for i in 1 2; do for j in a b; do if [ "$j" = b ]; then break 2; fi; echo $i$j; done; done`, "1a\n"},
		// continue
		{`for i in 1 2 3; do if [ $i -eq 2 ]; then continue; fi; echo $i; done`, "1\n3\n"},
		// && / ||
		{`true && echo yes; false && echo no; false || echo or-yes`, "yes\nor-yes\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

// -------- §6.10 Command substitution --------

func TestPhase6CommandSubstitution(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		// $(...)
		{`echo "$(echo nested)"`, "nested\n"},
		// nested $(...)
		{`echo "$(echo $(echo deep))"`, "deep\n"},
		// backticks
		{"echo `echo backtick`", "backtick\n"},
		// inside other expansions
		{`X=$(echo hello); echo "$X"`, "hello\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

func TestPhase6SubstitutionDepthCapTrips(t *testing.T) {
	// Override MaxSubstitutionDepth to 3 so the test fits comfortably
	// under MaxParserDepth=200. Build a chain of 6 nested $(...).
	low := 3
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{MaxSubstitutionDepth: &low},
	})
	if err != nil {
		t.Fatal(err)
	}
	src := "echo " + strings.Repeat("$(", 6) + "echo deep" + strings.Repeat(")", 6)
	_, err = b.Exec(context.Background(), src, gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected MaxSubstitutionDepth error; got nil")
	}
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) || ele.Limit != "MaxSubstitutionDepth" {
		t.Fatalf("err = %v; want MaxSubstitutionDepth", err)
	}
}

func TestPhase6SubstitutionDepthShallowPasses(t *testing.T) {
	// A depth-2 nesting must NOT trip with the default cap of 50.
	b := newBash(t)
	res := runScript(t, b, `echo "$(echo $(echo deep))"`)
	if res.Stdout != "deep\n" {
		t.Errorf("Stdout = %q", res.Stdout)
	}
}

// -------- §6.12 Functions --------

func TestPhase6Functions(t *testing.T) {
	b := newBash(t)
	cases := []struct {
		src  string
		want string
	}{
		// classic form
		{`foo() { echo "hi $1"; }; foo bar`, "hi bar\n"},
		// keyword form
		{`function foo { echo "kw $1"; }; foo bar`, "kw bar\n"},
		// local vars
		{`x=outer; foo() { local x=inner; echo $x; }; foo; echo $x`, "inner\nouter\n"},
		// return N
		{`foo() { return 7; }; foo; echo $?`, "7\n"},
		// recursion with arithmetic
		{`fact() { if [ $1 -le 1 ]; then echo 1; else echo $(( $1 * $(fact $(($1 - 1))) )); fi; }; fact 5`, "120\n"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			res := runScript(t, b, tc.src)
			if res.Stdout != tc.want {
				t.Errorf("Stdout = %q; want %q", res.Stdout, tc.want)
			}
		})
	}
}

// -------- runtime caps --------

func TestPhase6MaxStringLengthCap(t *testing.T) {
	// MaxStringLength default is 10MiB. Override to 16 and trip with
	// a 20-char arg.
	low := 16
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{MaxStringLength: &low},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(context.Background(),
		`x=$(printf '%s' "aaaaaaaaaaaaaaaaaaaaaaaaaa"); echo "$x"`,
		gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected MaxStringLength error; got nil")
	}
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) || ele.Limit != "MaxStringLength" {
		t.Fatalf("err = %v; want MaxStringLength", err)
	}
}

func TestPhase6MaxArrayElementsCap(t *testing.T) {
	low := 5
	b, err := gobash.New(gobash.BashOptions{
		ExecutionLimits: &gobash.ExecutionLimits{MaxArrayElements: &low},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Exec(context.Background(),
		`arr=(a b c d e f g h)`,
		gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected MaxArrayElements error; got nil")
	}
	var ele *gobash.ExecutionLimitError
	if !errors.As(err, &ele) || ele.Limit != "MaxArrayElements" {
		t.Fatalf("err = %v; want MaxArrayElements", err)
	}
}

func TestPhase6CapsDoNotImpactNormalScripts(t *testing.T) {
	// A vanilla script with small expansions, depth 2 subst, 3 array
	// elements must execute without tripping any §6 cap.
	b := newBash(t)
	res := runScript(t, b,
		`arr=(x y z); for e in "${arr[@]}"; do echo "$(echo got-$e)"; done`)
	want := "got-x\ngot-y\ngot-z\n"
	if res.Stdout != want {
		t.Errorf("Stdout = %q; want %q", res.Stdout, want)
	}
}
