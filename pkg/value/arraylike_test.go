package value

import (
	"math"
	"testing"
)

// arrayLike builds a plain object with the given length property and integer-keyed
// elements, the shape test262 borrows an Array.prototype method onto.
func arrayLike(length float64, elems ...Value) Value {
	o := NewObject()
	for i, e := range elems {
		o.SetIndex(float64(i), e)
	}
	o.Set(FromGoString("length"), Number(length))
	return o
}

// TestToLength proves the length coercion matches ToLength: a fractional length
// truncates toward zero, a negative or NaN length clamps to 0, a numeric-string
// length coerces through ToNumber, and an over-large length caps at 2^53 - 1.
func TestToLength(t *testing.T) {
	cases := []struct {
		in   Value
		want int
	}{
		{Number(2.9), 2},
		{Number(-5), 0},
		{Number(math.NaN()), 0},
		{StringValue(FromGoString("3")), 3},
		{Number(1e21), maxArrayLength},
		{Undefined, 0},
	}
	for _, c := range cases {
		if got := toLength(c.in); got != c.want {
			t.Fatalf("toLength(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestGenericLenClamp proves a generic-receiver method walks only the indices below
// the coerced length, so a fractional length hides the tail element and a negative
// length yields an empty walk.
func TestGenericLenClamp(t *testing.T) {
	o := arrayLike(2.9, StringValue(FromGoString("a")), StringValue(FromGoString("b")), StringValue(FromGoString("c")))
	if got := GenericIndexOf(o, StringValue(FromGoString("c"))); got.AsNumber() != -1 {
		t.Fatalf("indexOf c past a 2.9 length = %v, want -1", got.AsNumber())
	}
	neg := arrayLike(-5, StringValue(FromGoString("x")))
	if got := GenericIncludes(neg, StringValue(FromGoString("x"))); got.AsBool() {
		t.Fatal("includes x under a negative length = true, want false")
	}
}

// TestGenericIndexOf proves the search reads length and integer keys off a generic
// receiver, finds the first strict match, honors a negative fromIndex, and reports
// -1 when the element is absent.
func TestGenericIndexOf(t *testing.T) {
	o := arrayLike(3, StringValue(FromGoString("a")), StringValue(FromGoString("b")), StringValue(FromGoString("a")))
	if got := GenericIndexOf(o, StringValue(FromGoString("a"))); got.AsNumber() != 0 {
		t.Fatalf("indexOf a = %v, want 0", got.AsNumber())
	}
	if got := GenericIndexOf(o, StringValue(FromGoString("a")), Number(1)); got.AsNumber() != 2 {
		t.Fatalf("indexOf a from 1 = %v, want 2", got.AsNumber())
	}
	if got := GenericIndexOf(o, StringValue(FromGoString("a")), Number(-1)); got.AsNumber() != 2 {
		t.Fatalf("indexOf a from -1 = %v, want 2", got.AsNumber())
	}
	if got := GenericIndexOf(o, StringValue(FromGoString("z"))); got.AsNumber() != -1 {
		t.Fatalf("indexOf z = %v, want -1", got.AsNumber())
	}
}

// TestGenericLastIndexOf proves the reverse search finds the last strict match and
// honors a fromIndex bound.
func TestGenericLastIndexOf(t *testing.T) {
	o := arrayLike(3, Number(1), Number(2), Number(1))
	if got := GenericLastIndexOf(o, Number(1)); got.AsNumber() != 2 {
		t.Fatalf("lastIndexOf 1 = %v, want 2", got.AsNumber())
	}
	if got := GenericLastIndexOf(o, Number(1), Number(1)); got.AsNumber() != 0 {
		t.Fatalf("lastIndexOf 1 from 1 = %v, want 0", got.AsNumber())
	}
}

// double is a boxed callback that returns twice its first argument, standing in for
// a lowered arrow the generic-receiver methods invoke.
func double() Value {
	return NewFunc(func(args []Value) Value {
		return Number(Arg(args, 0).AsNumber() * 2)
	})
}

// isEven is a boxed predicate over the first argument.
func isEven() Value {
	return NewFunc(func(args []Value) Value {
		return Bool(int(Arg(args, 0).AsNumber())%2 == 0)
	})
}

// TestGenericCallbackMethods proves the callback methods invoke the callback on each
// element and shape their results the array way: map builds a new array, filter keeps
// the truthy elements, some and every fold to a boolean, and find and findIndex
// report the first match.
func TestGenericCallbackMethods(t *testing.T) {
	o := arrayLike(3, Number(1), Number(2), Number(3))

	m := GenericMap(o, double())
	for i, w := range []float64{2, 4, 6} {
		if v := m.GetIndex(float64(i)); v.AsNumber() != w {
			t.Fatalf("map[%d] = %v, want %v", i, v.AsNumber(), w)
		}
	}
	f := GenericFilter(o, isEven())
	if f.Get(FromGoString("length")).AsNumber() != 1 {
		t.Fatalf("filter length = %v, want 1", f.Get(FromGoString("length")).AsNumber())
	}
	if !GenericSome(o, isEven()).AsBool() {
		t.Fatal("some even = false, want true")
	}
	if GenericEvery(o, isEven()).AsBool() {
		t.Fatal("every even = true, want false")
	}
	if v := GenericFind(o, isEven()); v.AsNumber() != 2 {
		t.Fatalf("find even = %v, want 2", v.AsNumber())
	}
	if v := GenericFindIndex(o, isEven()); v.AsNumber() != 1 {
		t.Fatalf("findIndex even = %v, want 1", v.AsNumber())
	}
}

// TestGenericForEach proves forEach visits each element and returns undefined.
func TestGenericForEach(t *testing.T) {
	o := arrayLike(3, Number(1), Number(2), Number(3))
	sum := 0.0
	cb := NewFunc(func(args []Value) Value {
		sum += Arg(args, 0).AsNumber()
		return Undefined
	})
	if got := GenericForEach(o, cb); got.kind != KindUndefined {
		t.Fatalf("forEach returned %v, want undefined", got)
	}
	if sum != 6 {
		t.Fatalf("forEach sum = %v, want 6", sum)
	}
}

// TestGenericCallbackReceivesIndexAndObject proves the callback is invoked with the
// element, its index, and the receiver object, so a callback that reads the second or
// third parameter sees them.
func TestGenericCallbackReceivesIndexAndObject(t *testing.T) {
	o := arrayLike(2, StringValue(FromGoString("a")), StringValue(FromGoString("b")))
	var gotIndices []float64
	var gotObject Value
	cb := NewFunc(func(args []Value) Value {
		gotIndices = append(gotIndices, Arg(args, 1).AsNumber())
		gotObject = Arg(args, 2)
		return Arg(args, 0)
	})
	GenericMap(o, cb)
	if len(gotIndices) != 2 || gotIndices[0] != 0 || gotIndices[1] != 1 {
		t.Fatalf("callback indices = %v, want [0 1]", gotIndices)
	}
	if !StrictEquals(gotObject, o) {
		t.Fatal("callback did not receive the receiver as the third argument")
	}
}

// TestGenericFill proves fill writes value into each index in range as the property
// named by the number, honors relative bounds, and returns the receiver.
func TestGenericFill(t *testing.T) {
	o := arrayLike(4, Number(0), Number(0), Number(0), Number(0))
	got := GenericFill(o, Number(7), Number(1), Number(3))
	if !StrictEquals(got, o) {
		t.Fatal("fill did not return the receiver")
	}
	want := []float64{0, 7, 7, 0}
	for i, w := range want {
		if v := o.GetIndex(float64(i)); v.AsNumber() != w {
			t.Fatalf("o[%d] = %v, want %v", i, v.AsNumber(), w)
		}
	}
	// The written element is a named property on the generic object, reachable by its
	// key string, not a slice slot.
	if v := o.Get(FromGoString("1")); v.AsNumber() != 7 {
		t.Fatalf(`o["1"] = %v, want 7`, v.AsNumber())
	}
}

// TestGenericReverse proves reverse swaps elements in place across the middle,
// reading and writing each as the property named by its index, and returns the
// receiver.
func TestGenericReverse(t *testing.T) {
	o := arrayLike(3, Number(1), Number(2), Number(3))
	GenericReverse(o)
	want := []float64{3, 2, 1}
	for i, w := range want {
		if v := o.GetIndex(float64(i)); v.AsNumber() != w {
			t.Fatalf("o[%d] = %v, want %v", i, v.AsNumber(), w)
		}
	}
}

// TestGenericSlice proves slice reads the range [start, end) off a generic receiver,
// honors relative bounds, and returns a new array of those elements.
func TestGenericSlice(t *testing.T) {
	o := arrayLike(4, Number(1), Number(2), Number(3), Number(4))
	s := GenericSlice(o, Number(1), Number(3))
	if s.Get(FromGoString("length")).AsNumber() != 2 {
		t.Fatalf("slice length = %v, want 2", s.Get(FromGoString("length")).AsNumber())
	}
	for i, w := range []float64{2, 3} {
		if v := s.GetIndex(float64(i)); v.AsNumber() != w {
			t.Fatalf("slice[%d] = %v, want %v", i, v.AsNumber(), w)
		}
	}
	// A negative start counts from the end, and an omitted end runs to the length.
	tail := GenericSlice(o, Number(-2))
	if tail.Get(FromGoString("length")).AsNumber() != 2 || tail.GetIndex(0).AsNumber() != 3 {
		t.Fatalf("slice(-2) = %v, want [3 4]", ToString(tail))
	}
}

// TestGenericResultIsRealArray proves map, filter, and slice on a generic receiver
// return a real array, not a plain object: the result stringifies the array way, as
// its elements joined by commas rather than the [object Object] tag an object gives.
func TestGenericResultIsRealArray(t *testing.T) {
	o := arrayLike(3, Number(1), Number(2), Number(3))
	cases := []struct {
		name string
		got  Value
		want string
	}{
		{"map", GenericMap(o, double()), "2,4,6"},
		{"filter", GenericFilter(o, isEven()), "2"},
		{"slice", GenericSlice(o, Number(1)), "2,3"},
	}
	for _, c := range cases {
		if c.got.kind != KindArray {
			t.Fatalf("%s result kind = %v, want array", c.name, c.got.kind)
		}
		if s := ToString(c.got).ToGoString(); s != c.want {
			t.Fatalf("%s result string = %q, want %q", c.name, s, c.want)
		}
	}
}

// TestGenericIncludes proves the membership test uses SameValueZero, so a stored NaN
// is found where a strict search would miss it.
func TestGenericIncludes(t *testing.T) {
	o := arrayLike(2, Number(1), Number(math.NaN()))
	if got := GenericIncludes(o, Number(1)); !got.AsBool() {
		t.Fatal("includes 1 = false, want true")
	}
	if got := GenericIncludes(o, Number(math.NaN())); !got.AsBool() {
		t.Fatal("includes NaN = false, want true")
	}
	if got := GenericIncludes(o, Number(9)); got.AsBool() {
		t.Fatal("includes 9 = true, want false")
	}
}
