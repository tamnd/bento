package value

import (
	"math"
	"testing"
)

// TestGenericMapSliceThrowOnOverLongLength proves the ArrayCreate bound: map and slice
// build their result through ArraySpeciesCreate(O, len), which throws a RangeError when
// the length exceeds 2^32 - 1 before any element is created. A borrowed call on an
// array-like whose length is 2^32 therefore throws rather than driving an unbounded
// allocation, and the callback never runs.
func TestGenericMapSliceThrowOnOverLongLength(t *testing.T) {
	overLong := Number(math.Pow(2, 32)) // 2^32, one past ArrayCreate's max

	for _, tc := range []struct {
		name string
		run  func(recv Value)
	}{
		{"map", func(r Value) {
			called := false
			cb := NewFunc(func(args []Value) Value { called = true; return Undefined })
			defer func() {
				if called {
					t.Errorf("map callback ran before the RangeError")
				}
			}()
			GenericMap(r, cb)
		}},
		{"slice", func(r Value) { GenericSlice(r) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recv := NewObject()
			recv.Set(FromGoString("length"), overLong)
			defer func() {
				rec := recover()
				if rec == nil {
					t.Fatalf("%s on a length of 2^32 did not throw", tc.name)
				}
				if e := Caught(rec); !e.IsA("RangeError") {
					t.Fatalf("%s threw %v, want a RangeError", tc.name, e)
				}
			}()
			tc.run(recv)
		})
	}
}

// TestGenericMapSliceWithinBoundStillRun proves the guard does not disturb a length
// within ArrayCreate's bound: map still maps and slice still copies for an ordinary
// array-like.
func TestGenericMapSliceWithinBoundStillRun(t *testing.T) {
	src := arrayLike(3, Number(1), Number(2), Number(3))

	double := NewFunc(func(args []Value) Value { return Number(Arg(args, 0).AsNumber() * 2) })
	mapped := GenericMap(src, double)
	if mapped.GetIndex(0).AsNumber() != 2 || mapped.GetIndex(2).AsNumber() != 6 {
		t.Errorf("map within bound = %v, want [2 4 6]", mapped)
	}

	sliced := GenericSlice(src, Number(1))
	if int(sliced.Get(FromGoString("length")).AsNumber()) != 2 || sliced.GetIndex(0).AsNumber() != 2 {
		t.Errorf("slice within bound = %v, want [2 3]", sliced)
	}
}
