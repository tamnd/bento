// This file gives a primitive its prototype members on the dynamic path: the six on
// Number.prototype, the two on Boolean.prototype, and the two on BigInt.prototype.
//
// The statically typed path never needs them. A number the checker types as a number
// has its toFixed and its toString(radix) lowered to a direct value.NumberToFixed or
// value.NumberToStringRadix by calls.go, with the argument range checked at compile
// time where it can be. What reaches here is the other case: a receiver the checker
// typed any, so the member is read with a dynamic Get and the result invoked. That is
// the shape a Node program is full of, because most of what a Node API hands back is
// typed any by bento's ambient declarations. Buffer's read family is the immediate
// one, where buf.readUInt32BE(0).toString(16) reads a member off a number the checker
// only knows as any.
//
// Every member here delegates to the same formatter the static path emits, so a number
// formatted through a dynamic call and one formatted through a lowered call are the
// same digits, and a bad argument raises the same RangeError from the same place.

package value

import (
	"strconv"
)

// numberGet answers a member read off a number, the dynamic half of Number.prototype.
// It reports false for a name that is not one of them, so the read falls through to
// undefined the way a miss on any other receiver does.
func numberGet(x float64, name string) (Value, bool) {
	switch name {
	case "toString":
		return boundMethod("toString", func(args []Value) Value {
			// An omitted radix is Number::toString, base ten, and is not the same rule as
			// radix 10 passed explicitly only in that it skips the range check.
			if r := Arg(args, 0); r.kind != KindUndefined {
				return StringValue(NumberToStringRadixDynamic(x, ToNumber(r)))
			}
			return StringValue(NumberToString(x))
		}), true

	case "toLocaleString":
		// bento has no locale data, so toLocaleString is toString. Saying so plainly is
		// better than grouping digits by a guessed locale, which would be wrong for most
		// of them and unverifiable for the rest.
		return boundMethod("toLocaleString", func(args []Value) Value {
			return StringValue(NumberToString(x))
		}), true

	case "valueOf":
		return boundMethod("valueOf", func(args []Value) Value { return Number(x) }), true

	case "toFixed":
		return boundMethod("toFixed", func(args []Value) Value {
			return StringValue(NumberToFixedDynamic(x, numberFormatArg(Arg(args, 0))))
		}), true

	case "toPrecision":
		return boundMethod("toPrecision", func(args []Value) Value {
			// toPrecision is the one of the three whose omitted form is not its zero form:
			// with no argument it is String(x) outright, not a one-significant-digit render.
			if Arg(args, 0).kind == KindUndefined {
				return StringValue(NumberToString(x))
			}
			return StringValue(NumberToPrecisionDynamic(x, ToNumber(args[0])))
		}), true

	case "toExponential":
		return boundMethod("toExponential", func(args []Value) Value {
			if Arg(args, 0).kind == KindUndefined {
				return StringValue(numberToExponentialShortest(x))
			}
			return StringValue(NumberToExponentialDynamic(x, ToNumber(args[0])))
		}), true
	}
	return Undefined, false
}

// numberFormatArg coerces a digit-count argument, reading an omitted one as zero the
// way toFixed does.
func numberFormatArg(v Value) float64 {
	if v.kind == KindUndefined {
		return 0
	}
	return ToNumber(v)
}

// numberToExponentialShortest is n.toExponential() with no digit count: the spec says
// to use as many fraction digits as are needed to name the value uniquely, so the
// count is the smallest one whose render reads back as the same double.
func numberToExponentialShortest(x float64) BStr {
	for d := 0; d <= 100; d++ {
		s := NumberToExponential(x, d)
		if back, err := strconv.ParseFloat(s.ToGoString(), 64); err == nil && back == x {
			return s
		}
	}
	return NumberToExponential(x, 100)
}

// boolGet answers a member read off a boolean, the whole of Boolean.prototype.
func boolGet(b bool, name string) (Value, bool) {
	switch name {
	case "toString":
		return boundMethod("toString", func(args []Value) Value { return StringValue(BoolToString(b)) }), true
	case "valueOf":
		return boundMethod("valueOf", func(args []Value) Value { return Bool(b) }), true
	}
	return Undefined, false
}

// bigIntGet answers a member read off a bigint, the three on BigInt.prototype. The
// radix form validates its argument and raises the same RangeError the lowered
// value.BigIntToStringRadix does, since it is that function.
func bigIntGet(v Value, name string) (Value, bool) {
	switch name {
	case "toString", "toLocaleString":
		return boundMethod(name, func(args []Value) Value {
			if r := Arg(args, 0); r.kind != KindUndefined {
				return StringValue(BigIntToStringRadix(&v.bigint().i, ToNumber(r)))
			}
			return StringValue(BigIntToString(&v.bigint().i))
		}), true
	case "valueOf":
		return boundMethod("valueOf", func(args []Value) Value { return v }), true
	}
	return Undefined, false
}
