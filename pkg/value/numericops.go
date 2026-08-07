package value

import "math"

// This file implements the JavaScript arithmetic and bitwise operators over two
// dynamic values, the siblings of Add for every operator that is not +.
//
// They exist because a bigint and a number are two different numeric kinds that
// never mix. `2n * 2n` is 4n, `2 * 2` is 4, and `2n * 2` is a TypeError, so an
// operator whose operand kinds are only known at runtime has to decide there too.
// The checker refuses to type such an expression at all (it reports the mix as an
// error), which is exactly the signal lowering uses to route here: when the static
// answer is "there is no static answer", the runtime gives the one the language
// gives.
//
// Each operator coerces its operands with ToPrimitive under the number hint, the
// hint every arithmetic operator but + uses, then dispatches once on whether the
// pair is a bigint pair. The bigint arms reuse the same helpers the statically
// typed bigint path emits, so a dynamic 7n / 0n throws the same catchable
// RangeError a static one does, and the number arms are the same float64
// arithmetic the statically typed number path emits.

// numericOperands runs ToPrimitive on both operands under the number hint and
// reports whether they are a bigint pair. A pair with exactly one bigint throws the
// TypeError the language throws, the rule that makes 1n * 1 an error rather than 1:
// arithmetic never silently converts between the two numeric kinds in either
// direction, because a bigint is exact and a number is not.
func numericOperands(a, b Value) (Value, Value, bool) {
	pa, pb := toPrimitiveNumber(a), toPrimitiveNumber(b)
	if pa.kind == KindBigInt || pb.kind == KindBigInt {
		if pa.kind != KindBigInt || pb.kind != KindBigInt {
			Throw(NewTypeError(FromGoString("Cannot mix BigInt and other types, use explicit conversions")))
		}
		return pa, pb, true
	}
	return pa, pb, false
}

// Sub implements the - operator over two dynamic values.
func Sub(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		out := &BigInt{}
		out.i.Sub(&pa.bigint().i, &pb.bigint().i)
		return BigIntValue(out)
	}
	return Number(ToNumber(pa) - ToNumber(pb))
}

// Mul implements the * operator over two dynamic values.
func Mul(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		out := &BigInt{}
		out.i.Mul(&pa.bigint().i, &pb.bigint().i)
		return BigIntValue(out)
	}
	return Number(ToNumber(pa) * ToNumber(pb))
}

// Div implements the / operator over two dynamic values. Dividing numbers by zero
// gives an infinity, while dividing bigints by 0n throws, so the two arms differ in
// more than their result kind.
func Div(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		return BigIntFromBig(BigIntDiv(&pa.bigint().i, &pb.bigint().i))
	}
	return Number(ToNumber(pa) / ToNumber(pb))
}

// Rem implements the % operator over two dynamic values. The number arm is
// math.Mod, which keeps the sign of the dividend the way JavaScript's remainder
// does rather than the sign of the divisor a modulo would take.
func Rem(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		return BigIntFromBig(BigIntRem(&pa.bigint().i, &pb.bigint().i))
	}
	return Number(math.Mod(ToNumber(pa), ToNumber(pb)))
}

// Exponentiate implements the ** operator over two dynamic values.
func Exponentiate(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		return BigIntFromBig(BigIntPow(&pa.bigint().i, &pb.bigint().i))
	}
	return Number(Pow(ToNumber(pa), ToNumber(pb)))
}

// BitAnd implements the & operator over two dynamic values. The bigint arm computes
// on big.Int's infinite two's complement, which is the bit model a negative
// JavaScript bigint means, while the number arm narrows each side to a signed
// 32-bit integer first, which is the bit model a number means.
func BitAnd(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		out := &BigInt{}
		out.i.And(&pa.bigint().i, &pb.bigint().i)
		return BigIntValue(out)
	}
	return Number(float64(ToInt32(ToNumber(pa)) & ToInt32(ToNumber(pb))))
}

// BitOr implements the | operator over two dynamic values.
func BitOr(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		out := &BigInt{}
		out.i.Or(&pa.bigint().i, &pb.bigint().i)
		return BigIntValue(out)
	}
	return Number(float64(ToInt32(ToNumber(pa)) | ToInt32(ToNumber(pb))))
}

// BitXor implements the ^ operator over two dynamic values.
func BitXor(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		out := &BigInt{}
		out.i.Xor(&pa.bigint().i, &pb.bigint().i)
		return BigIntValue(out)
	}
	return Number(float64(ToInt32(ToNumber(pa)) ^ ToInt32(ToNumber(pb))))
}

// ShiftLeft implements the << operator over two dynamic values. A bigint shifts by
// the whole count, since a bigint has no width to overflow; a number masks the
// count to five bits, since it is shifting a 32-bit integer.
func ShiftLeft(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		return BigIntFromBig(BigIntLsh(&pa.bigint().i, &pb.bigint().i))
	}
	return Number(float64(ToInt32(ToNumber(pa)) << (ToUint32(ToNumber(pb)) & 31)))
}

// ShiftRight implements the >> operator over two dynamic values, the arithmetic
// shift that keeps the sign in both arms.
func ShiftRight(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		return BigIntFromBig(BigIntRsh(&pa.bigint().i, &pb.bigint().i))
	}
	return Number(float64(ToInt32(ToNumber(pa)) >> (ToUint32(ToNumber(pb)) & 31)))
}

// UnsignedShiftRight implements the >>> operator over two dynamic values. There is
// no bigint arm: >>> reads the operand as an unsigned integer of a fixed width, and
// a bigint has no width, so the language throws rather than pick one.
func UnsignedShiftRight(a, b Value) Value {
	pa, pb, big := numericOperands(a, b)
	if big {
		Throw(NewTypeError(FromGoString("BigInts have no unsigned right shift, use >> instead")))
	}
	return Number(float64(ToUint32(ToNumber(pa)) >> (ToUint32(ToNumber(pb)) & 31)))
}
