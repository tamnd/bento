package value

import "testing"

// drain pulls a helper to exhaustion and returns the numbers it yielded, the shape
// most helper tests check against.
func drain(h *IterHelper) []float64 {
	var got []float64
	for {
		r := h.Next()
		if r.Done {
			return got
		}
		got = append(got, r.Value.AsNumber())
	}
}

// TestIterMap lifts each value through its callback and passes the zero-based index,
// and a value the source never reaches is never mapped.
func TestIterMap(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(10), Number(20), Number(30)}), ArrayIterValues)
	fn := NewFunc(func(args []Value) Value {
		return Number(Arg(args, 0).AsNumber() + Arg(args, 1).AsNumber()*100)
	})
	got := drain(IterMap(src.Next, fn))
	want := []float64{10, 120, 230}
	if len(got) != len(want) {
		t.Fatalf("IterMap yielded %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("IterMap[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestIterFilter keeps the values whose callback is truthy and drops the rest without
// materializing them, and the index counts every value the predicate sees.
func TestIterFilter(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(1), Number(2), Number(3), Number(4), Number(5)}), ArrayIterValues)
	fn := NewFunc(func(args []Value) Value {
		if int(Arg(args, 0).AsNumber())%2 == 0 {
			return Bool(true)
		}
		return Bool(false)
	})
	got := drain(IterFilter(src.Next, fn))
	want := []float64{2, 4}
	if len(got) != len(want) {
		t.Fatalf("IterFilter yielded %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("IterFilter[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestIterMapFilterChain wraps a filter over a map, the way a helper chain nests each
// iterator's Next inside the one above it, and pulls the composed result.
func TestIterMapFilterChain(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(1), Number(2), Number(3), Number(4), Number(5), Number(6)}), ArrayIterValues)
	add1 := NewFunc(func(args []Value) Value { return Number(Arg(args, 0).AsNumber() + 1) })
	div3 := NewFunc(func(args []Value) Value { return Bool(int(Arg(args, 0).AsNumber())%3 == 0) })
	got := drain(IterFilter(IterMap(src.Next, add1).Next, div3))
	want := []float64{3, 6}
	if len(got) != len(want) {
		t.Fatalf("chain yielded %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chain[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestIterHelperNilNext reports done forever when the closure is nil, so a helper over
// an empty or exhausted source is safe to pull past its end.
func TestIterHelperNilNext(t *testing.T) {
	h := NewIterHelper(nil)
	for i := 0; i < 3; i++ {
		r := h.Next()
		if !r.Done || !r.Value.IsUndefined() {
			t.Fatalf("nil-next Next = %+v, want done undefined", r)
		}
	}
}
