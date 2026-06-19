package ringbuf_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/mark3labs/go-bash/internal/ringbuf"
)

func TestLimitedWriterUnderLimit(t *testing.T) {
	var buf bytes.Buffer
	tr := ringbuf.NewTracker(100, func(int64) error { return errors.New("nope") })
	lw := ringbuf.NewLimitedWriter(&buf, tr)
	n, err := lw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write err = %v; want nil", err)
	}
	if n != 5 {
		t.Errorf("n = %d; want 5", n)
	}
	if buf.String() != "hello" {
		t.Errorf("buf = %q; want %q", buf.String(), "hello")
	}
	if tr.Written() != 5 {
		t.Errorf("Written = %d; want 5", tr.Written())
	}
	if tr.Overflowed() {
		t.Error("Overflowed = true; want false")
	}
}

func TestLimitedWriterTripsOnExactBoundary(t *testing.T) {
	var buf bytes.Buffer
	limitErr := errors.New("limit hit")
	tr := ringbuf.NewTracker(5, func(int64) error { return limitErr })
	lw := ringbuf.NewLimitedWriter(&buf, tr)
	// First write fills the budget exactly.
	if _, err := lw.Write([]byte("hello")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	// Second write should overflow and return the limit error.
	n, err := lw.Write([]byte("x"))
	if !errors.Is(err, limitErr) {
		t.Fatalf("Write err = %v; want %v", err, limitErr)
	}
	if n != 0 {
		t.Errorf("n = %d; want 0", n)
	}
	if got := buf.String(); got != "hello" {
		t.Errorf("buf = %q; want %q", got, "hello")
	}
}

func TestLimitedWriterPartialWriteOnOverflow(t *testing.T) {
	var buf bytes.Buffer
	limitErr := errors.New("limit hit")
	tr := ringbuf.NewTracker(3, func(int64) error { return limitErr })
	lw := ringbuf.NewLimitedWriter(&buf, tr)
	// Single 8-byte write must flush the first 3 bytes and trip.
	n, err := lw.Write([]byte("abcdefgh"))
	if !errors.Is(err, limitErr) {
		t.Fatalf("Write err = %v; want %v", err, limitErr)
	}
	if n != 3 {
		t.Errorf("n = %d; want 3", n)
	}
	if buf.String() != "abc" {
		t.Errorf("buf = %q; want %q", buf.String(), "abc")
	}
	if !tr.Overflowed() {
		t.Error("Overflowed = false; want true")
	}
}

func TestLimitedWriterStickyError(t *testing.T) {
	var buf bytes.Buffer
	limitErr := errors.New("once")
	calls := 0
	tr := ringbuf.NewTracker(2, func(int64) error {
		calls++
		return limitErr
	})
	lw := ringbuf.NewLimitedWriter(&buf, tr)
	_, _ = lw.Write([]byte("ab"))   // fills
	_, err1 := lw.Write([]byte("c")) // trips
	_, err2 := lw.Write([]byte("d")) // sticky
	if !errors.Is(err1, limitErr) || !errors.Is(err2, limitErr) {
		t.Fatalf("expected sticky %v; got %v / %v", limitErr, err1, err2)
	}
	if calls != 1 {
		t.Errorf("onOver called %d times; want exactly once", calls)
	}
}

func TestTrackerSharedAcrossWriters(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	limitErr := errors.New("shared")
	tr := ringbuf.NewTracker(5, func(int64) error { return limitErr })
	out := ringbuf.NewLimitedWriter(&outBuf, tr)
	er := ringbuf.NewLimitedWriter(&errBuf, tr)
	if _, err := out.Write([]byte("abc")); err != nil {
		t.Fatalf("out write: %v", err)
	}
	// Only 2 bytes of budget left; this write overflows.
	n, err := er.Write([]byte("xyz"))
	if !errors.Is(err, limitErr) {
		t.Fatalf("err = %v; want %v", err, limitErr)
	}
	if n != 2 {
		t.Errorf("n = %d; want 2", n)
	}
	if errBuf.String() != "xy" {
		t.Errorf("errBuf = %q; want %q", errBuf.String(), "xy")
	}
	if tr.Written() != 5 {
		t.Errorf("Written = %d; want 5", tr.Written())
	}
}

func TestLimitedWriterZeroLimitPassthrough(t *testing.T) {
	var buf bytes.Buffer
	tr := ringbuf.NewTracker(0, func(int64) error { return errors.New("never") })
	lw := ringbuf.NewLimitedWriter(&buf, tr)
	n, err := lw.Write([]byte("anything"))
	if err != nil {
		t.Fatalf("Write err = %v; want nil", err)
	}
	if n != 8 {
		t.Errorf("n = %d; want 8", n)
	}
}

func TestLimitedWriterNilTrackerPassthrough(t *testing.T) {
	var buf bytes.Buffer
	lw := ringbuf.NewLimitedWriter(&buf, nil)
	if _, err := lw.Write([]byte("x")); err != nil {
		t.Fatalf("Write err = %v; want nil", err)
	}
	if buf.String() != "x" {
		t.Errorf("buf = %q; want %q", buf.String(), "x")
	}
}
