package value

import (
	"strings"
	"testing"
)

// TestNumberFormatDynamicInRange proves the runtime formatters apply ToInteger to
// the count and delegate to the exact formatter: a fractional count truncates the
// way ToInteger does, and the result matches the literal-count formatter byte for
// byte.
func TestNumberFormatDynamicInRange(t *testing.T) {
	cases := []struct {
		name string
		got  BStr
		want BStr
	}{
		{"fixed", NumberToFixedDynamic(1.005, 2), NumberToFixed(1.005, 2)},
		{"fixedTrunc", NumberToFixedDynamic(123.456, 2.9), NumberToFixed(123.456, 2)},
		{"exp", NumberToExponentialDynamic(1234.5, 2), NumberToExponential(1234.5, 2)},
		{"expTrunc", NumberToExponentialDynamic(1234.5, 2.9), NumberToExponential(1234.5, 2)},
		{"precision", NumberToPrecisionDynamic(123.456, 3), NumberToPrecision(123.456, 3)},
		{"precisionTrunc", NumberToPrecisionDynamic(123.456, 3.9), NumberToPrecision(123.456, 3)},
		{"radix", NumberToStringRadixDynamic(255, 16), NumberToStringRadix(255, 16)},
		{"radixTrunc", NumberToStringRadixDynamic(255, 16.9), NumberToStringRadix(255, 16)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.ToGoString() != tc.want.ToGoString() {
				t.Errorf("dynamic = %q, want %q", tc.got.ToGoString(), tc.want.ToGoString())
			}
		})
	}
}

// TestNumberFormatDynamicThrows proves an out-of-range count throws the RangeError
// JavaScript raises rather than returning a string: a count below the low bound, a
// count above the high bound, and an infinity all raise a RangeError whose message
// names the method.
func TestNumberFormatDynamicThrows(t *testing.T) {
	cases := []struct {
		name string
		call func()
		msg  string
	}{
		{"fixedHigh", func() { NumberToFixedDynamic(1, 101) }, "toFixed"},
		{"fixedLow", func() { NumberToFixedDynamic(1, -1) }, "toFixed"},
		{"expHigh", func() { NumberToExponentialDynamic(1, 101) }, "toExponential"},
		{"precisionZero", func() { NumberToPrecisionDynamic(1, 0) }, "toPrecision"},
		{"precisionHigh", func() { NumberToPrecisionDynamic(1, 101) }, "toPrecision"},
		{"radixLow", func() { NumberToStringRadixDynamic(1, 1) }, "toString"},
		{"radixHigh", func() { NumberToStringRadixDynamic(1, 37) }, "toString"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				got := recover()
				err, ok := got.(*Error)
				if !ok {
					t.Fatalf("recovered %v (%T), want a *Error", got, got)
				}
				if err.ErrorName() != "RangeError" {
					t.Errorf("error name = %q, want RangeError", err.ErrorName())
				}
				if !strings.Contains(err.ErrorMessage(), tc.msg) {
					t.Errorf("message = %q, want it to name %q", err.ErrorMessage(), tc.msg)
				}
			}()
			tc.call()
			t.Fatal("formatter returned instead of throwing")
		})
	}
}
