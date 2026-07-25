package value

import "testing"

// Temporal.Instant.from must reject a string whose date digits form no real ISO date, even
// though the string is otherwise well shaped: day zero, month zero or thirteen, and a day past
// the month's length are all RangeErrors, the same as every other date-bearing string form.
func TestInstantFromStringRejectsInvalidDate(t *testing.T) {
	bad := []string{
		"2020-01-00T00:00Z",
		"2020-00-01T00:00Z",
		"2020-13-01T00:00Z",
		"2020-02-30T00:00Z",
	}
	for _, s := range bad {
		if got := catchThrow(func() { InstantFromString(s) }); got != "RangeError" {
			t.Errorf("InstantFromString(%q) threw %q, want RangeError", s, got)
		}
	}
	// A valid neighbour still parses.
	if got := catchThrow(func() { InstantFromString("2020-01-01T00:00Z") }); got != "" {
		t.Errorf("InstantFromString of a valid date threw %q, want no throw", got)
	}
}
