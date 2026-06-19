package transform_test

import (
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/ast"
	"github.com/mark3labs/go-bash/parser"
	"github.com/mark3labs/go-bash/transform"
)

// roundTripScripts is the curated representative set: each script must
// survive Parse → Serialize → Parse with byte-identical Serialize
// output the second time around. Scripts cover Phase 4's parser
// surface; later phases extend the list as new constructs land.
var roundTripScripts = []string{
	`echo hello`,
	`echo "hello world"`,
	`echo 'single quoted'`,
	`echo $((1+2))`,
	`echo $((1+2)) | grep o`,
	`a && b || c`,
	`a; b; c`,
	`sleep 5 &`,
	`echo hi > out.txt`,
	`echo hi >> out.txt`,
	`echo hi 2>&1`,
	`cat < in.txt`,
	`echo "$VAR"`,
	`echo "${VAR:-default}"`,
	`FOO=bar echo hi`,
	`for i in 1 2 3; do echo "$i"; done`,
	`while true; do break; done`,
	`if [ -f x ]; then echo yes; else echo no; fi`,
	`case "$x" in a) echo one ;; b) echo two ;; *) echo other ;; esac`,
	`greet() { echo hi; }`,
	`(cd /tmp && ls)`,
	`{ echo a; echo b; }`,
	`echo $(date)`,
	"cat <<EOF\nhello\nworld\nEOF\n",
	`echo "${arr[0]}"`,
	`arr=(a b c)`,
	`declare -A m`,
	`[[ -n "$x" ]] && echo set`,
	`for ((i=0; i<3; i++)); do echo "$i"; done`,
	`x=1; ((x++))`,
}

func TestRoundTripCuratedSet(t *testing.T) {
	for _, src := range roundTripScripts {
		src := src
		t.Run(src, func(t *testing.T) {
			script, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out1, err := transform.Serialize(script)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}

			// Re-parse the output and serialize again — the second
			// Serialize must equal the first byte-for-byte.
			script2, err := parser.Parse(out1)
			if err != nil {
				t.Fatalf("re-Parse(%q): %v", out1, err)
			}
			out2, err := transform.Serialize(script2)
			if err != nil {
				t.Fatalf("Serialize round-trip: %v", err)
			}
			if out1 != out2 {
				t.Errorf("round-trip drift:\nsrc: %q\nout1: %q\nout2: %q", src, out1, out2)
			}
		})
	}
}

// TestSerializePluginSynthesized exercises the astToFile fallback used
// by transform plugins (Phase 13). For Phase 4 the inverse translator
// covers SimpleCommand + Word{Literal,Single,Double}; anything richer
// returns an error, which is the contract we want.
func TestSerializePluginSynthesized(t *testing.T) {
	// A hand-built script with no Origin field — equivalent to
	// `echo hello`.
	script := &ast.Script{
		Statements: []*ast.Statement{
			{
				Pipelines: []*ast.Pipeline{
					{
						Commands: []ast.Command{
							&ast.SimpleCommand{
								Name: &ast.Word{Parts: []ast.WordPart{&ast.Literal{Value: "echo"}}},
								Args: []*ast.Word{
									{Parts: []ast.WordPart{&ast.Literal{Value: "hello"}}},
								},
							},
						},
					},
				},
			},
		},
	}
	out, err := transform.Serialize(script)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if !strings.Contains(out, "echo hello") {
		t.Fatalf("expected output to contain 'echo hello', got %q", out)
	}
}

func TestSerializeNilScript(t *testing.T) {
	if _, err := transform.Serialize(nil); err == nil {
		t.Fatalf("expected error on nil script")
	}
}
