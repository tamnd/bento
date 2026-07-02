package value

import (
	"math"
	"testing"
)

func TestNumberToBool(t *testing.T) {
	cases := []struct {
		in   float64
		want bool
	}{
		{0, false},
		{math.Copysign(0, -1), false}, // -0 is falsy
		{math.NaN(), false},           // the guard a bare x != 0 would miss
		{1, true},
		{-1, true},
		{0.5, true},
		{math.Inf(1), true},
		{math.Inf(-1), true},
		{math.SmallestNonzeroFloat64, true},
	}
	for _, c := range cases {
		if got := NumberToBool(c.in); got != c.want {
			t.Errorf("NumberToBool(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestStringToBool(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"a", true},
		{" ", true}, // whitespace is still non-empty
		{"0", true}, // content does not matter, only length
		{"false", true},
		{"😀", true},
	}
	for _, c := range cases {
		if got := StringToBool(FromGoString(c.in)); got != c.want {
			t.Errorf("StringToBool(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
