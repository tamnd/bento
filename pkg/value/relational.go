package value

import (
	"math"
	"math/big"
)

// This file lowers the four relational operators over dynamic values: <, <=, >,
// and >= where an operand's kind is only known at runtime. They all share the
// Abstract Relational Comparison, the spec operation behind every one of them, so
// the runtime carries that operation once and the four public helpers spell each
// operator on top of it. A Go developer would write value.Less(a, b) here, so the
// emitted AOT stays readable; the coercion the comparison needs (ToPrimitive, then
// a string order or a numeric compare) has no stdlib spelling that matches the
// language, which is why it lives in the value model rather than inline.
//
// The comparison is three-valued: x < y is true, false, or undefined, the last
// when either operand becomes NaN, because NaN orders below and above nothing. The
// four operators fold that third state differently, so the core returns it and the
// helpers each decide what undefined means for them: for < and > it is false, and
// for <= and >= it is also false, since a <= b is "not (b < a) and not undefined".

// The three states of the Abstract Relational Comparison. undefined is its own
// state because NaN makes every one of <, <=, >, and >= false, which is not the
// same as the comparison simply being false.
const (
	relFalse = iota
	relTrue
	relUndefined
)

// Less implements a < b over two dynamic values.
func Less(a, b Value) bool { return relCompare(a, b) == relTrue }

// Greater implements a > b, which is b < a with the operands swapped.
func Greater(a, b Value) bool { return relCompare(b, a) == relTrue }

// LessEqual implements a <= b, which holds exactly when b < a is false and not
// undefined: an operand that makes b < a undefined also makes a <= b false.
func LessEqual(a, b Value) bool { return relCompare(b, a) == relFalse }

// GreaterEqual implements a >= b, which holds exactly when a < b is false and not
// undefined, the mirror of LessEqual.
func GreaterEqual(a, b Value) bool { return relCompare(a, b) == relFalse }

// relCompare runs the Abstract Relational Comparison x < y. Both operands take
// ToPrimitive with the number hint, then two strings order by code unit and every
// other pair compares as numbers, except that a bigint keeps its exact value
// rather than round through a float64. The dynamic path has no user valueOf, so
// ToPrimitive has no side effect and the spec's left-to-right ordering is not
// observable; the operands are coerced in place.
func relCompare(x, y Value) int {
	px := toPrimitiveNumber(x)
	py := toPrimitiveNumber(y)
	if px.kind == KindString && py.kind == KindString {
		return relBool(px.str().Compare(py.str()) < 0)
	}
	if px.kind == KindBigInt || py.kind == KindBigInt {
		return bigIntRelCompare(px, py)
	}
	nx := ToNumber(px)
	ny := ToNumber(py)
	if math.IsNaN(nx) || math.IsNaN(ny) {
		return relUndefined
	}
	return relBool(nx < ny)
}

// bigIntRelCompare orders a pair where at least one side is a bigint. Two bigints
// compare exactly through big.Int, and a bigint against a number compares through
// big.Float so a value past 2^53 is not flattened onto a double; a NaN number
// operand leaves the comparison undefined, the same as the numeric path. A string
// operand has already coerced to a number by the time it reaches here through the
// caller's ToPrimitive, so the mixed case is always bigint against a number.
func bigIntRelCompare(px, py Value) int {
	switch {
	case px.kind == KindBigInt && py.kind == KindBigInt:
		return relBool(px.bigint().i.Cmp(&py.bigint().i) < 0)
	case px.kind == KindBigInt:
		ny := ToNumber(py)
		if math.IsNaN(ny) {
			return relUndefined
		}
		return relBool(bigIntCmpFloat(px.bigint(), ny) < 0)
	default:
		nx := ToNumber(px)
		if math.IsNaN(nx) {
			return relUndefined
		}
		return relBool(bigIntCmpFloat(py.bigint(), nx) > 0)
	}
}

// bigIntCmpFloat compares a bigint against a float64 exactly, returning the sign
// of bigint minus the float. An infinite float has no big.Float form, so the two
// infinities are handled directly: every finite bigint is below +Inf and above
// -Inf.
func bigIntCmpFloat(b *BigInt, f float64) int {
	if math.IsInf(f, 1) {
		return -1
	}
	if math.IsInf(f, -1) {
		return 1
	}
	return new(big.Float).SetInt(&b.i).Cmp(big.NewFloat(f))
}

// relBool maps a Go comparison result to the true/false states, leaving the
// undefined state to the callers that produce it.
func relBool(b bool) int {
	if b {
		return relTrue
	}
	return relFalse
}
