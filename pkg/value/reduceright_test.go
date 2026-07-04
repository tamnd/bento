package value

import "testing"

// TestReduceRight pins the initial-value right fold: subtracting from the right
// over [1, 2, 3] starting at 0 gives ((0-3)-2)-1 = -6, so order matters.
func TestReduceRight(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := ReduceRight(a, func(acc, x float64) float64 { return acc - x }, 0)
	if got != -6 {
		t.Errorf("ReduceRight = %v, want -6", got)
	}
}

// TestReduceRightChangingType pins that the accumulator type can differ from the
// element type, folding numbers into a string that spells them last to first.
func TestReduceRightChangingType(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := ReduceRight(a, func(acc BStr, x float64) BStr {
		return acc.ConcatN(NumberToString(x))
	}, FromGoString(""))
	if got.ToGoString() != "321" {
		t.Errorf("ReduceRight string = %q, want %q", got.ToGoString(), "321")
	}
}

// TestReduceRightEmptyReturnsInit pins that an empty array returns the initial
// value unchanged, never running the callback.
func TestReduceRightEmptyReturnsInit(t *testing.T) {
	a := NewArray[float64]()
	got := ReduceRight(a, func(acc, x float64) float64 {
		t.Fatal("callback ran on an empty reduceRight")
		return acc
	}, 42)
	if got != 42 {
		t.Errorf("ReduceRight empty = %v, want 42", got)
	}
}

// TestReduceRightNoInit pins the no-init right fold: the accumulator seeds from
// the last element and folds toward the first, so subtracting over [1, 2, 3, 10]
// gives ((10-3)-2)-1 = 4.
func TestReduceRightNoInit(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 10)
	got := a.ReduceRightNoInit(func(acc, x float64) float64 { return acc - x })
	if got != 4 {
		t.Errorf("ReduceRightNoInit = %v, want 4", got)
	}
}

// TestReduceRightNoInitSingle pins that a one-element array returns that element
// without running the callback.
func TestReduceRightNoInitSingle(t *testing.T) {
	a := NewArray[float64](7)
	got := a.ReduceRightNoInit(func(acc, x float64) float64 {
		t.Fatal("callback ran on a single-element reduceRight")
		return acc
	})
	if got != 7 {
		t.Errorf("ReduceRightNoInit single = %v, want 7", got)
	}
}

// TestReduceRightNoInitEmptyThrows pins that an empty array throws a TypeError
// with the same message JavaScript uses.
func TestReduceRightNoInitEmptyThrows(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("ReduceRightNoInit on empty array did not throw")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("ReduceRightNoInit threw %T, want *Error", r)
		}
		if e.Name().ToGoString() != "TypeError" {
			t.Errorf("ReduceRightNoInit threw %q, want TypeError", e.Name().ToGoString())
		}
		if got := e.ErrorMessage(); got != "Reduce of empty array with no initial value" {
			t.Errorf("ReduceRightNoInit message = %q", got)
		}
	}()
	NewArray[float64]().ReduceRightNoInit(func(acc, x float64) float64 { return acc - x })
}
