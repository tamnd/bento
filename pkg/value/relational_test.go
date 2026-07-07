package value

import (
	"math"
	"testing"
)

func num(f float64) Value { return Number(f) }
func str(s string) Value  { return StringValue(FromGoString(s)) }
func bigS(s string) Value {
	v, ok := BigIntFromString(s)
	if !ok {
		panic("bad bigint literal in test: " + s)
	}
	return v
}

// TestRelationalNumbers pins the numeric comparisons, the common case where both
// operands are numbers and the four operators reduce to the plain float64 order.
func TestRelationalNumbers(t *testing.T) {
	cases := []struct {
		name          string
		a, b          Value
		lt, le, g, ge bool
	}{
		{"less", num(1), num(2), true, true, false, false},
		{"equal", num(2), num(2), false, true, false, true},
		{"greater", num(3), num(2), false, false, true, true},
		{"negatives", num(-2), num(-1), true, true, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if Less(c.a, c.b) != c.lt {
				t.Errorf("Less = %v, want %v", Less(c.a, c.b), c.lt)
			}
			if LessEqual(c.a, c.b) != c.le {
				t.Errorf("LessEqual = %v, want %v", LessEqual(c.a, c.b), c.le)
			}
			if Greater(c.a, c.b) != c.g {
				t.Errorf("Greater = %v, want %v", Greater(c.a, c.b), c.g)
			}
			if GreaterEqual(c.a, c.b) != c.ge {
				t.Errorf("GreaterEqual = %v, want %v", GreaterEqual(c.a, c.b), c.ge)
			}
		})
	}
}

// TestRelationalStrings pins that two strings order by code unit, not by numeric
// value: "10" < "9" because '1' precedes '9', the opposite of the numeric order.
func TestRelationalStrings(t *testing.T) {
	if !Less(str("a"), str("b")) {
		t.Error(`Less("a","b") = false, want true`)
	}
	if !Less(str("10"), str("9")) {
		t.Error(`Less("10","9") = false, want true (code-unit order)`)
	}
	if Less(str("9"), str("10")) {
		t.Error(`Less("9","10") = true, want false`)
	}
	if !LessEqual(str("abc"), str("abc")) {
		t.Error(`LessEqual("abc","abc") = false, want true`)
	}
}

// TestRelationalMixedCoercesToNumber pins that a string against a number compares
// numerically, not by code unit: "10" is coerced to 10 the moment the other side
// is a number.
func TestRelationalMixedCoercesToNumber(t *testing.T) {
	if !Less(num(9), str("10")) {
		t.Error(`Less(9,"10") = false, want true`)
	}
	if Less(str("10"), num(9)) {
		t.Error(`Less("10",9) = true, want false`)
	}
}

// TestRelationalNaNIsAlwaysFalse pins that a NaN operand makes every one of the
// four operators false, including <= and >=, because the comparison is undefined
// rather than merely false. An empty string coerces to 0, but a non-numeric string
// coerces to NaN and takes the same path.
func TestRelationalNaNIsAlwaysFalse(t *testing.T) {
	nan := num(math.NaN())
	if Less(nan, num(1)) || LessEqual(nan, num(1)) || Greater(nan, num(1)) || GreaterEqual(nan, num(1)) {
		t.Error("a NaN operand should make all four relational operators false")
	}
	if Less(str("x"), num(1)) || LessEqual(str("x"), num(1)) {
		t.Error(`a non-numeric string coerces to NaN and should compare false`)
	}
}

// TestRelationalBigInt pins bigint ordering, including the case a float64 cannot
// represent: 2^53+1 against the double 2^53 must compare exactly, not through a
// rounded float that would call them equal.
func TestRelationalBigInt(t *testing.T) {
	if !Less(bigS("1"), bigS("2")) {
		t.Error("Less(1n,2n) = false, want true")
	}
	if !Greater(bigS("2"), bigS("1")) {
		t.Error("Greater(2n,1n) = false, want true")
	}
	// 2^53 = 9007199254740992 is the largest double with an exact successor gap of
	// one; 2^53+1 is not representable, so a float round would flatten it onto 2^53
	// and lose the comparison. The exact path must still see it as greater.
	if !Greater(bigS("9007199254740993"), num(9007199254740992)) {
		t.Error("Greater(2^53+1 n, 2^53) = false, want true (exact bigint compare)")
	}
	if Less(bigS("9007199254740993"), num(9007199254740992)) {
		t.Error("Less(2^53+1 n, 2^53) = true, want false")
	}
	// A bigint against an infinite float has no big.Float form and takes the direct
	// branch: every finite bigint is below +Inf and above -Inf.
	if !Less(bigS("1000000000000000000000"), num(math.Inf(1))) {
		t.Error("a finite bigint should be less than +Inf")
	}
	if !Greater(bigS("-1000000000000000000000"), num(math.Inf(-1))) {
		t.Error("a finite bigint should be greater than -Inf")
	}
}
