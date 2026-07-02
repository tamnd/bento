package value

import (
	"math"
	"testing"
)

func TestStringToNumber(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		// Empty and whitespace map to +0.
		{"", 0},
		{"   ", 0},
		{"\t\n\r ", 0},
		// Trimming around a value.
		{"  42  ", 42},
		{"\t10\n", 10},
		// Plain decimals.
		{"0", 0},
		{"00", 0},
		{"42", 42},
		{"3.14", 3.14},
		{"5.", 5},
		{".5", 0.5},
		{"+.5", 0.5},
		{"-0.25", -0.25},
		{"+7", 7},
		// Exponents.
		{"1e3", 1000},
		{"1E3", 1000},
		{"1.5e2", 150},
		{"2e-3", 0.002},
		{"1e999", math.Inf(1)},
		{"1e-999", 0},
		// Infinity word.
		{"Infinity", math.Inf(1)},
		{"+Infinity", math.Inf(1)},
		{"-Infinity", math.Inf(-1)},
		// Radix integers, any prefix case.
		{"0x1F", 31},
		{"0X1f", 31},
		{"0b101", 5},
		{"0B101", 5},
		{"0o17", 15},
		{"0O17", 15},
		{"  0xFF  ", 255},
		// A radix value beyond int64 still converts through big.Int.
		{"0xFFFFFFFFFFFFFFFFFF", 4722366482869645213696},
	}
	for _, c := range cases {
		got := StringToNumber(FromGoString(c.in))
		if got != c.want {
			t.Errorf("StringToNumber(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestStringToNumberNaN(t *testing.T) {
	// Each of these is not a JavaScript numeric string, so Number(s) is NaN. The
	// list targets the forms strconv.ParseFloat would wrongly accept.
	cases := []string{
		"abc",
		".",
		"1e",
		"1e+",
		"0x",
		"0b",
		"0o",
		"0xG",
		"0b2",
		"0o8",
		"0x-1",    // a sign inside the radix digits
		"Inf",     // JavaScript spells it Infinity
		"NaN",     // the word is not a numeric literal
		"0x1p-2",  // a hexadecimal float
		"1_000",   // digit separators are source-only
		"0xFF_FF", // separators rejected in radix too
		"1 2",     // interior space
		"++1",     // a doubled sign
		"5.5.5",   // two points
		"0x1.8",   // a point in a radix integer
		"123n",    // a BigInt literal is not a number string
	}
	for _, in := range cases {
		got := StringToNumber(FromGoString(in))
		if !math.IsNaN(got) {
			t.Errorf("StringToNumber(%q) = %v, want NaN", in, got)
		}
	}
}
