package value

import "testing"

// TestMathRandomRange proves every draw lands in [0, 1), the range Math.random
// and rand.Float64 share, across a large sample.
func TestMathRandomRange(t *testing.T) {
	for range 100000 {
		r := MathRandom()
		if r < 0 || r >= 1 {
			t.Fatalf("MathRandom returned %v, want a value in [0, 1)", r)
		}
	}
}

// TestMathRandomVaries proves the draws are not a constant: a run of samples holds
// more than one distinct value, which is the shape the differential oracle checks
// in place of an exact match.
func TestMathRandomVaries(t *testing.T) {
	first := MathRandom()
	varied := false
	for range 1000 {
		if MathRandom() != first {
			varied = true
			break
		}
	}
	if !varied {
		t.Fatal("MathRandom returned the same value on every draw")
	}
}
