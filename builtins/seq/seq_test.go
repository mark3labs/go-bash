package seq_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/go-bash/builtins/seq"
	"github.com/mark3labs/go-bash/command"
)

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var o, e bytes.Buffer
	res := seq.New().Execute(context.Background(), append([]string{"seq"}, args...), &command.Context{Stdout: &o, Stderr: &e})
	return o.String(), e.String(), res.ExitCode
}

func TestSeqOneArg(t *testing.T) {
	out, _, code := run(t, "3")
	if code != 0 || out != "1\n2\n3\n" {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestSeqTwoArgs(t *testing.T) {
	out, _, _ := run(t, "2", "5")
	if out != "2\n3\n4\n5\n" {
		t.Errorf("out=%q", out)
	}
}

func TestSeqThreeArgs(t *testing.T) {
	out, _, _ := run(t, "1", "2", "7")
	if out != "1\n3\n5\n7\n" {
		t.Errorf("out=%q", out)
	}
}

func TestSeqDescending(t *testing.T) {
	out, _, _ := run(t, "5", "-1", "3")
	if out != "5\n4\n3\n" {
		t.Errorf("out=%q", out)
	}
}

func TestSeqSep(t *testing.T) {
	out, _, _ := run(t, "-s", ",", "1", "3")
	if out != "1,2,3\n" {
		t.Errorf("out=%q", out)
	}
}

func TestSeqWidth(t *testing.T) {
	out, _, _ := run(t, "-w", "8", "10")
	if out != "08\n09\n10\n" {
		t.Errorf("out=%q", out)
	}
}

func TestSeqFractional(t *testing.T) {
	out, _, _ := run(t, "1", "0.5", "2")
	if out != "1\n1.5\n2\n" {
		t.Errorf("out=%q", out)
	}
}

func TestSeqFormat(t *testing.T) {
	out, _, _ := run(t, "-f", "%.2f", "1", "3")
	if out != "1.00\n2.00\n3.00\n" {
		t.Errorf("out=%q", out)
	}
}

func TestSeqInvalid(t *testing.T) {
	_, err, code := run(t, "abc")
	if code != 1 || !strings.Contains(err, "invalid") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestSeqZeroIncrement(t *testing.T) {
	_, err, code := run(t, "1", "0", "5")
	if code != 1 || !strings.Contains(err, "invalid Zero increment") {
		t.Errorf("err=%q code=%d", err, code)
	}
}

func TestSeqHelp(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage: seq") {
		t.Errorf("out=%q code=%d", out, code)
	}
}

func TestSeqEmpty(t *testing.T) {
	// seq 5 3 with default incr 1 generates nothing.
	out, _, code := run(t, "5", "3")
	if code != 0 || out != "" {
		t.Errorf("out=%q code=%d", out, code)
	}
}
