package value

import "testing"

// TestMapIndex pins the (element, index) map: each callback sees the float64
// position and the result keeps the element type.
func TestMapIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := a.MapIndex(func(x, i float64) float64 { return x + i })
	want := []float64{1, 3, 5}
	for k, w := range want {
		if got.elems[k] != w {
			t.Fatalf("MapIndex[%d] = %v, want %v", k, got.elems[k], w)
		}
	}
}

// TestMapArrayIndex pins the type-changing (element, index) map: a number array
// mapped to strings through the index.
func TestMapArrayIndex(t *testing.T) {
	a := NewArray[float64](10, 20, 30)
	got := MapArrayIndex(a, func(x, i float64) BStr {
		return NumberToString(i).ConcatN(FromGoString(":"), NumberToString(x))
	})
	want := []string{"0:10", "1:20", "2:30"}
	for k, w := range want {
		if got.elems[k].ToGoString() != w {
			t.Fatalf("MapArrayIndex[%d] = %q, want %q", k, got.elems[k].ToGoString(), w)
		}
	}
}

// TestFilterIndex pins the (element, index) filter: the predicate reads the
// position, keeping even indices.
func TestFilterIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	got := a.FilterIndex(func(x, i float64) bool { return int(i)%2 == 0 })
	want := []float64{1, 3}
	if got.Len() != 2 {
		t.Fatalf("FilterIndex Len = %v, want 2", got.Len())
	}
	for k, w := range want {
		if got.elems[k] != w {
			t.Fatalf("FilterIndex[%d] = %v, want %v", k, got.elems[k], w)
		}
	}
}
