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
