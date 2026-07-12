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

// TestIterReduce folds the source with its callback, seeding from the initial value and
// counting the index from zero, the terminal reduce with a seed.
func TestIterReduce(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(1), Number(2), Number(3), Number(4)}), ArrayIterValues)
	fn := NewFunc(func(args []Value) Value { return Number(Arg(args, 0).AsNumber() + Arg(args, 1).AsNumber()) })
	got := IterReduce(src.Next, fn, true, Number(100)).AsNumber()
	if got != 110 {
		t.Errorf("reduce(sum, 100) = %v, want 110", got)
	}
}

// TestIterReduceNoInit seeds the accumulator from the first value and counts the index
// from one, the terminal reduce without a seed.
func TestIterReduceNoInit(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(2), Number(3), Number(4)}), ArrayIterValues)
	fn := NewFunc(func(args []Value) Value { return Number(Arg(args, 0).AsNumber() * Arg(args, 1).AsNumber()) })
	got := IterReduce(src.Next, fn, false, Undefined).AsNumber()
	if got != 24 {
		t.Errorf("reduce(product) = %v, want 24", got)
	}
}

// TestIterReduceEmptyNoInit throws a TypeError when the source is empty and no seed is
// given, the way the spec rejects a reduce with nothing to fold.
func TestIterReduceEmptyNoInit(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("reduce over an empty source with no seed did not throw")
		}
	}()
	src := NewArrayIter(NewArrayValue(nil), ArrayIterValues)
	fn := NewFunc(func(args []Value) Value { return Arg(args, 0) })
	IterReduce(src.Next, fn, false, Undefined)
}

// TestIterToArray collects the source into a new array, the terminal toArray.
func TestIterToArray(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(5), Number(6), Number(7)}), ArrayIterValues)
	arr := IterToArray(src.Next)
	if arr.kind != KindArray || arrayLikeLen(arr) != 3 {
		t.Fatalf("toArray = %+v, want an array of three", arr)
	}
	for i, want := range []float64{5, 6, 7} {
		if got := arrayLikeGet(arr, i).AsNumber(); got != want {
			t.Errorf("toArray[%d] = %v, want %v", i, got, want)
		}
	}
}

// TestIterToArrayEmpty collects an empty source into an empty array.
func TestIterToArrayEmpty(t *testing.T) {
	src := NewArrayIter(NewArrayValue(nil), ArrayIterValues)
	arr := IterToArray(src.Next)
	if arr.kind != KindArray || arrayLikeLen(arr) != 0 {
		t.Errorf("toArray over an empty source = %+v, want an empty array", arr)
	}
}

// TestIterForEach visits every value with the callback and passes the zero-based index,
// returning undefined.
func TestIterForEach(t *testing.T) {
	src := NewArrayIter(NewArrayValue([]Value{Number(10), Number(20), Number(30)}), ArrayIterValues)
	var seen []float64
	fn := NewFunc(func(args []Value) Value {
		seen = append(seen, Arg(args, 0).AsNumber()+Arg(args, 1).AsNumber())
		return Undefined
	})
	if got := IterForEach(src.Next, fn); !got.IsUndefined() {
		t.Errorf("forEach returned %+v, want undefined", got)
	}
	want := []float64{10, 21, 32}
	if len(seen) != len(want) || seen[0] != want[0] || seen[2] != want[2] {
		t.Errorf("forEach visited %v, want %v", seen, want)
	}
}

// TestIterSome returns true as soon as a value passes and false over a source with none
// passing, and an empty source is false.
func TestIterSome(t *testing.T) {
	vals := []Value{Number(1), Number(2), Number(3)}
	gt2 := NewFunc(func(args []Value) Value { return Bool(Arg(args, 0).AsNumber() > 2) })
	gt9 := NewFunc(func(args []Value) Value { return Bool(Arg(args, 0).AsNumber() > 9) })
	if !IterSome(NewArrayIter(NewArrayValue(vals), ArrayIterValues).Next, gt2) {
		t.Errorf("some(>2) = false, want true")
	}
	if IterSome(NewArrayIter(NewArrayValue(vals), ArrayIterValues).Next, gt9) {
		t.Errorf("some(>9) = true, want false")
	}
	if IterSome(NewArrayIter(NewArrayValue(nil), ArrayIterValues).Next, gt2) {
		t.Errorf("some over empty = true, want false")
	}
}

// TestIterEvery returns false as soon as a value fails and true over a source with all
// passing, and an empty source is true.
func TestIterEvery(t *testing.T) {
	vals := []Value{Number(1), Number(2), Number(3)}
	gt0 := NewFunc(func(args []Value) Value { return Bool(Arg(args, 0).AsNumber() > 0) })
	gt2 := NewFunc(func(args []Value) Value { return Bool(Arg(args, 0).AsNumber() > 2) })
	if !IterEvery(NewArrayIter(NewArrayValue(vals), ArrayIterValues).Next, gt0) {
		t.Errorf("every(>0) = false, want true")
	}
	if IterEvery(NewArrayIter(NewArrayValue(vals), ArrayIterValues).Next, gt2) {
		t.Errorf("every(>2) = true, want false")
	}
	if !IterEvery(NewArrayIter(NewArrayValue(nil), ArrayIterValues).Next, gt2) {
		t.Errorf("every over empty = false, want true")
	}
}

// TestIterFind returns the first passing value and undefined when none passes.
func TestIterFind(t *testing.T) {
	vals := []Value{Number(1), Number(2), Number(3), Number(4)}
	even := NewFunc(func(args []Value) Value { return Bool(int(Arg(args, 0).AsNumber())%2 == 0) })
	gt9 := NewFunc(func(args []Value) Value { return Bool(Arg(args, 0).AsNumber() > 9) })
	if got := IterFind(NewArrayIter(NewArrayValue(vals), ArrayIterValues).Next, even); got.AsNumber() != 2 {
		t.Errorf("find(even) = %v, want 2", got.AsNumber())
	}
	if got := IterFind(NewArrayIter(NewArrayValue(vals), ArrayIterValues).Next, gt9); !got.IsUndefined() {
		t.Errorf("find(>9) = %+v, want undefined", got)
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
