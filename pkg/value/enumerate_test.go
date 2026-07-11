package value

import "testing"

// TestAssign proves the target gains each source's own enumerable string
// properties in enumeration order, that a later source overwrites an earlier key,
// and that the target itself is returned.
func TestAssign(t *testing.T) {
	target := NewObject()
	target.Set(FromGoString("a"), Number(1))

	src1 := NewObject()
	src1.Set(FromGoString("b"), Number(2))
	src2 := NewObject()
	src2.Set(FromGoString("a"), Number(9))
	src2.Set(FromGoString("c"), Number(3))

	got := target.Assign(src1, src2)
	if got.ref != target.ref {
		t.Fatal("Assign did not return the target")
	}
	for k, want := range map[string]float64{"a": 9, "b": 2, "c": 3} {
		if v := target.Get(FromGoString(k)); v.scalar != Number(want).scalar {
			t.Fatalf("target[%q] = %v, want %v", k, v, want)
		}
	}
}

// TestAssignSkipsNullish proves a null or undefined source contributes nothing and
// does not fault, the no-op the spec performs for a nullish source.
func TestAssignSkipsNullish(t *testing.T) {
	target := NewObject()
	src := NewObject()
	src.Set(FromGoString("x"), Number(5))

	target.Assign(Null, src, Undefined)
	if v := target.Get(FromGoString("x")); v.scalar != Number(5).scalar {
		t.Fatalf("target[x] = %v, want 5", v)
	}
}

// TestAssignSkipsNonEnumerable proves a source's non-enumerable property is left
// behind, matching Object.assign copying only own enumerable properties.
func TestAssignSkipsNonEnumerable(t *testing.T) {
	target := NewObject()
	src := NewObject()
	src.object().keys = append(src.object().keys, FromGoString("hidden"))
	src.object().descs = append(src.object().descs, descriptor{value: Number(7)})

	target.Assign(src)
	if v := target.Get(FromGoString("hidden")); v.kind != KindUndefined {
		t.Fatalf("target copied a non-enumerable property: got %v, want undefined", v)
	}
}

// TestAssignStringSource proves a string source contributes its index characters,
// the own enumerable properties its wrapper exposes.
func TestAssignStringSource(t *testing.T) {
	target := NewObject()
	target.Assign(StringValue(FromGoString("hi")))
	if v := target.Get(FromGoString("0")); v.kind != KindString || v.str().ToGoString() != "h" {
		t.Fatalf("target[0] = %v, want \"h\"", v)
	}
	if v := target.Get(FromGoString("1")); v.kind != KindString || v.str().ToGoString() != "i" {
		t.Fatalf("target[1] = %v, want \"i\"", v)
	}
}

// TestOwnSymbols proves the receiver's own symbol keys come back in insertion order
// by identity, including a non-enumerable one, and that a receiver with no symbol
// keys yields an empty array.
func TestOwnSymbols(t *testing.T) {
	s1 := NewSymbol(FromGoString("one"))
	s2 := NewSymbol(FromGoString("two"))
	o := NewObject()
	o.SetElem(s1, Number(1))
	o.SetElem(s2, Number(2))

	got := o.OwnSymbols()
	if got.Len() != 2 {
		t.Fatalf("OwnSymbols len = %v, want 2", got.Len())
	}
	if !StrictEquals(got.At(0), s1) || !StrictEquals(got.At(1), s2) {
		t.Fatal("OwnSymbols did not return the symbol keys by identity in insertion order")
	}
	if NewObject().OwnSymbols().Len() != 0 {
		t.Fatal("an object with no symbol keys reported symbols")
	}
}

// TestEntries proves the receiver's own enumerable properties come back as [key,
// value] pairs in enumeration order, integer indices before named keys.
func TestEntries(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("b"), Number(2))
	o.Set(FromGoString("a"), Number(1))

	got := o.Entries()
	first := got.GetIndex(0)
	if k := first.GetIndex(0); k.kind != KindString || k.str().ToGoString() != "b" {
		t.Fatalf("first pair key = %v, want \"b\"", k)
	}
	if v := first.GetIndex(1); v.scalar != Number(2).scalar {
		t.Fatalf("first pair value = %v, want 2", v)
	}
	second := got.GetIndex(1)
	if k := second.GetIndex(0); k.kind != KindString || k.str().ToGoString() != "a" {
		t.Fatalf("second pair key = %v, want \"a\"", k)
	}
}

// TestFromEntries proves each key-value pair becomes an own property of the fresh
// object and that a later pair overwrites an earlier one with the same key.
func TestFromEntries(t *testing.T) {
	pairs := NewArrayValue([]Value{
		NewArrayValue([]Value{StringValue(FromGoString("a")), Number(1)}),
		NewArrayValue([]Value{StringValue(FromGoString("b")), Number(2)}),
		NewArrayValue([]Value{StringValue(FromGoString("a")), Number(9)}),
	})
	o := FromEntries(pairs)
	if v := o.Get(FromGoString("a")); v.scalar != Number(9).scalar {
		t.Fatalf("o[a] = %v, want 9", v)
	}
	if v := o.Get(FromGoString("b")); v.scalar != Number(2).scalar {
		t.Fatalf("o[b] = %v, want 2", v)
	}
}
