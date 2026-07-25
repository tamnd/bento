package value

import "testing"

// TestReduceIndex pins the (accumulator, element, index) left fold with an
// initial value: the callback sees each element's float64 position.
func TestReduceIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	got := ReduceIndex(a, func(acc, x, i float64) float64 { return acc + x*i }, 0)
	// 1*0 + 2*1 + 3*2 + 4*3 = 20
	if got != 20 {
		t.Fatalf("ReduceIndex = %v, want 20", got)
	}
}

// TestReduceNoInitIndex pins the no-init left fold: the seed is the first
// element and the first callback index is 1.
func TestReduceNoInitIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	got := a.ReduceNoInitIndex(func(acc, x, i float64) float64 { return acc + x*i })
	// seed 1; +2*1 +3*2 +4*3 = 21
	if got != 21 {
		t.Fatalf("ReduceNoInitIndex = %v, want 21", got)
	}
}

// TestReduceRightIndex pins the (accumulator, element, index) right fold with an
// initial value: elements are visited last-to-first with descending indices.
func TestReduceRightIndex(t *testing.T) {
	a := NewArray[BStr](FromGoString("a"), FromGoString("b"), FromGoString("c"))
	got := ReduceRightIndex(a, func(acc, x BStr, i float64) BStr {
		return acc.ConcatN(x, NumberToString(i))
	}, FromGoString(""))
	if got.ToGoString() != "c2b1a0" {
		t.Fatalf("ReduceRightIndex = %q, want %q", got.ToGoString(), "c2b1a0")
	}
}

// TestReduceRightNoInitIndex pins the no-init right fold: the seed is the last
// element and the first callback index is len-2.
func TestReduceRightNoInitIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	got := a.ReduceRightNoInitIndex(func(acc, x, i float64) float64 { return acc + x*i })
	// seed 4; +3*2 +2*1 +1*0 = 12
	if got != 12 {
		t.Fatalf("ReduceRightNoInitIndex = %v, want 12", got)
	}
}
