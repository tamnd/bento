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
