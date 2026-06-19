// Package runtimestate carries the per-Bash mutable state shared
// between built-in commands and the runtime.
//
// Phase 10 Wave G introduces two carriers:
//
//   - AliasTable: shell aliases set via `alias NAME=VALUE` and removed
//     via `unalias NAME`. The table is consulted at parse time by the
//     Phase 11 alias-expansion path (not yet wired) when the
//     `expand_aliases` shopt is on; Wave G implements only the
//     read/write surface so scripts can observe the table.
//   - HistoryRing: a bounded ring buffer of the most recent commands
//     for the `history` built-in.
//
// Both types are concurrency-safe. They live in an internal package so
// command.Context can hold typed pointers to them without pulling the
// gobash root package (which would create an import cycle).
package runtimestate

import (
	"sort"
	"sync"
)

// DefaultHistorySize is the spec Wave G ring capacity for the
// `history` built-in (last 500 commands).
const DefaultHistorySize = 500

// AliasTable is the per-Bash alias map. It is safe for concurrent use.
type AliasTable struct {
	mu sync.RWMutex
	m  map[string]string
}

// NewAliasTable returns an empty alias table.
func NewAliasTable() *AliasTable { return &AliasTable{m: make(map[string]string)} }

// Set assigns value to name, replacing any prior entry. Empty name is
// a no-op (mirrors bash's `alias =foo` rejection at the parser).
func (a *AliasTable) Set(name, value string) {
	if a == nil || name == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m == nil {
		a.m = make(map[string]string)
	}
	a.m[name] = value
}

// Get returns the value bound to name and whether it was present.
func (a *AliasTable) Get(name string) (string, bool) {
	if a == nil {
		return "", false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	v, ok := a.m[name]
	return v, ok
}

// Unset removes name from the table and reports whether it had been
// present. Removing a missing name is a no-op (returns false).
func (a *AliasTable) Unset(name string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.m[name]; !ok {
		return false
	}
	delete(a.m, name)
	return true
}

// Clear removes every entry.
func (a *AliasTable) Clear() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m = make(map[string]string)
}

// Names returns every alias name, sorted alphabetically.
func (a *AliasTable) Names() []string {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]string, 0, len(a.m))
	for k := range a.m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// All returns a snapshot of every binding. The returned map is owned
// by the caller — mutating it does not affect the table.
func (a *AliasTable) All() map[string]string {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]string, len(a.m))
	for k, v := range a.m {
		out[k] = v
	}
	return out
}

// HistoryRing is a bounded ring buffer of recent commands keyed by
// monotonic 1-based sequence numbers (matching `history`'s output).
// Safe for concurrent use.
type HistoryRing struct {
	mu   sync.Mutex
	buf  []string
	size int
	next int // 1-based sequence of the NEXT Add
}

// NewHistoryRing returns a ring of the given capacity. A non-positive
// size defaults to DefaultHistorySize.
func NewHistoryRing(size int) *HistoryRing {
	if size <= 0 {
		size = DefaultHistorySize
	}
	return &HistoryRing{size: size, next: 1}
}

// Add records cmd, evicting the oldest entry when the ring is full.
// Empty cmd is dropped.
func (h *HistoryRing) Add(cmd string) {
	if h == nil || cmd == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.buf) < h.size {
		h.buf = append(h.buf, cmd)
	} else {
		// Drop the oldest entry. Append + slice avoids a ring index
		// at the cost of a copy; the buffer is bounded (default 500
		// entries) so this is fine.
		copy(h.buf, h.buf[1:])
		h.buf[len(h.buf)-1] = cmd
	}
	h.next++
}

// List returns every recorded entry paired with its 1-based sequence
// number. The first return slice is parallel to the second.
//
// The returned slices are owned by the caller.
func (h *HistoryRing) List() (seqs []int, cmds []string) {
	if h == nil {
		return nil, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n := len(h.buf)
	if n == 0 {
		return nil, nil
	}
	first := h.next - n
	seqs = make([]int, n)
	cmds = make([]string, n)
	for i := 0; i < n; i++ {
		seqs[i] = first + i
		cmds[i] = h.buf[i]
	}
	return seqs, cmds
}

// Len returns the number of entries currently in the ring.
func (h *HistoryRing) Len() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.buf)
}

// Clear empties the ring but does NOT reset the sequence counter,
// matching `history -c` behavior in real bash.
func (h *HistoryRing) Clear() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf = h.buf[:0]
}

// ShoptTable is the per-Bash shell-option table (`shopt`). Each
// option is a boolean. nil is treated as empty (every option off,
// writes dropped). Safe for concurrent use.
type ShoptTable struct {
	mu sync.RWMutex
	m  map[string]bool
}

// NewShoptTable returns an empty shopt table. Per the spec the
// runtime preseeds a small set of defaults; callers do that
// explicitly via Set after construction.
func NewShoptTable() *ShoptTable { return &ShoptTable{m: make(map[string]bool)} }

// IsSet reports whether name is on. An unset option is reported as off.
func (s *ShoptTable) IsSet(name string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[name]
}

// Set writes on against name. Empty name is a no-op.
func (s *ShoptTable) Set(name string, on bool) {
	if s == nil || name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = make(map[string]bool)
	}
	s.m[name] = on
}

// Names returns every known option (whether on or off), sorted.
func (s *ShoptTable) Names() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
