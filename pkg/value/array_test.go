package value

import "testing"

// TestNewArrayLenElems pins the dense array header: length is the element count
// as a float64, and iteration reads the elements back in order and unchanged.
func TestNewArrayLenElems(t *testing.T) {
	a := NewArray[float64](10, 20, 30)
	if got := a.Len(); got != 3 {
		t.Fatalf("Len() = %v, want 3", got)
	}
	want := []float64{10, 20, 30}
	got := a.Elems()
	if len(got) != len(want) {
		t.Fatalf("Elems() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Elems()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestNewArrayEmpty pins that an empty literal is a length-zero array, not a nil
// header, so .length reads 0 and iteration visits nothing.
func TestNewArrayEmpty(t *testing.T) {
	a := NewArray[float64]()
	if got := a.Len(); got != 0 {
		t.Fatalf("Len() = %v, want 0", got)
	}
	if got := len(a.Elems()); got != 0 {
		t.Fatalf("Elems() len = %d, want 0", got)
	}
}

// TestNewArrayOwnsStorage pins that NewArray copies its elements into its own
// backing store, so mutating the caller's slice after construction does not
// reach into the array.
func TestNewArrayOwnsStorage(t *testing.T) {
	src := []float64{1, 2, 3}
	a := NewArray(src...)
	src[0] = 99
	if got := a.Elems()[0]; got != 1 {
		t.Fatalf("array aliased its argument: Elems()[0] = %v, want 1", got)
	}
}

// TestPush pins that push appends, returns the new length, and grows the array
// the iteration reads back, including the variadic multi-argument form.
func TestPush(t *testing.T) {
	a := NewArray[float64](1)
	if got := a.Push(2); got != 2 {
		t.Fatalf("Push(2) = %v, want 2", got)
	}
	if got := a.Push(3, 4); got != 4 {
		t.Fatalf("Push(3, 4) = %v, want 4", got)
	}
	want := []float64{1, 2, 3, 4}
	got := a.Elems()
	if len(got) != len(want) {
		t.Fatalf("after pushes len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Elems()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestPushShared pins that push mutates through the pointer, so a second
// reference to the same array sees the appended element. This is the shared
// mutation a const binding does not prevent.
func TestPushShared(t *testing.T) {
	a := NewArray[float64](1)
	b := a
	a.Push(2)
	if got := b.Len(); got != 2 {
		t.Fatalf("shared reference Len() = %v, want 2", got)
	}
}

// TestMap pins that map applies the callback to each element in order and
// returns a fresh array, leaving the receiver unchanged.
func TestMap(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	b := a.Map(func(x float64) float64 { return x * 10 })
	want := []float64{10, 20, 30}
	got := b.Elems()
	if len(got) != len(want) {
		t.Fatalf("Map len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Map()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if a.Elems()[0] != 1 {
		t.Errorf("Map mutated the receiver: Elems()[0] = %v, want 1", a.Elems()[0])
	}
}

// TestFilter pins that filter keeps the elements the predicate accepts, in
// order, and returns a fresh array, leaving the receiver unchanged.
func TestFilter(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	b := a.Filter(func(x float64) bool { return x > 2 })
	want := []float64{3, 4}
	got := b.Elems()
	if len(got) != len(want) {
		t.Fatalf("Filter len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Filter()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if a.Len() != 4 {
		t.Errorf("Filter mutated the receiver: Len() = %v, want 4", a.Len())
	}
}

// TestFilterNoneKept pins that a predicate that rejects everything yields a
// length-zero array, not a nil header.
func TestFilterNoneKept(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	b := a.Filter(func(x float64) bool { return x > 100 })
	if got := b.Len(); got != 0 {
		t.Fatalf("Filter len = %v, want 0", got)
	}
}

// TestSlice pins the slice bounds: a half-open two-bound range, a one-bound run
// to the end, a negative start counting from the end, and the no-argument whole
// copy, each returning a fresh array and leaving the receiver unchanged.
func TestArraySlice(t *testing.T) {
	a := NewArray[float64](0, 1, 2, 3, 4)
	cases := []struct {
		name   string
		bounds []float64
		want   []float64
	}{
		{"two bounds", []float64{1, 3}, []float64{1, 2}},
		{"one bound", []float64{2}, []float64{2, 3, 4}},
		{"negative start", []float64{-2}, []float64{3, 4}},
		{"no argument", nil, []float64{0, 1, 2, 3, 4}},
		{"crossed pair", []float64{3, 1}, []float64{}},
		{"start past end", []float64{99}, []float64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := a.Slice(tc.bounds...).Elems()
			if len(got) != len(tc.want) {
				t.Fatalf("Slice(%v) len = %d, want %d", tc.bounds, len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("Slice(%v)[%d] = %v, want %v", tc.bounds, i, got[i], tc.want[i])
				}
			}
		})
	}
	if a.Len() != 5 {
		t.Errorf("Slice mutated the receiver: Len() = %v, want 5", a.Len())
	}
}

// numberStrictEq is the strict-equality closure the lowerer synthesizes for
// indexOf on a number array: Go == on two float64.
func numberStrictEq(a, b float64) bool { return a == b }

// numberSameValueZero is the closure for includes on a number array: == plus the
// NaN-matches-NaN case SameValueZero adds.
func numberSameValueZero(a, b float64) bool { return a == b || a != a && b != b }

// TestIndexOf pins that indexOf returns the first matching index or -1, and that
// with strict equality a NaN target is never found, matching indexOf.
func TestIndexOf(t *testing.T) {
	a := NewArray[float64](10, 20, 30, 20)
	if got := a.IndexOf(20, numberStrictEq); got != 1 {
		t.Errorf("IndexOf(20) = %v, want 1", got)
	}
	if got := a.IndexOf(99, numberStrictEq); got != -1 {
		t.Errorf("IndexOf(99) = %v, want -1", got)
	}
	nan := 0.0
	nan = nan / nan
	withNaN := NewArray(1, nan, 3)
	if got := withNaN.IndexOf(nan, numberStrictEq); got != -1 {
		t.Errorf("IndexOf(NaN) with strict equality = %v, want -1", got)
	}
}

// TestIncludes pins that includes reports membership and, with SameValueZero,
// does find a NaN, the point where it diverges from indexOf.
func TestIncludes(t *testing.T) {
	a := NewArray[float64](10, 20, 30)
	if !a.Includes(20, numberSameValueZero) {
		t.Error("Includes(20) = false, want true")
	}
	if a.Includes(99, numberSameValueZero) {
		t.Error("Includes(99) = true, want false")
	}
	nan := 0.0
	nan = nan / nan
	withNaN := NewArray(1, nan, 3)
	if !withNaN.Includes(nan, numberSameValueZero) {
		t.Error("Includes(NaN) with SameValueZero = false, want true")
	}
}

// TestJoin pins that join stringifies each element and interleaves the
// separator, that an empty array joins to the empty string, and that a single
// element carries no separator.
func TestJoin(t *testing.T) {
	num := func(x float64) BStr { return NumberToString(x) }
	a := NewArray[float64](1, 2, 3)
	if got := a.Join(FromGoString("-"), num).ToGoString(); got != "1-2-3" {
		t.Errorf("Join(-) = %q, want \"1-2-3\"", got)
	}
	one := NewArray[float64](7)
	if got := one.Join(FromGoString("-"), num).ToGoString(); got != "7" {
		t.Errorf("Join on one element = %q, want \"7\"", got)
	}
	empty := NewArray[float64]()
	if got := empty.Join(FromGoString("-"), num).ToGoString(); got != "" {
		t.Errorf("Join on empty = %q, want \"\"", got)
	}
	id := func(x BStr) BStr { return x }
	words := NewArray(FromGoString("a"), FromGoString("bb"))
	if got := words.Join(FromGoString(", "), id).ToGoString(); got != "a, bb" {
		t.Errorf("Join on strings = %q, want \"a, bb\"", got)
	}
}

// TestNewArrayString pins the header at a non-numeric element type, the string[]
// case the lowerer emits as *Array[BStr].
func TestNewArrayString(t *testing.T) {
	a := NewArray(FromGoString("a"), FromGoString("bb"))
	if got := a.Len(); got != 2 {
		t.Fatalf("Len() = %v, want 2", got)
	}
	if got := a.Elems()[1].Length(); got != 2 {
		t.Fatalf("second element length = %v, want 2", got)
	}
}
