package value

import "testing"

// hasIndex reports whether the array carries an own property at index i, the in
// operator's answer, so a test can assert a hole is absent where a stored value is
// present.
func hasIndex(v Value, i int) bool {
	return v.HasProperty(NumberToString(float64(i)))
}

// TestDeleteMakesHole proves delete a[i] leaves a hole, not a stored undefined: the
// index is no longer an own property, the read still yields undefined, and the
// length does not change. A neighboring stored value stays present.
func TestDeleteMakesHole(t *testing.T) {
	a := NewArrayValue([]Value{Number(1), Number(2), Number(3)})
	if !a.DeleteIndex(1) {
		t.Fatal("delete a[1] returned false")
	}
	if hasIndex(a, 1) {
		t.Fatal("1 in a after delete = true, want false (a hole is absent)")
	}
	if a.GetIndex(1).kind != KindUndefined {
		t.Fatalf("a[1] after delete = %v, want undefined", a.GetIndex(1))
	}
	if l := a.Get(FromGoString("length")).AsNumber(); l != 3 {
		t.Fatalf("a.length after delete = %v, want 3 (delete does not shorten)", l)
	}
	if !hasIndex(a, 0) || !hasIndex(a, 2) {
		t.Fatal("delete a[1] disturbed a neighboring index")
	}
}

// TestStoredUndefinedIsPresent proves a stored undefined is a present own property,
// distinct from a hole: the in operator says true where the same read yields
// undefined for both.
func TestStoredUndefinedIsPresent(t *testing.T) {
	a := NewArrayValue([]Value{Number(1), Undefined, Number(3)})
	if !hasIndex(a, 1) {
		t.Fatal("1 in [1, undefined, 3] = false, want true (a stored undefined is present)")
	}
	if a.GetIndex(1).kind != KindUndefined {
		t.Fatalf("a[1] = %v, want undefined", a.GetIndex(1))
	}
}

// TestSparseGrowthMakesHoles proves that assigning past the end grows the array
// with holes, not stored undefineds: the skipped indices are absent, while the
// written one is present, and the length spans them all.
func TestSparseGrowthMakesHoles(t *testing.T) {
	a := NewArrayValue([]Value{Number(0)})
	a.SetIndex(3, Number(9))
	if l := a.Get(FromGoString("length")).AsNumber(); l != 4 {
		t.Fatalf("length after a[3]=9 = %v, want 4", l)
	}
	if hasIndex(a, 1) || hasIndex(a, 2) {
		t.Fatal("sparse growth left a present index where a hole was expected")
	}
	if !hasIndex(a, 3) {
		t.Fatal("the written index a[3] is not present")
	}
}

// TestHoleSkippedByOwnKeys proves a hole is not enumerated: Object.keys over an
// array with a hole lists the present indices only.
func TestHoleSkippedByOwnKeys(t *testing.T) {
	a := NewArrayValue([]Value{Number(1), Number(2), Number(3)})
	a.DeleteIndex(1)
	keys := a.OwnKeys().Elems()
	var got []string
	for _, k := range keys {
		got = append(got, k.ToGoString())
	}
	if len(got) != 2 || got[0] != "0" || got[1] != "2" {
		t.Fatalf("own keys after delete a[1] = %v, want [0 2]", got)
	}
}

// TestHoleHasNoDescriptor proves getOwnPropertyDescriptor over a hole reports the
// property is absent, the same as an out-of-range index.
func TestHoleHasNoDescriptor(t *testing.T) {
	a := NewArrayValue([]Value{Number(1), Number(2), Number(3)})
	a.DeleteIndex(1)
	if d := a.GetOwnPropertyDescriptor(Number(1)); d.kind != KindUndefined {
		t.Fatalf("descriptor of hole a[1] = %v, want undefined", d)
	}
}

// TestLengthGrowMakesHoles proves that writing a larger length extends the array with
// holes: the length spans the new indices, the original elements stay present, and the
// added indices are absent own properties that read undefined.
func TestLengthGrowMakesHoles(t *testing.T) {
	a := NewArrayValue([]Value{Number(1), Number(2), Number(3)})
	a.SetKey(FromGoString("length"), Number(5))
	if l := a.Get(FromGoString("length")).AsNumber(); l != 5 {
		t.Fatalf("length after grow = %v, want 5", l)
	}
	if !hasIndex(a, 0) || !hasIndex(a, 2) {
		t.Fatal("grow dropped an original element")
	}
	if hasIndex(a, 3) || hasIndex(a, 4) {
		t.Fatal("grow left a present index where a hole was expected")
	}
	if a.GetIndex(4).kind != KindUndefined {
		t.Fatalf("a[4] after grow = %v, want undefined", a.GetIndex(4))
	}
}

// TestLengthShrinkTruncates proves that writing a smaller length drops the tail: the
// length shrinks, the surviving elements stay present, and the dropped indices are
// absent.
func TestLengthShrinkTruncates(t *testing.T) {
	a := NewArrayValue([]Value{Number(1), Number(2), Number(3), Number(4), Number(5)})
	a.SetKey(FromGoString("length"), Number(2))
	if l := a.Get(FromGoString("length")).AsNumber(); l != 2 {
		t.Fatalf("length after shrink = %v, want 2", l)
	}
	if !hasIndex(a, 0) || !hasIndex(a, 1) {
		t.Fatal("shrink dropped a surviving element")
	}
	if hasIndex(a, 2) || hasIndex(a, 4) {
		t.Fatal("shrink left a dropped index present")
	}
}

// TestLengthInvalidThrows proves that a length that ToUint32 does not leave unchanged,
// a negative or fractional value, throws a RangeError rather than resizing.
func TestLengthInvalidThrows(t *testing.T) {
	for _, bad := range []Value{Number(-1), Number(1.5)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("a.length = %v did not throw", bad)
				}
			}()
			a := NewArrayValue([]Value{Number(1)})
			a.SetKey(FromGoString("length"), bad)
		}()
	}
}
