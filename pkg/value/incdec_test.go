package value

import (
	"math"
	"testing"
)

// TestInc covers the ++ update over each kind: a number adds one, a string and a
// boolean coerce to a number first, null is zero so it becomes one, undefined has
// no numeric form so it is NaN, and a bigint stays a bigint at arbitrary
// precision. This is ToNumeric plus one, not Add over one, so no string case
// concatenates.
func TestInc(t *testing.T) {
	if got := Inc(Number(5)); got.kind != KindNumber || got.AsNumber() != 6 {
		t.Errorf("Inc(5) = %v, want number 6", got)
	}
	if got := Inc(StringValue(FromGoString("5"))); got.kind != KindNumber || got.AsNumber() != 6 {
		t.Errorf(`Inc("5") = %v, want number 6`, got)
	}
	if got := Inc(Bool(true)); got.kind != KindNumber || got.AsNumber() != 2 {
		t.Errorf("Inc(true) = %v, want number 2", got)
	}
	if got := Inc(Null); got.kind != KindNumber || got.AsNumber() != 1 {
		t.Errorf("Inc(null) = %v, want number 1", got)
	}
	if got := Inc(Undefined); got.kind != KindNumber || !math.IsNaN(got.AsNumber()) {
		t.Errorf("Inc(undefined) = %v, want NaN", got)
	}
	if got := Inc(BigIntFromInt64(10)); got.kind != KindBigInt || got.bigint().i.Int64() != 11 {
		t.Errorf("Inc(10n) = %v, want bigint 11", got)
	}
}

// TestDec covers the -- update: the numeric decrement sibling of Inc, a number
// minus one, a coerced string, and a bigint that stays a bigint.
func TestDec(t *testing.T) {
	if got := Dec(Number(3)); got.kind != KindNumber || got.AsNumber() != 2 {
		t.Errorf("Dec(3) = %v, want number 2", got)
	}
	if got := Dec(StringValue(FromGoString("5"))); got.kind != KindNumber || got.AsNumber() != 4 {
		t.Errorf(`Dec("5") = %v, want number 4`, got)
	}
	if got := Dec(BigIntFromInt64(2)); got.kind != KindBigInt || got.bigint().i.Int64() != 1 {
		t.Errorf("Dec(2n) = %v, want bigint 1", got)
	}
}
