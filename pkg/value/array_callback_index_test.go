package value

import "testing"

// TestSomeEveryIndex pins the (element, index) predicates: each callback sees
// the float64 position, and short-circuiting is unchanged.
func TestSomeEveryIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	if !a.SomeIndex(func(x, i float64) bool { return i == 2 && x == 3 }) {
		t.Fatalf("SomeIndex did not see the last element at index 2")
	}
	if a.SomeIndex(func(_, i float64) bool { return i > 5 }) {
		t.Fatalf("SomeIndex accepted an index no element has")
	}
	if !a.EveryIndex(func(x, i float64) bool { return x > i }) {
		t.Fatalf("EveryIndex should hold: every element exceeds its index")
	}
	if a.EveryIndex(func(_, i float64) bool { return i < 2 }) {
		t.Fatalf("EveryIndex should fail at the index-2 element")
	}
}

// TestForEachIndex pins that the effect callback sees each position in order.
func TestForEachIndex(t *testing.T) {
	a := NewArray[float64](10, 20, 30)
	var seen []float64
	a.ForEachIndex(func(x, i float64) { seen = append(seen, i*100+x) })
	want := []float64{10, 120, 230}
	for k, w := range want {
		if seen[k] != w {
			t.Fatalf("ForEachIndex[%d] = %v, want %v", k, seen[k], w)
		}
	}
}

// TestFindIndexedVariants pins the find family index callbacks: the element and
// position finders, forward and from the end.
func TestFindIndexedVariants(t *testing.T) {
	a := NewArray[float64](5, 6, 7)
	if got := a.FindIndexed(func(_, i float64) bool { return i == 1 }); got.IsUndefined() || got.Get() != 6 {
		t.Fatalf("FindIndexed = %v, want present 6", got)
	}
	if got := a.FindIndexIndex(func(x, i float64) bool { return x == 7 && i == 2 }); got != 2 {
		t.Fatalf("FindIndexIndex = %v, want 2", got)
	}
	if got := a.FindLastIndexed(func(_, i float64) bool { return i < 2 }); got.IsUndefined() || got.Get() != 6 {
		t.Fatalf("FindLastIndexed = %v, want present 6", got)
	}
	if got := a.FindLastIndexIndex(func(x, _ float64) bool { return x > 5 }); got != 2 {
		t.Fatalf("FindLastIndexIndex = %v, want 2", got)
	}
	if got := a.FindIndexed(func(_, i float64) bool { return i > 9 }); !got.IsUndefined() {
		t.Fatalf("FindIndexed found a match at an impossible index")
	}
	if got := a.FindIndexIndex(func(_, i float64) bool { return i > 9 }); got != -1 {
		t.Fatalf("FindIndexIndex = %v, want -1 sentinel", got)
	}
}
