package runtimestate

import (
	"testing"
)

func TestAliasTableSetGet(t *testing.T) {
	a := NewAliasTable()
	a.Set("ll", "ls -l")
	v, ok := a.Get("ll")
	if !ok || v != "ls -l" {
		t.Errorf("Get: got %q,%v", v, ok)
	}
}

func TestAliasTableUnset(t *testing.T) {
	a := NewAliasTable()
	a.Set("ll", "ls -l")
	if !a.Unset("ll") {
		t.Error("Unset returned false for existing key")
	}
	if a.Unset("nope") {
		t.Error("Unset returned true for missing key")
	}
}

func TestAliasTableNames(t *testing.T) {
	a := NewAliasTable()
	a.Set("b", "x")
	a.Set("a", "y")
	a.Set("c", "z")
	got := a.Names()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Names len: got=%d want=%d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Names[%d]: got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestAliasTableNilSafe(t *testing.T) {
	var a *AliasTable
	a.Set("x", "y")
	if _, ok := a.Get("x"); ok {
		t.Error("nil Get reported ok")
	}
	if a.Unset("x") {
		t.Error("nil Unset returned true")
	}
	if a.Names() != nil {
		t.Error("nil Names returned non-nil")
	}
	if a.All() != nil {
		t.Error("nil All returned non-nil")
	}
}

func TestHistoryRingAddList(t *testing.T) {
	h := NewHistoryRing(3)
	h.Add("a")
	h.Add("b")
	h.Add("c")
	seqs, cmds := h.List()
	if len(seqs) != 3 {
		t.Fatalf("len=%d", len(seqs))
	}
	if seqs[0] != 1 || seqs[2] != 3 {
		t.Errorf("seqs=%v", seqs)
	}
	if cmds[0] != "a" || cmds[2] != "c" {
		t.Errorf("cmds=%v", cmds)
	}
}

func TestHistoryRingEvict(t *testing.T) {
	h := NewHistoryRing(2)
	h.Add("a")
	h.Add("b")
	h.Add("c") // evicts "a"
	seqs, cmds := h.List()
	if len(seqs) != 2 {
		t.Fatalf("len=%d", len(seqs))
	}
	if seqs[0] != 2 || seqs[1] != 3 {
		t.Errorf("seqs=%v want=[2 3]", seqs)
	}
	if cmds[0] != "b" || cmds[1] != "c" {
		t.Errorf("cmds=%v want=[b c]", cmds)
	}
}

func TestHistoryRingClear(t *testing.T) {
	h := NewHistoryRing(3)
	h.Add("a")
	h.Add("b")
	h.Clear()
	if h.Len() != 0 {
		t.Error("Clear left entries")
	}
	h.Add("c")
	seqs, _ := h.List()
	if len(seqs) != 1 || seqs[0] != 3 {
		t.Errorf("post-clear seq=%v want=[3]", seqs)
	}
}

func TestHistoryRingNilSafe(t *testing.T) {
	var h *HistoryRing
	h.Add("x")
	if h.Len() != 0 {
		t.Error("nil Len != 0")
	}
	s, c := h.List()
	if s != nil || c != nil {
		t.Error("nil List returned non-nil")
	}
}

func TestHistoryRingDefaultsSize(t *testing.T) {
	h := NewHistoryRing(0)
	if h.size != DefaultHistorySize {
		t.Errorf("size=%d want=%d", h.size, DefaultHistorySize)
	}
}
