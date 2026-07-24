package value

import (
	"math"
	"testing"
)

// TestAtomicStoreNormalizesNegativeZero proves Atomics.store runs its value through
// ToIntegerOrInfinity: storing -0 returns +0 (Object.is distinguishes them) and the
// stored element reads back +0, matching the negative-zero test262 case.
func TestAtomicStoreNormalizesNegativeZero(t *testing.T) {
	a := NewInt32Array(4)
	got := AtomicStore(a, 0, math.Copysign(0, -1))
	if math.Signbit(got) {
		t.Errorf("Atomics.store(a, 0, -0) returned -0, want +0")
	}
	if el := a.At(0); math.Signbit(el) || el != 0 {
		t.Errorf("stored element = %v (signbit %v), want +0", el, math.Signbit(el))
	}
}

// TestAtomicStoreTruncatesFraction proves a fractional value truncates toward zero
// before it is stored and returned, the ToIntegerOrInfinity rule.
func TestAtomicStoreTruncatesFraction(t *testing.T) {
	a := NewInt32Array(4)
	if got := AtomicStore(a, 0, 3.9); got != 3 {
		t.Errorf("Atomics.store(a, 0, 3.9) = %v, want 3", got)
	}
	if got := AtomicStore(a, 0, -3.9); got != -3 {
		t.Errorf("Atomics.store(a, 0, -3.9) = %v, want -3", got)
	}
}

// TestAtomicStoreKeepsInteger proves an ordinary in-range integer stores and returns
// unchanged, so the normalization does not disturb the common path.
func TestAtomicStoreKeepsInteger(t *testing.T) {
	a := NewInt32Array(4)
	if got := AtomicStore(a, 1, 42); got != 42 {
		t.Errorf("Atomics.store(a, 1, 42) = %v, want 42", got)
	}
	if a.At(1) != 42 {
		t.Errorf("stored element = %v, want 42", a.At(1))
	}
}
