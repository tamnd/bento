package value

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// referenceNumberToString is the split-and-reconcatenate form NumberToString
// used before it moved to a stack buffer. It is kept here as an independent
// oracle: the buffer version must agree with it byte for byte on every value, so
// the rewrite cannot have changed a single character of any String(x).
func referenceNumberToString(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case x == 0:
		return "0"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	}
	if x == math.Trunc(x) && x >= -twoPow53 && x <= twoPow53 {
		return strconv.FormatInt(int64(x), 10)
	}
	if x < 0 {
		return "-" + refFormatFinite(-x)
	}
	return refFormatFinite(x)
}

func refFormatFinite(x float64) string {
	s := strconv.FormatFloat(x, 'e', -1, 64)
	mant, expStr, _ := strings.Cut(s, "e")
	exp, _ := strconv.Atoi(expStr)
	if before, after, found := strings.Cut(mant, "."); found {
		mant = before + after
	}
	digits := mant
	k := len(digits)
	n := exp + 1
	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	default:
		mant := digits
		if len(digits) > 1 {
			mant = digits[:1] + "." + digits[1:]
		}
		e := n - 1
		sign := "+"
		if e < 0 {
			sign = "-"
			e = -e
		}
		return mant + "e" + sign + strconv.Itoa(e)
	}
}

// TestNumberToStringMatchesReference walks a broad set of magnitudes, including
// the thresholds where JavaScript flips into exponential notation, and every
// bit-adjacent double around a spread of seeds, and requires the buffer
// formatter to produce exactly what the reference does. A mismatch is a
// regression in the rewrite, not a matter of taste, since the reference is the
// prior shipped behavior.
func TestNumberToStringMatchesReference(t *testing.T) {
	check := func(x float64) {
		t.Helper()
		got := NumberToString(x).ToGoString()
		want := referenceNumberToString(x)
		if got != want {
			t.Fatalf("NumberToString(%v): got %q, want %q", x, got, want)
		}
	}

	// The numfmt workload's own distribution.
	for i := 1; i < 3000; i++ {
		x := float64(i) * 1.000001
		check(x)
		check(x * 1e18)
		check(x / 1e12)
		check(-x)
	}

	// Structured edge cases: the exponential thresholds and boundary magnitudes.
	edges := []float64{
		1, -1, 0.5, 0.1, 0.2, 0.3,
		1e-6, 1e-7, 1e-8, 9.999999e-7, 1.0000001e-6,
		1e20, 1e21, 1e22, 9.999999999999999e20, 1.0000000000000001e21,
		123456789012345680000, 12345678901234568,
		math.MaxFloat64, math.SmallestNonzeroFloat64,
		1.7976931348623157e308, 2.2250738585072014e-308,
		3.141592653589793, 2.718281828459045,
	}
	for _, x := range edges {
		check(x)
		check(-x)
	}

	// A deterministic sweep of bit-adjacent doubles around several seeds, so a
	// carry or rounding difference in the shortest-digit handling would surface.
	seeds := []float64{1, 10, 100, 1.5, 0.001, 1e-7, 1e21, 6.022e23, 1.602e-19}
	for _, seed := range seeds {
		x := seed
		for i := 0; i < 200; i++ {
			check(x)
			check(-x)
			x = math.Nextafter(x, math.Inf(1))
		}
	}
}
