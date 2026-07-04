package value

import "testing"

// TestSetAddDedupAndOrder proves Add inserts a new member, leaves a member already
// present untouched, and keeps insertion order across both, the three things a
// JavaScript Set guarantees that a plain append would not. The size after the
// duplicate add stays at the distinct count, and Range visits the members in the
// order they first entered.
func TestSetAddDedupAndOrder(t *testing.T) {
	s := NewNumberSet()
	s.Add(3).Add(1).Add(3).Add(2).Add(1)
	if got := s.Size(); got != 3 {
		t.Fatalf("size after dedup = %v, want 3", got)
	}
	var order []float64
	s.Range(func(v float64) { order = append(order, v) })
	want := []float64{3, 1, 2}
	if len(order) != len(want) {
		t.Fatalf("range visited %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("range order = %v, want %v", order, want)
		}
	}
}

// TestSetAddReturnsReceiver proves Add returns the set itself so a chained add
// lowers with no temporary, and that the chain mutates one set rather than copies.
func TestSetAddReturnsReceiver(t *testing.T) {
	s := NewNumberSet()
	if s.Add(1) != s {
		t.Fatal("Add did not return the receiver")
	}
}

// TestSetHasAndDelete proves Has reports membership, Delete drops a member and
// reports it was present, deleting an absent member reports false and changes
// nothing, and the members that survive a delete keep their relative order.
func TestSetHasAndDelete(t *testing.T) {
	s := NewStringSet()
	s.Add(FromGoString("a")).Add(FromGoString("b")).Add(FromGoString("c"))
	if !s.Has(FromGoString("b")) {
		t.Fatal("Has(b) = false, want true")
	}
	if !s.Delete(FromGoString("b")) {
		t.Fatal("Delete(b) = false, want true")
	}
	if s.Has(FromGoString("b")) {
		t.Fatal("Has(b) after delete = true, want false")
	}
	if s.Delete(FromGoString("b")) {
		t.Fatal("Delete of an absent member = true, want false")
	}
	if got := s.Size(); got != 2 {
		t.Fatalf("size after delete = %v, want 2", got)
	}
	var order []string
	s.Range(func(v BStr) { order = append(order, v.ToGoString()) })
	if len(order) != 2 || order[0] != "a" || order[1] != "c" {
		t.Fatalf("survivors after delete = %v, want [a c]", order)
	}
}

// TestSetNumberSameValueZero proves number members compare by SameValueZero: every
// NaN is the same member so a second NaN add is a no-op, and +0 and -0 are the same
// member so adding one after the other does not grow the set. A plain == would get
// the zeroes right but miss the NaN fold, which is the case worth pinning.
func TestSetNumberSameValueZero(t *testing.T) {
	nan := nanValue()
	s := NewNumberSet()
	s.Add(nan).Add(nan)
	if got := s.Size(); got != 1 {
		t.Fatalf("size after adding NaN twice = %v, want 1", got)
	}
	if !s.Has(nanValue()) {
		t.Fatal("Has(NaN) = false, want true")
	}
	z := NewNumberSet()
	z.Add(0).Add(negZero())
	if got := z.Size(); got != 1 {
		t.Fatalf("size after adding +0 and -0 = %v, want 1", got)
	}
}

// TestSetClearKeepsUsable proves Clear empties the set and that a set refilled after
// a clear behaves as a fresh one.
func TestSetClearKeepsUsable(t *testing.T) {
	s := NewBoolSet()
	s.Add(true).Add(false)
	s.Clear()
	if got := s.Size(); got != 0 {
		t.Fatalf("size after clear = %v, want 0", got)
	}
	s.Add(true)
	if got := s.Size(); got != 1 || !s.Has(true) {
		t.Fatalf("set unusable after clear: size %v", got)
	}
}
