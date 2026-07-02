package value

import (
	"math"
	"math/big"
	"strconv"
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

// TestParseRadixFastPathMatchesBig proves the strconv.ParseUint fast path returns
// exactly the same float64 the big.Int path would for values on both sides of the
// uint64 boundary. The fast path only fires when the digits fit in a uint64, and a
// uint64-to-float64 conversion rounds to nearest even the same way big.Float does,
// so the two must agree bit for bit; a run one digit too long must still convert
// through big.Int rather than fall to NaN.
func TestParseRadixFastPathMatchesBig(t *testing.T) {
	cases := []struct {
		digits string
		base   int
	}{
		{"0", 16},
		{"ff", 16},
		{"ffffffff", 16},              // 2^32-1
		{"ffffffffffffffff", 16},      // uint64 max, the last value the fast path takes
		{"10000000000000000", 16},     // 2^64, one past the boundary, big.Int path
		{"fffffffffffffffff", 16},     // 17 hex digits, well past uint64
		{"1111111111111111111", 2},    // a long binary run
		{"7777777777777777777", 8},    // octal, within uint64
		{"1777777777777777777777", 8}, // uint64 max in octal, still the fast path
		{"7fffffffffffffff", 16},      // int64 max, the sign bit clear
		{"8000000000000000", 16},      // the sign bit set, a value ParseInt would reject
	}
	for _, c := range cases {
		got := parseRadix(c.digits, c.base)
		// Reference: the pre-change path, big.Int then big.Float rounded to float64.
		i, ok := new(big.Int).SetString(c.digits, c.base)
		if !ok {
			t.Fatalf("test digits %q base %d did not parse as big.Int", c.digits, c.base)
		}
		want, _ := new(big.Float).SetInt(i).Float64()
		if got != want {
			t.Errorf("parseRadix(%q, %d) = %v, want %v", c.digits, c.base, got, want)
		}
	}
}

// TestStringToNumberTrimsNonASCIIWhitespace pins that the Go-string trim path
// removes the full ECMAScript StrWhiteSpace set, not just the ASCII part, so a
// value wrapped in a non-breaking space or a byte-order mark still parses. These
// are the code points the code-unit trim handled that a naive byte trim would
// miss. The whitespace is written with explicit escapes so the source stays ASCII.
func TestStringToNumberTrimsNonASCIIWhitespace(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"\u00A042\u00A0", 42},     // no-break space
		{"\uFEFF7", 7},             // byte-order mark
		{"\u20051.5\u2005", 1.5},   // four-per-em space
		{"\u3000255\u3000", 255},   // ideographic space
		{"\u202812\u2029", 12},     // line and paragraph separators
		{"\t \u00A09\u00A0 \n", 9}, // mixed ASCII and non-ASCII whitespace
	}
	for _, c := range cases {
		got := StringToNumber(FromGoString(c.in))
		if got != c.want {
			t.Errorf("StringToNumber(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestParseUintFloatAgreement is the property the fast path rests on, stated
// directly: for every uint64, converting to float64 gives the same value as
// routing it through big.Float. If a future Go release ever changed either
// rounding, this catches it before parseRadix silently diverges.
func TestParseUintFloatAgreement(t *testing.T) {
	for _, u := range []uint64{
		0, 1, 2, 255, 1<<24 - 1, 1 << 24, 1<<53 - 1, 1 << 53, 1<<53 + 1,
		1<<63 - 1, 1 << 63, math.MaxUint64,
	} {
		direct := float64(u)
		viaBig, _ := new(big.Float).SetInt(new(big.Int).SetUint64(u)).Float64()
		if direct != viaBig {
			t.Errorf("float64(%d) = %v, but big.Float path = %v", u, direct, viaBig)
		}
		// And the whole parse agrees when handed the decimal digits of u.
		if got := parseRadix(strconv.FormatUint(u, 10), 10); got != direct {
			t.Errorf("parseRadix(%d, 10) = %v, want %v", u, got, direct)
		}
	}
}
