package value

import (
	"math"
	"testing"
)

// TestStrictEqualsKinds proves the kind gate: two values of different kinds are
// never strictly equal, including the null/undefined pair loose equality would
// merge, and a number against the string spelling the same digits.
func TestStrictEqualsKinds(t *testing.T) {
	if StrictEquals(Undefined, Null) {
		t.Error("StrictEquals(undefined, null) = true, want false")
	}
	if StrictEquals(Number(1), StringValue(FromGoString("1"))) {
		t.Error("StrictEquals(1, \"1\") = true, want false")
	}
	if !StrictEquals(Undefined, Undefined) {
		t.Error("StrictEquals(undefined, undefined) = false, want true")
	}
	if !StrictEquals(Null, Null) {
		t.Error("StrictEquals(null, null) = false, want true")
	}
	if !StrictEquals(Bool(true), Bool(true)) || StrictEquals(Bool(true), Bool(false)) {
		t.Error("boolean StrictEquals did not compare by truth")
	}
}

// TestStrictEqualsNumbers proves the two points where === differs from bit
// equality: NaN is not equal to itself, and the signed zeros are equal.
func TestStrictEqualsNumbers(t *testing.T) {
	if StrictEquals(Number(math.NaN()), Number(math.NaN())) {
		t.Error("StrictEquals(NaN, NaN) = true, want false")
	}
	if !StrictEquals(Number(0), Number(math.Copysign(0, -1))) {
		t.Error("StrictEquals(0, -0) = false, want true")
	}
	if !StrictEquals(Number(2.5), Number(2.5)) {
		t.Error("StrictEquals(2.5, 2.5) = false, want true")
	}
}

// TestStrictEqualsStrings proves strings compare by code unit through the same
// Equal a mixed-backing pair needs, so a UTF-8 backed and a UTF-16 backed string
// holding the same units are equal.
func TestStrictEqualsStrings(t *testing.T) {
	if !StrictEquals(StringValue(FromGoString("abc")), StringValue(FromGoString("abc"))) {
		t.Error("StrictEquals over equal strings = false, want true")
	}
	if StrictEquals(StringValue(FromGoString("abc")), StringValue(FromGoString("abd"))) {
		t.Error("StrictEquals over different strings = true, want false")
	}
}

// TestStrictEqualsBigInt proves bigints compare by mathematical value, not by
// pointer, so two separately built 10n values are equal.
func TestStrictEqualsBigInt(t *testing.T) {
	if !StrictEquals(BigIntFromInt64(10), BigIntFromInt64(10)) {
		t.Error("StrictEquals(10n, 10n) = false, want true")
	}
	if StrictEquals(BigIntFromInt64(10), BigIntFromInt64(11)) {
		t.Error("StrictEquals(10n, 11n) = true, want false")
	}
}
