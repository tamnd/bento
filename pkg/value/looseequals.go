package value

import "math"

// This file lowers loose equality (== and !=) over dynamic values, the Abstract
// Equality Comparison. Unlike ===, it coerces across kinds before comparing, so
// 1 == "1" is true and null == undefined is true. The lowering maps == to
// value.LooseEquals(a, b) and != to its negation, the readable spelling of the
// operation the operand kinds decide at runtime. The coercion has no Go operator,
// which is why it lives in the value model rather than inline.
//
// The comparison walks the spec's ladder: equal kinds defer to StrictEquals, null
// and undefined match only each other, a number against a string coerces the
// string to a number, a boolean coerces to its 0 or 1 and re-enters, an object
// coerces to its primitive and re-enters, and a bigint against a number or a
// numeric string compares by exact mathematical value rather than a rounded
// float64.

// LooseEquals implements a == b over two dynamic values.
func LooseEquals(a, b Value) bool {
	if a.kind == b.kind {
		return StrictEquals(a, b)
	}
	switch {
	case a.kind == KindNull && b.kind == KindUndefined,
		a.kind == KindUndefined && b.kind == KindNull:
		return true
	case a.kind == KindNumber && b.kind == KindString:
		return a.AsNumber() == StringToNumber(b.str())
	case a.kind == KindString && b.kind == KindNumber:
		return StringToNumber(a.str()) == b.AsNumber()
	case a.kind == KindBigInt && b.kind == KindString:
		return bigIntEqualsNumericString(a.bigint(), b.str())
	case a.kind == KindString && b.kind == KindBigInt:
		return bigIntEqualsNumericString(b.bigint(), a.str())
	case a.kind == KindBool:
		return LooseEquals(Number(BoolToNumber(a.AsBool())), b)
	case b.kind == KindBool:
		return LooseEquals(a, Number(BoolToNumber(b.AsBool())))
	case isPrimitiveOperand(a) && isReferenceOperand(b):
		return LooseEquals(a, toPrimitiveDefault(b))
	case isReferenceOperand(a) && isPrimitiveOperand(b):
		return LooseEquals(toPrimitiveDefault(a), b)
	case a.kind == KindBigInt && b.kind == KindNumber:
		return bigIntEqualsNumber(a.bigint(), b.AsNumber())
	case a.kind == KindNumber && b.kind == KindBigInt:
		return bigIntEqualsNumber(b.bigint(), a.AsNumber())
	}
	return false
}

// isPrimitiveOperand reports whether a value is one of the primitive kinds that
// compares against an object by coercing the object to its primitive: a number,
// string, bigint, or symbol. Boolean, null, and undefined are handled earlier in
// the ladder, so they are not counted here.
func isPrimitiveOperand(v Value) bool {
	switch v.kind {
	case KindNumber, KindString, KindBigInt, KindSymbol:
		return true
	default:
		return false
	}
}

// isReferenceOperand reports whether a value is a reference kind that carries a
// ToPrimitive: an object, an array, or a function.
func isReferenceOperand(v Value) bool {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return true
	default:
		return false
	}
}

// bigIntEqualsNumber reports whether a bigint equals a number by exact value. A
// NaN or infinite number equals no bigint, and a number with a fractional part
// cannot equal an integer, so both are unequal before the exact compare that a
// value past 2^53 needs to avoid a float round flattening it onto a double.
func bigIntEqualsNumber(b *BigInt, f float64) bool {
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
		return false
	}
	return bigIntCmpFloat(b, f) == 0
}

// bigIntEqualsNumericString reports whether a bigint equals the bigint a string
// denotes. A string that is not a canonical bigint literal is unequal, the same
// as the spec's StringToBigInt returning undefined; == has no error to raise, so
// it reads the failure as "not equal" rather than throwing the way BigInt(s) does.
func bigIntEqualsNumericString(b *BigInt, s BStr) bool {
	parsed, ok := parseBigInt(s)
	if !ok {
		return false
	}
	return b.i.Cmp(parsed) == 0
}
