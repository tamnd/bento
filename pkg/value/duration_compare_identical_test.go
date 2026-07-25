package value

import "testing"

// Temporal.Duration.compare returns 0 for two instances that agree on every field, and it does
// so before it requires a relativeTo reference. A same-length but non-identical calendar pair
// still needs the reference and throws without one.
func TestDurationCompareIdenticalShortCircuits(t *testing.T) {
	d1 := NewDuration(5, 5, 5, 5, 5, 5, 5, 5, 5, 5)
	d2 := NewDuration(5, 5, 5, 5, 5, 5, 5, 5, 5, 5)
	if got := DurationCompare(d1, d2, nil); got != 0 {
		t.Fatalf("identical durations without relativeTo: got %v, want 0", got)
	}

	// Same length (all carry a year) but not identical: the relativeTo requirement still applies.
	a := NewDuration(1, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	b := NewDuration(2, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if name := catchThrow(func() { DurationCompare(a, b, nil) }); name != "RangeError" {
		t.Fatalf("distinct year durations without relativeTo: got %q, want RangeError", name)
	}
}
