package value

import "testing"

// TestReduceNoInit pins the no-init fold: the accumulator seeds from the first
// element and the callback runs from the second, so summing 1..4 gives 10.
func TestReduceNoInit(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	sum := a.ReduceNoInit(func(acc, x float64) float64 { return acc + x })
	if sum != 10 {
		t.Errorf("ReduceNoInit sum = %v, want 10", sum)
	}
}

// TestReduceNoInitSingle pins that a one-element array returns that element
// without ever calling the callback, since there is nothing to fold.
func TestReduceNoInitSingle(t *testing.T) {
	a := NewArray[float64](7)
	got := a.ReduceNoInit(func(acc, x float64) float64 {
		t.Fatal("callback ran on a single-element reduce")
		return acc
	})
	if got != 7 {
		t.Errorf("ReduceNoInit single = %v, want 7", got)
	}
}

// TestReduceNoInitEmptyThrows pins that an empty array throws a TypeError the way
// JavaScript does, rather than seeding a value out of nothing.
func TestReduceNoInitEmptyThrows(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("ReduceNoInit on empty array did not throw")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("ReduceNoInit threw %T, want *Error", r)
		}
		if e.Name().ToGoString() != "TypeError" {
			t.Errorf("ReduceNoInit threw %q, want TypeError", e.Name().ToGoString())
		}
		if got := e.ErrorMessage(); got != "Reduce of empty array with no initial value" {
			t.Errorf("ReduceNoInit message = %q", got)
		}
	}()
	NewArray[float64]().ReduceNoInit(func(acc, x float64) float64 { return acc + x })
}
