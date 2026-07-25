package value

import "testing"

// A single large Duration field near Number.MAX_SAFE_INTEGER (or that value scaled up for the
// sub-second units) is a valid Duration and must construct without throwing. Adding such a
// duration to itself overflows the representable range, and the balanced result must throw a
// RangeError rather than wrap a huge field down to a small one through an int64 truncation.
func TestDurationMaxFieldConstructsAndAddOverflows(t *testing.T) {
	const (
		maxSec = 9007199254740991.0
		maxMs  = 9007199254740991487.0
		maxUs  = 9007199254740991475711.0
		maxNs  = 9007199254740991463129087.0
	)
	cases := []struct {
		name string
		d    *Duration
	}{
		{"seconds", NewDuration(0, 0, 0, 0, 0, 0, maxSec, 0, 0, 0)},
		{"milliseconds", NewDuration(0, 0, 0, 0, 0, 0, 0, maxMs, 0, 0)},
		{"microseconds", NewDuration(0, 0, 0, 0, 0, 0, 0, 0, maxUs, 0)},
		{"nanoseconds", NewDuration(0, 0, 0, 0, 0, 0, 0, 0, 0, maxNs)},
	}
	for _, c := range cases {
		if name := catchThrow(func() { c.d.Add(c.d) }); name != "RangeError" {
			t.Errorf("%s: add overflow got %q, want RangeError", c.name, name)
		}
	}
}

// A balanced Duration whose finest fields sum up into the seconds field must keep full precision
// when the total runs past 2^53: the balancing splits a big.Int, and each field must survive as a
// float64 without an int64 wrap. toJSON renders the balanced seconds with its fractional part.
func TestDurationMaxBalanceToJSON(t *testing.T) {
	const maxSecs = 9007199254740991.0
	cases := []struct {
		name string
		d    *Duration
		want string
	}{
		{"max ms field", NewDuration(0, 0, 0, 0, 0, 0, 0, maxSecs, 0, 0), "PT9007199254740.991S"},
		{"balance ms to s", NewDuration(0, 0, 0, 0, 0, 0, maxSecs-9007199254740, maxSecs, 0, 0), "PT9007199254740991.991S"},
		{"balance ns to s", NewDuration(0, 0, 0, 0, 0, 0, maxSecs-9007199, 0, 0, maxSecs), "PT9007199254740991.254740991S"},
	}
	for _, c := range cases {
		var got string
		if name := catchThrow(func() { got = c.d.ToJSON().ToGoString() }); name != "" {
			t.Errorf("%s: unexpected throw %q", c.name, name)
			continue
		}
		if got != c.want {
			t.Errorf("%s: toJSON got %q, want %q", c.name, got, c.want)
		}
	}
}

// The range check must be symmetric in sign: a valid Duration and its negation are both valid.
// The exact total of the day-and-below fields for these maxima is 9007199254740991.something
// seconds, below the 2^53-second limit, so neither sign goes out of range. An earlier check
// floored each sub-second field to whole seconds before the magnitude test, which rounded a
// negative field away from zero and pushed its magnitude up to exactly 2^53, rejecting the
// negative arm of an otherwise valid pair.
func TestDurationMaxBalanceSignSymmetric(t *testing.T) {
	const maxSecs = 9007199254740991.0
	cases := []struct {
		name    string
		pos     *Duration
		neg     *Duration
		wantPos string
	}{
		{
			"ms, us, ns and s",
			NewDuration(0, 0, 0, 0, 0, 0, maxSecs, 999, 999, 999),
			NewDuration(0, 0, 0, 0, 0, 0, -maxSecs, -999, -999, -999),
			"PT9007199254740991.999999999S",
		},
		{
			"balance ms to s",
			NewDuration(0, 0, 0, 0, 0, 0, maxSecs-9007199254740, maxSecs, 0, 0),
			NewDuration(0, 0, 0, 0, 0, 0, -(maxSecs - 9007199254740), -maxSecs, 0, 0),
			"PT9007199254740991.991S",
		},
		{
			"balance ns to s",
			NewDuration(0, 0, 0, 0, 0, 0, maxSecs-9007199, 0, 0, maxSecs),
			NewDuration(0, 0, 0, 0, 0, 0, -(maxSecs - 9007199), 0, 0, -maxSecs),
			"PT9007199254740991.254740991S",
		},
	}
	for _, c := range cases {
		var gotPos, gotNeg string
		if name := catchThrow(func() { gotPos = c.pos.ToJSON().ToGoString() }); name != "" {
			t.Errorf("%s: positive arm threw %q", c.name, name)
		} else if gotPos != c.wantPos {
			t.Errorf("%s: positive toJSON got %q, want %q", c.name, gotPos, c.wantPos)
		}
		if name := catchThrow(func() { gotNeg = c.neg.ToJSON().ToGoString() }); name != "" {
			t.Errorf("%s: negative arm threw %q", c.name, name)
		} else if want := "-" + c.wantPos; gotNeg != want {
			t.Errorf("%s: negative toJSON got %q, want %q", c.name, gotNeg, want)
		}
	}
}
