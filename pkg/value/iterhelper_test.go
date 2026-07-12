package value

import (
	"math"
	"testing"
)

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

// TestIterTake yields at most the count and then reports done, and a count larger than
// the source yields the whole source.
func TestIterTake(t *testing.T) {
	newSrc := func() func() IterResult {
		return NewArrayIter(NewArrayValue([]Value{Number(1), Number(2), Number(3), Number(4)}), ArrayIterValues).Next
	}
	if got := drain(IterTake(newSrc(), Number(2))); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("take(2) = %v, want [1 2]", got)
	}
	if got := drain(IterTake(newSrc(), Number(10))); len(got) != 4 {
		t.Errorf("take(10) over four values = %v, want all four", got)
	}
	if got := drain(IterTake(newSrc(), Number(0))); len(got) != 0 {
		t.Errorf("take(0) = %v, want nothing", got)
	}
}

// TestIterDrop skips the count and yields the rest, and dropping more than the source
// holds yields nothing.
func TestIterDrop(t *testing.T) {
	newSrc := func() func() IterResult {
		return NewArrayIter(NewArrayValue([]Value{Number(1), Number(2), Number(3), Number(4)}), ArrayIterValues).Next
	}
	if got := drain(IterDrop(newSrc(), Number(2))); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Errorf("drop(2) = %v, want [3 4]", got)
	}
	if got := drain(IterDrop(newSrc(), Number(10))); len(got) != 0 {
		t.Errorf("drop(10) over four values = %v, want nothing", got)
	}
}

// TestIterLimitRangeError rejects a NaN or negative count with a RangeError before the
// helper yields anything, the validation take and drop share.
func TestIterLimitRangeError(t *testing.T) {
	cases := []struct {
		name  string
		limit Value
	}{
		{"nan", Number(math.NaN())},
		{"negative", Number(-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("iterLimit(%s) did not throw", tc.name)
				}
			}()
			iterLimit(tc.limit)
		})
	}
}

// TestIterFlatMap maps each value to an array and flattens the results in order,
// driving each mapped array to exhaustion before the next outer value.
func TestIterFlatMap(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(1), Number(2), Number(3)}), ArrayIterValues)
	fn := NewFunc(func(args []Value) Value {
		n := Arg(args, 0).AsNumber()
		return NewArrayValue([]Value{Number(n), Number(n * 10)})
	})
	got := drain(IterFlatMap(src.Next, fn))
	want := []float64{1, 10, 2, 20, 3, 30}
	if len(got) != len(want) {
		t.Fatalf("flatMap yielded %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("flatMap[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestIterFlatMapEmpty drops a mapped empty array, yielding nothing for it, so the
// flattened result skips over it.
func TestIterFlatMapEmpty(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(1), Number(2), Number(3)}), ArrayIterValues)
	fn := NewFunc(func(args []Value) Value {
		n := Arg(args, 0).AsNumber()
		if int(n) == 2 {
			return NewArrayValue(nil)
		}
		return NewArrayValue([]Value{Number(n)})
	})
	got := drain(IterFlatMap(src.Next, fn))
	want := []float64{1, 3}
	if len(got) != len(want) || got[0] != 1 || got[1] != 3 {
		t.Errorf("flatMap dropping an empty array = %v, want %v", got, want)
	}
}

// TestIterFlatMapRejectsPrimitive throws a TypeError when the mapper returns a value
// that is not an array, the reject-primitives handling flatMap's flatten step takes.
func TestIterFlatMapRejectsPrimitive(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("flatMap over a number-returning mapper did not throw")
		}
	}()
	src := NewArrayIter(NewArrayValue([]Value{Number(1)}), ArrayIterValues)
	fn := NewFunc(func(args []Value) Value { return Arg(args, 0) })
	IterFlatMap(src.Next, fn).Next()
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
