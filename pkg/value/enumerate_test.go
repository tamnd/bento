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
