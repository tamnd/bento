package value

import "testing"

// TestFindLast pins that findLast returns the last matching element as a present
// optional and the undefined optional when nothing matches, walking from the end so
// it returns the later of two matches.
func TestFindLast(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	even := func(n float64) bool { return int(n)%2 == 0 }
	got := a.FindLast(even)
	if got.IsUndefined() || got.Get() != 4 {
		t.Errorf("FindLast even = %v, want 4", got)
	}
	none := a.FindLast(func(n float64) bool { return n > 10 })
	if !none.IsUndefined() {
		t.Errorf("FindLast no match = %v, want undefined", none)
	}
	empty := NewArray[float64]()
	if !empty.FindLast(even).IsUndefined() {
		t.Error("FindLast on empty array is not undefined")
	}
}

// TestFindLastIndex pins that findLastIndex returns the index of the last match and
// -1 when nothing matches.
func TestFindLastIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	even := func(n float64) bool { return int(n)%2 == 0 }
	if got := a.FindLastIndex(even); got != 3 {
		t.Errorf("FindLastIndex even = %v, want 3", got)
	}
	if got := a.FindLastIndex(func(n float64) bool { return n > 10 }); got != -1 {
		t.Errorf("FindLastIndex no match = %v, want -1", got)
	}
}
