package gobash_test

import (
	"context"
	"strings"
	"testing"

	gobash "github.com/mark3labs/go-bash"
	"github.com/mark3labs/go-bash/transform"
	"github.com/mark3labs/go-bash/transform/plugins/collector"
	teeplugin "github.com/mark3labs/go-bash/transform/plugins/tee"
)

// callRecorder is a Phase-13 test plugin that records every Transform
// call and returns metadata so the host can assert the per-plugin
// payload survives all the way out to BashExecResult.Metadata.
type callRecorder struct {
	name  string
	calls *int
}

func (p *callRecorder) Name() string { return p.name }

func (p *callRecorder) Transform(ctx transform.Context) transform.Result {
	if p.calls != nil {
		*p.calls++
	}
	return transform.Result{
		AST: ctx.AST,
		Metadata: map[string]any{
			"called": true,
			"name":   p.name,
		},
	}
}

// TestPhase13PipelineNoOpWhenEmpty pins the spec acceptance:
// when zero plugins are registered, Bash.Exec runs unchanged and
// BashExecResult.Metadata is nil (no allocation).
func TestPhase13PipelineNoOpWhenEmpty(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := b.Exec(context.Background(), "echo hi", gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Stdout != "hi\n" {
		t.Fatalf("stdout: %q", res.Stdout)
	}
	if res.Metadata != nil {
		t.Fatalf("Metadata should be nil with no plugins: %v", res.Metadata)
	}
}

// TestPhase13PluginsFireInOrder pins the spec acceptance: plugins
// fire in registration order on every Exec call.
func TestPhase13PluginsFireInOrder(t *testing.T) {
	var firstCalls, secondCalls, thirdCalls int
	b, err := gobash.New(gobash.BashOptions{
		TransformPlugins: []transform.Plugin{
			&callRecorder{name: "first", calls: &firstCalls},
			&callRecorder{name: "second", calls: &secondCalls},
			&callRecorder{name: "third", calls: &thirdCalls},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Exec twice — every plugin must fire once per call.
	for i := 0; i < 2; i++ {
		res, err := b.Exec(context.Background(), "true", gobash.ExecOptions{})
		if err != nil {
			t.Fatalf("Exec %d: %v", i, err)
		}
		for _, name := range []string{"first", "second", "third"} {
			m, ok := res.Metadata[name].(map[string]any)
			if !ok {
				t.Fatalf("call %d: Metadata[%q] type %T", i, name, res.Metadata[name])
			}
			if m["called"] != true {
				t.Fatalf("call %d: Metadata[%q].called: %v", i, name, m["called"])
			}
		}
	}
	if firstCalls != 2 || secondCalls != 2 || thirdCalls != 2 {
		t.Fatalf("call counts: first=%d second=%d third=%d", firstCalls, secondCalls, thirdCalls)
	}
}

// TestPhase13RegisterTransformPlugin pins the late-binding API.
func TestPhase13RegisterTransformPlugin(t *testing.T) {
	var calls int
	b, err := gobash.New(gobash.BashOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, _ := b.Exec(context.Background(), "true", gobash.ExecOptions{})
	if res.Metadata != nil {
		t.Fatalf("before Register: Metadata = %v", res.Metadata)
	}

	b.RegisterTransformPlugin(&callRecorder{name: "late", calls: &calls})
	res, _ = b.Exec(context.Background(), "true", gobash.ExecOptions{})
	if calls != 1 {
		t.Fatalf("calls: %d", calls)
	}
	if _, ok := res.Metadata["late"]; !ok {
		t.Fatalf("Metadata[late] missing: %v", res.Metadata)
	}

	// Nil register is a no-op.
	b.RegisterTransformPlugin(nil)
	_, _ = b.Exec(context.Background(), "true", gobash.ExecOptions{})
	if calls != 2 {
		t.Fatalf("calls: %d", calls)
	}
}

// TestPhase13CommandCollectorViaExec pins the spec: the
// CommandCollectorPlugin's metadata surfaces from BashExecResult.
func TestPhase13CommandCollectorViaExec(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		TransformPlugins: []transform.Plugin{collector.New()},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := b.Exec(context.Background(), `echo hi | grep h && true`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	meta := res.Metadata[collector.Name].(map[string]any)
	got, ok := meta[collector.MetadataKey].([]string)
	if !ok {
		t.Fatalf("commands: %T", meta[collector.MetadataKey])
	}
	want := []string{"echo", "grep", "true"}
	if len(got) != len(want) {
		t.Fatalf("commands: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("commands[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestPhase13TeePluginViaExec pins the spec: the TeePlugin actually
// mirrors stdout into /OUT/<idx>-<cmd>.stdout.txt files within the
// Bash VFS.
func TestPhase13TeePluginViaExec(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		TransformPlugins: []transform.Plugin{
			teeplugin.New(teeplugin.Options{OutputDir: "/out"}),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.FS().MkdirAll("/out", 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	res, err := b.Exec(context.Background(), `echo hello`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	// stdout still flows to the caller; tee mirrors it.
	if res.Stdout != "hello\n" {
		t.Fatalf("stdout: %q", res.Stdout)
	}

	// The mirrored file lives in the VFS at /out/0-echo.stdout.txt.
	contents, err := b.FS().ReadFile("/out/0-echo.stdout.txt")
	if err != nil {
		t.Fatalf("readfile: %v", err)
	}
	if string(contents) != "hello\n" {
		t.Fatalf("mirrored: %q", contents)
	}

	// Metadata payload reflects the tee'd files.
	meta := res.Metadata[teeplugin.Name].(map[string]any)
	files := meta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 1 || files[0].StdoutFile != "/out/0-echo.stdout.txt" {
		t.Fatalf("files: %+v", files)
	}
}

// TestPhase13TeePluginMultiStagePipeline pins per-stage wrapping on
// a real Exec.
func TestPhase13TeePluginMultiStagePipeline(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		TransformPlugins: []transform.Plugin{
			teeplugin.New(teeplugin.Options{OutputDir: "/out"}),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.FS().MkdirAll("/out", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	res, err := b.Exec(context.Background(), `echo hello | tr a-z A-Z`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "HELLO" {
		t.Fatalf("stdout: %q", res.Stdout)
	}

	mirroredEcho, err := b.FS().ReadFile("/out/0-echo.stdout.txt")
	if err != nil {
		t.Fatalf("read 0: %v", err)
	}
	if string(mirroredEcho) != "hello\n" {
		t.Fatalf("0-echo: %q", mirroredEcho)
	}
	mirroredTr, err := b.FS().ReadFile("/out/1-tr.stdout.txt")
	if err != nil {
		t.Fatalf("read 1: %v", err)
	}
	if string(mirroredTr) != "HELLO\n" {
		t.Fatalf("1-tr: %q", mirroredTr)
	}
}

// TestPhase13MultiplePluginsCombine verifies that the collector and
// tee plugin compose cleanly. Each plugin's metadata appears under its
// own key.
func TestPhase13MultiplePluginsCombine(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		TransformPlugins: []transform.Plugin{
			collector.New(),
			teeplugin.New(teeplugin.Options{OutputDir: "/out"}),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.FS().MkdirAll("/out", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	res, err := b.Exec(context.Background(), `echo hi`, gobash.ExecOptions{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	collMeta := res.Metadata[collector.Name].(map[string]any)
	commands := collMeta[collector.MetadataKey].([]string)
	// The collector runs BEFORE the tee plugin, so it sees the
	// original `echo` only — not the post-tee `echo | tee`.
	if len(commands) != 1 || commands[0] != "echo" {
		t.Fatalf("collector saw post-tee tree: %v", commands)
	}

	teeMeta := res.Metadata[teeplugin.Name].(map[string]any)
	files := teeMeta[teeplugin.MetadataKey].([]teeplugin.TeeFileInfo)
	if len(files) != 1 {
		t.Fatalf("tee files: %+v", files)
	}
}

// TestPhase13PluginParseErrorBubblesUp pins that a parse error from
// the pipeline surfaces as gobash.ParseError (aka *parser.ParseError).
func TestPhase13PluginParseErrorBubblesUp(t *testing.T) {
	b, err := gobash.New(gobash.BashOptions{
		TransformPlugins: []transform.Plugin{collector.New()},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = b.Exec(context.Background(), "if then fi", gobash.ExecOptions{})
	if err == nil {
		t.Fatal("expected parse error")
	}
}
