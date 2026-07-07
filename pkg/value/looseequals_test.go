package value

import (
	"math"
	"testing"
)

// TestLooseEqualsSameKind pins that operands of the same kind defer to the strict
// comparison: two equal numbers match, two different strings do not, and NaN
// matches nothing, since the ladder starts by handing the same-kind case to
// StrictEquals.
func TestLooseEqualsSameKind(t *testing.T) {
	if !LooseEquals(num(2), num(2)) {
		t.Error("2 == 2 should be true")
	}
	if LooseEquals(str("a"), str("b")) {
		t.Error(`"a" == "b" should be false`)
	}
	if LooseEquals(num(math.NaN()), num(math.NaN())) {
		t.Error("NaN == NaN should be false")
	}
}

// TestLooseEqualsNullUndefined pins that null and undefined are loosely equal to
// each other and to nothing else, the one cross-kind pair the ladder matches
// before any coercion.
func TestLooseEqualsNullUndefined(t *testing.T) {
	if !LooseEquals(Null, Undefined) {
		t.Error("null == undefined should be true")
	}
	if !LooseEquals(Undefined, Null) {
		t.Error("undefined == null should be true")
	}
	if LooseEquals(Null, num(0)) {
		t.Error("null == 0 should be false")
	}
	if LooseEquals(Undefined, num(0)) {
		t.Error("undefined == 0 should be false")
	}
}

// TestLooseEqualsNumberString pins that a number against a string coerces the
// string to a number, so 1 == "1" holds and a non-numeric string never matches.
func TestLooseEqualsNumberString(t *testing.T) {
	if !LooseEquals(num(1), str("1")) {
		t.Error(`1 == "1" should be true`)
	}
	if !LooseEquals(str("1"), num(1)) {
		t.Error(`"1" == 1 should be true`)
	}
	if LooseEquals(num(1), str("x")) {
		t.Error(`1 == "x" should be false`)
	}
	if !LooseEquals(num(0), str("")) {
		t.Error(`0 == "" should be true, the empty string coerces to 0`)
	}
}

// TestLooseEqualsBoolean pins that a boolean coerces to its 0 or 1 and re-enters
// the ladder, so true == 1, false == 0, and true == "1" all hold through the
// number path.
func TestLooseEqualsBoolean(t *testing.T) {
	if !LooseEquals(Bool(true), num(1)) {
		t.Error("true == 1 should be true")
	}
	if !LooseEquals(Bool(false), num(0)) {
		t.Error("false == 0 should be true")
	}
	if !LooseEquals(Bool(true), str("1")) {
		t.Error(`true == "1" should be true`)
	}
	if LooseEquals(Bool(true), num(2)) {
		t.Error("true == 2 should be false")
	}
}

// TestLooseEqualsBigIntNumber pins bigint against number by exact value: 1n == 1,
// and 2^53+1 as a bigint is not equal to the double 2^53 that a float round would
// flatten it onto. A fractional or infinite number equals no bigint.
func TestLooseEqualsBigIntNumber(t *testing.T) {
	if !LooseEquals(bigS("1"), num(1)) {
		t.Error("1n == 1 should be true")
	}
	if !LooseEquals(num(1), bigS("1")) {
		t.Error("1 == 1n should be true")
	}
	if LooseEquals(bigS("9007199254740993"), num(9007199254740992)) {
		t.Error("(2^53+1)n == 2^53 should be false, exact compare")
	}
	if LooseEquals(bigS("1"), num(1.5)) {
		t.Error("1n == 1.5 should be false")
	}
	if LooseEquals(bigS("1"), num(math.Inf(1))) {
		t.Error("1n == Infinity should be false")
	}
	if LooseEquals(bigS("1"), num(math.NaN())) {
		t.Error("1n == NaN should be false")
	}
}

// TestLooseEqualsBigIntString pins bigint against a numeric string: "10" parses to
// 10n so 10n == "10" holds, the radix prefixes parse the same way BigInt(s) reads
// them, and a string that is not a bigint literal is unequal rather than throwing.
func TestLooseEqualsBigIntString(t *testing.T) {
	if !LooseEquals(bigS("10"), str("10")) {
		t.Error(`10n == "10" should be true`)
	}
	if !LooseEquals(str("0x10"), bigS("16")) {
		t.Error(`"0x10" == 16n should be true`)
	}
	if !LooseEquals(bigS("0"), str("  ")) {
		t.Error(`0n == "  " should be true, blank string is 0n`)
	}
	if LooseEquals(bigS("10"), str("10.5")) {
		t.Error(`10n == "10.5" should be false, not a bigint literal`)
	}
	if LooseEquals(bigS("10"), str("ten")) {
		t.Error(`10n == "ten" should be false`)
	}
}
