package value

import "testing"

// TestGetIndexArray pins that GetIndex reads an array element by a numeric index
// and reports its length through the "length" property, the dispatch a dynamic
// a[i] read takes on an array receiver.
func TestGetIndexArray(t *testing.T) {
	arr := NewArrayValue([]Value{Number(10), Number(20), Number(30)})
	if got := arr.GetIndex(1); got.Kind() != KindNumber || got.AsNumber() != 20 {
		t.Fatalf("arr[1] = %v, want 20", got)
	}
	if got := arr.GetIndex(0); got.AsNumber() != 10 {
		t.Fatalf("arr[0] = %v, want 10", got)
	}
	if got := arr.Get(FromGoString("length")); got.AsNumber() != 3 {
		t.Fatalf("arr.length = %v, want 3", got)
	}
}

// TestGetIndexOutOfRange pins that an index past the end reads undefined, the
// JavaScript result for a missing element rather than a fault.
func TestGetIndexOutOfRange(t *testing.T) {
	arr := NewArrayValue([]Value{Number(1)})
	if got := arr.GetIndex(5); !got.IsUndefined() {
		t.Fatalf("arr[5] = %v, want undefined", got)
	}
	if got := arr.GetIndex(1.5); !got.IsUndefined() {
		t.Fatalf("arr[1.5] = %v, want undefined", got)
	}
}

// TestGetIndexString pins that a numeric index into a string reads its
// one-code-unit string, and "length" reports the code-unit count.
func TestGetIndexString(t *testing.T) {
	s := StringValue(FromGoString("abc"))
	if got := s.GetIndex(2); got.Kind() != KindString || got.AsString().ToGoString() != "c" {
		t.Fatalf(`"abc"[2] = %v, want "c"`, got)
	}
	if got := s.Get(FromGoString("length")); got.AsNumber() != 3 {
		t.Fatalf(`"abc".length = %v, want 3`, got)
	}
}

// TestGetElemDynamicKey pins that GetElem coerces a dynamic key to a property key
// the way JavaScript does: a number key round-trips to its canonical string and
// reads the same element GetIndex would, and a string key is used as is.
func TestGetElemDynamicKey(t *testing.T) {
	arr := NewArrayValue([]Value{Number(7), Number(8), Number(9)})
	if got := arr.GetElem(Number(2)); got.AsNumber() != 9 {
		t.Fatalf("arr[Number(2)] = %v, want 9", got)
	}
	if got := arr.GetElem(StringValue(FromGoString("length"))); got.AsNumber() != 3 {
		t.Fatalf(`arr[String("length")] = %v, want 3`, got)
	}
}

// TestGetElemObjectProperty pins that a dynamic key reads a plain object's named
// property, the object side of the same runtime dispatch.
func TestGetElemObjectProperty(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("x"), Number(42))
	if got := o.GetElem(StringValue(FromGoString("x"))); got.AsNumber() != 42 {
		t.Fatalf(`o["x"] = %v, want 42`, got)
	}
	if got := o.GetElem(StringValue(FromGoString("missing"))); !got.IsUndefined() {
		t.Fatalf(`o["missing"] = %v, want undefined`, got)
	}
}
