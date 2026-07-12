package value

import "testing"

// TestArrayIterValues walks an array's elements, checking each step's value and
// that the walk reports done with undefined once the indices run out.
func TestArrayIterValues(t *testing.T) {
	src := NewArrayValue([]Value{Number(10), Number(20)})
	it := NewArrayIter(src, ArrayIterValues)
	for _, want := range []float64{10, 20} {
		r := it.Next()
		if r.Done {
			t.Fatalf("Next reported done early, want value %v", want)
		}
		if got := r.Value.AsNumber(); got != want {
			t.Errorf("Next value = %v, want %v", got, want)
		}
	}
	end := it.Next()
	if !end.Done {
		t.Errorf("Next after the last element = %+v, want done", end)
	}
	if !end.Value.IsUndefined() {
		t.Errorf("done value = %+v, want undefined", end.Value)
	}
}

// TestArrayIterKeys walks an array's indices, which the keys kind yields as numbers
// regardless of the elements.
func TestArrayIterKeys(t *testing.T) {
	src := NewArrayValue([]Value{StringValue(FromGoString("a")), StringValue(FromGoString("b")), StringValue(FromGoString("c"))})
	it := NewArrayIter(src, ArrayIterKeys)
	for _, want := range []float64{0, 1, 2} {
		r := it.Next()
		if r.Done || r.Value.AsNumber() != want {
			t.Errorf("keys Next = %+v, want index %v", r, want)
		}
	}
	if !it.Next().Done {
		t.Errorf("keys Next after the last index, want done")
	}
}

// TestArrayIterEntries walks an array's [index, element] pairs, which the entries
// kind yields as two-element arrays.
func TestArrayIterEntries(t *testing.T) {
	src := NewArrayValue([]Value{StringValue(FromGoString("x")), StringValue(FromGoString("y"))})
	it := NewArrayIter(src, ArrayIterEntries)
	wantVals := []string{"x", "y"}
	for i, want := range wantVals {
		r := it.Next()
		if r.Done {
			t.Fatalf("entries Next reported done early at %d", i)
		}
		if idx := r.Value.GetIndex(0).AsNumber(); idx != float64(i) {
			t.Errorf("entry %d index = %v, want %v", i, idx, i)
		}
		if got := ToString(r.Value.GetIndex(1)).ToGoString(); got != want {
			t.Errorf("entry %d element = %q, want %q", i, got, want)
		}
	}
	if !it.Next().Done {
		t.Errorf("entries Next after the last pair, want done")
	}
}

// TestArrayIterFromTyped boxes a typed array's elements once and then walks them,
// the path a statically typed arr.values() takes.
func TestArrayIterFromTyped(t *testing.T) {
	a := ArrayFrom([]float64{1, 2, 3})
	it := ArrayIterFromTyped(a, ArrayIterValues, func(x float64) Value { return Number(x) })
	sum := 0.0
	for {
		r := it.Next()
		if r.Done {
			break
		}
		sum += r.Value.AsNumber()
	}
	if sum != 6 {
		t.Errorf("sum over typed values = %v, want 6", sum)
	}
}
