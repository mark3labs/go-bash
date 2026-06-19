// Package ringbuf provides a bounded output writer used by the bash
// runtime to enforce SPEC §2.1's MaxOutputSize limit.
//
// Despite the package name (which mirrors SPEC §0.4's layout table), the
// current implementation is a forward-only limited writer; the "ring"
// semantics — where over-cap writes overwrite the oldest bytes — are not
// needed for the limit-enforcement path. The package name is kept so the
// import path matches the spec; a true ring-buffered variant can land
// later under the same package without breaking the limit wiring.
package ringbuf

import (
	"io"
	"sync"
	"sync/atomic"
)

// Tracker holds the shared output budget enforced by one or more
// LimitedWriter instances. Stdout and stderr both wrap the same Tracker
// so MaxOutputSize caps the combined output of a single Exec call.
//
// All methods are safe for concurrent use.
type Tracker struct {
	limit   int64
	written atomic.Int64

	once     sync.Once
	overflow atomic.Bool
	onOver   func(limit int64) error
	overErr  atomic.Value // holds error; set inside once.Do
}

// NewTracker returns a Tracker enforcing the given byte limit. onOver is
// invoked exactly once, the first time a write would push the cumulative
// count past limit; its return value is the error returned from
// subsequent Write calls.
//
// A non-positive limit is treated as "no limit" — Write always succeeds
// and onOver is never called.
func NewTracker(limit int64, onOver func(limit int64) error) *Tracker {
	return &Tracker{limit: limit, onOver: onOver}
}

// Written returns the total bytes written across every LimitedWriter
// sharing this Tracker.
func (t *Tracker) Written() int64 { return t.written.Load() }

// Limit returns the configured cap.
func (t *Tracker) Limit() int64 { return t.limit }

// Overflowed reports whether the limit has been tripped.
func (t *Tracker) Overflowed() bool { return t.overflow.Load() }

// Err returns the error returned by the onOver callback the first time
// the limit was tripped, or nil if the tracker has not overflowed.
func (t *Tracker) Err() error {
	if v := t.overErr.Load(); v != nil {
		if err, ok := v.(error); ok {
			return err
		}
	}
	return nil
}

// trip is called the first time the limit is exceeded.
func (t *Tracker) trip() error {
	t.once.Do(func() {
		t.overflow.Store(true)
		var err error
		if t.onOver != nil {
			err = t.onOver(t.limit)
		}
		if err != nil {
			t.overErr.Store(err)
		}
	})
	return t.Err()
}

// LimitedWriter wraps an io.Writer and accounts every byte written to it
// against a shared Tracker.
//
// Once the Tracker overflows, further Write calls write nothing and
// return the Tracker's overflow error. Partial writes are still flushed
// to the underlying writer up to the remaining budget on the call that
// trips the limit, so the caller's captured output reflects what would
// have been written had there been room.
type LimitedWriter struct {
	w io.Writer
	t *Tracker
}

// NewLimitedWriter wraps w to enforce t's limit. If t is nil the limit is
// not enforced and writes pass through unchanged.
func NewLimitedWriter(w io.Writer, t *Tracker) *LimitedWriter {
	return &LimitedWriter{w: w, t: t}
}

// Write implements io.Writer. It returns the number of bytes actually
// written to the underlying writer. When the Tracker's limit is tripped,
// Write returns the Tracker's overflow error (n may be > 0 for the call
// that crosses the boundary).
func (l *LimitedWriter) Write(p []byte) (int, error) {
	if l.t == nil || l.t.limit <= 0 {
		return l.w.Write(p)
	}
	if l.t.overflow.Load() {
		return 0, l.t.Err()
	}
	if len(p) == 0 {
		return 0, nil
	}
	current := l.t.written.Load()
	remaining := l.t.limit - current
	if remaining <= 0 {
		return 0, l.t.trip()
	}
	overflow := false
	write := p
	if int64(len(p)) > remaining {
		write = p[:remaining]
		overflow = true
	}
	n, err := l.w.Write(write)
	if n > 0 {
		l.t.written.Add(int64(n))
	}
	if err != nil {
		return n, err
	}
	if overflow {
		return n, l.t.trip()
	}
	return n, nil
}
