package value

import (
	"math"
	"math/big"
	"unsafe"
)

// This file owns the boxed bigint, the arbitrary-precision integer JavaScript
// exposes as its own primitive type (10_value_model section 5.3). A bigint never
// overflows, so there is no fixed-width Go integer that models it; the value wraps
// math/big.Int. bigint is immutable at the JavaScript level, so a *BigInt is shared
// across the boundary by pointer with no copy, the same as a string.

// BigInt is a JavaScript bigint. It is its own type, not a bare big.Int, so the
// value model owns bigint identity and a later small-int fast path can slot in
// without changing any caller: the first cut is just big.Int for correctness.
type BigInt struct {
	// i is the value. A future optimization can add an inline small-int field and
	// fall back to i only when the value does not fit; callers never see i.
	i big.Int
}

// BigIntFromInt64 boxes a Go int64 as a bigint. It is the crossing a Go int64
// projected as bigint uses and the base BigInt(n) lowers to for a number that fits
// an int64.
func BigIntFromInt64(n int64) Value {
	b := &BigInt{}
	b.i.SetInt64(n)
	return Value{kind: KindBigInt, ref: unsafe.Pointer(b)}
}

// BigIntFromString boxes the arbitrary-precision integer a decimal digit string
// denotes, the path a bigint literal like 123n lowers through when the value does
// not fit an int64. It reports false for a string that is not a canonical base-10
// integer, which the caller reports as a SyntaxError at lower time.
func BigIntFromString(s string) (Value, bool) {
	b := &BigInt{}
	if _, ok := b.i.SetString(s, 10); !ok {
		return Undefined, false
	}
	return Value{kind: KindBigInt, ref: unsafe.Pointer(b)}, true
}

// BigIntValue boxes an existing *BigInt without copying, the crossing that hands a
// bigint the typed side already lowered to *big.Int straight into the dynamic
// world. The pointer is shared because a bigint is immutable.
func BigIntValue(b *BigInt) Value {
	return Value{kind: KindBigInt, ref: unsafe.Pointer(b)}
}

// bigint returns the *BigInt a bigint box holds.
func (v Value) bigint() *BigInt { return (*BigInt)(v.ref) }

// Inc implements the ++ update over a dynamic value: the operand's ToNumeric plus
// one. Every kind but bigint coerces to a number, so "5"++ is 6 and true++ is 2,
// and a bigint stays a bigint so 9n++ is 10n, the ToNumeric contract the update
// operators keep. It differs from Add over one, which would concatenate a string
// operand rather than coerce it, so the update stays numeric on every kind.
func Inc(v Value) Value { return bumpNumeric(v, 1) }

// Dec implements the -- update, the numeric decrement sibling of Inc: the
// operand's ToNumeric minus one, a bigint staying a bigint.
func Dec(v Value) Value { return bumpNumeric(v, -1) }

// bumpNumeric adds a small integer delta to a value under ToNumeric, the shared
// body of the increment and decrement updates. A bigint adds through big.Int so
// it keeps arbitrary precision; every other kind coerces to a double and adds
// there, where ToNumber already throws on a value that has no numeric form.
func bumpNumeric(v Value, delta int64) Value {
	if v.kind == KindBigInt {
		sum := &BigInt{}
		sum.i.Add(&v.bigint().i, big.NewInt(delta))
		return BigIntValue(sum)
	}
	return Number(ToNumber(v) + float64(delta))
}

// Int is the arbitrary-precision value, for a caller that needs the underlying
// math/big.Int, such as the bridge marshaling a bigint back to a Go integer.
func (b *BigInt) Int() *big.Int { return &b.i }

// String renders a bigint as its decimal digits with no suffix, the value String(b)
// and a `${b}` template produce: a bigint's ToString is just its digits, the "n"
// suffix belongs only to how console.log inspects a value, not to string coercion.
func (b *BigInt) String() string { return b.i.String() }

// IsZero reports whether the bigint is 0n, the falsy bigint.
func (b *BigInt) IsZero() bool { return b.i.Sign() == 0 }

// BigIntToString renders a *big.Int the typed side holds as its decimal digits,
// the value String(b) and a `${b}` template produce. It is the typed-side companion
// of the boxed ToString: a bigint's string form is just its digits, with no suffix.
func BigIntToString(b *big.Int) BStr {
	return FromGoString(b.String())
}

// BigIntToStringRadix renders a bigint in the given base, the lowering of
// b.toString(radix). The radix runs through ToIntegerOrInfinity and must land in
// [2, 36], else it throws the RangeError JavaScript raises. big.Int.Text uses the
// same 0-9a-z digits JavaScript does for those bases, and it keeps the leading
// minus, so (-255n).toString(16) is "-ff" the same as V8.
func BigIntToStringRadix(b *big.Int, radix float64) BStr {
	r := math.Trunc(radix)
	if math.IsNaN(radix) {
		r = 0
	}
	if r < 2 || r > 36 {
		Throw(NewRangeError(FromGoString("toString() radix must be between 2 and 36")))
	}
	return FromGoString(b.Text(int(r)))
}

// BigIntToConsole renders a *big.Int the way console.log inspects a bigint: the
// decimal digits with a trailing "n", so console.log(10n) prints "10n" while
// String(10n) and `${10n}` stay "10". Only the console inspector adds the suffix,
// which is why it is its own helper and not BigIntToString.
func BigIntToConsole(b *big.Int) BStr {
	return FromGoString(b.String() + "n")
}

// maxBigIntBits caps how large a bigint an operator may build, the same order of
// bound V8 applies (2^30 bits, 128 MiB of magnitude). A shift or exponent that
// would clear it throws a RangeError instead of exhausting memory, the "Maximum
// BigInt size exceeded" JavaScript raises.
const maxBigIntBits = 1 << 30

// BigIntMustParse parses the decimal digits of a wide bigint literal, the form
// the lowering emits as a package-level var so a literal past int64 is parsed
// once at init and reused (05_type_lowering section 4). The digits come from the
// compiler, which already normalized radix prefixes and separators away, so a
// parse failure is a lowering bug and panics rather than throwing.
func BigIntMustParse(s string) *big.Int {
	i, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("value.BigIntMustParse: the compiler emitted a malformed bigint literal: " + s)
	}
	return i
}

// NumberToBigInt converts a number to a bigint, the lowering of BigInt(n). Only a
// finite integral number converts; a fractional value, NaN, or an infinity throws
// the RangeError JavaScript raises, because a bigint has no way to hold it. The
// conversion goes through big.Float so an integral number past int64 (1e21) still
// converts exactly.
func NumberToBigInt(f float64) *big.Int {
	if math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
		Throw(NewRangeError(FromGoString("The number " + NumberToString(f).ToGoString() + " cannot be converted to a BigInt because it is not an integer")))
	}
	i, _ := new(big.Float).SetFloat64(f).Int(nil)
	return i
}

// StringToBigInt converts a string to a bigint, the lowering of BigInt(s). It
// implements the ECMAScript StringToBigInt grammar: whitespace trims away, the
// empty remainder is 0n, a decimal form may carry one sign, and the 0x, 0o, and 0b
// radix prefixes are read unsigned. Anything else, a trailing character, a digit
// separator, or a sign on a prefixed form, throws the SyntaxError JavaScript
// raises, since unlike Number(s) there is no NaN to fall back to.
func StringToBigInt(s BStr) *big.Int {
	i, ok := parseBigInt(s)
	if !ok {
		Throw(NewSyntaxError(FromGoString("Cannot convert " + s.Trim().ToGoString() + " to a BigInt")))
	}
	return i
}

// parseBigInt reads the ECMAScript StringToBigInt grammar and reports whether the
// string is a bigint at all: whitespace trims away, the empty remainder is 0n, a
// decimal form may carry one sign, and the 0x, 0o, and 0b radix prefixes are read
// unsigned. It returns ok=false for anything else, a trailing character, a digit
// separator, or a sign on a prefixed form. StringToBigInt turns that into the
// SyntaxError the BigInt(s) coercion raises; loose equality reads it as "not a
// bigint" and compares unequal, since == has no error to raise.
func parseBigInt(s BStr) (*big.Int, bool) {
	text := s.Trim().ToGoString()
	if text == "" {
		return new(big.Int), true
	}
	base := 10
	digits := text
	if len(text) >= 2 && text[0] == '0' {
		switch text[1] {
		case 'x', 'X':
			base, digits = 16, text[2:]
		case 'o', 'O':
			base, digits = 8, text[2:]
		case 'b', 'B':
			base, digits = 2, text[2:]
		}
	}
	// A prefixed form takes no sign, and big.Int.SetString would accept one, so
	// reject it here. SetString with an explicit base already rejects a digit
	// separator, an empty digit run, and a stray character.
	if base != 10 && (digits == "" || digits[0] == '+' || digits[0] == '-') {
		return nil, false
	}
	return new(big.Int).SetString(digits, base)
}

// BoolToBigInt converts a boolean to a bigint, the lowering of BigInt(b): true is
// 1n and false is 0n.
func BoolToBigInt(b bool) *big.Int {
	if b {
		return big.NewInt(1)
	}
	return new(big.Int)
}

// BigIntToNumber converts a bigint to a number, the lowering of Number(b). The
// conversion rounds to the nearest float64 the way JavaScript does, so a bigint
// past 2^53 loses its low bits and a bigint past the float64 range becomes an
// infinity; big.Float's round-to-nearest-even is exactly that rounding.
func BigIntToNumber(b *big.Int) float64 {
	f, _ := new(big.Float).SetInt(b).Float64()
	return f
}

// BigIntToBool is the truthiness of a bigint, the lowering of Boolean(b) and a
// bigint in condition position: only 0n is false.
func BigIntToBool(b *big.Int) bool {
	return b.Sign() != 0
}

// BigIntPow computes x ** y on bigints. A negative exponent throws the RangeError
// JavaScript raises, since a bigint cannot hold a fraction, and an exponent that
// would build a result past the size cap throws the size RangeError rather than
// exhaust memory. The |x| <= 1 bases are exempt from the cap because their powers
// never grow.
func BigIntPow(x, y *big.Int) *big.Int {
	if y.Sign() < 0 {
		Throw(NewRangeError(FromGoString("Exponent must be non-negative")))
	}
	if x.BitLen() > 1 && (!y.IsInt64() || y.Int64()*int64(x.BitLen()) > maxBigIntBits) {
		Throw(NewRangeError(FromGoString("Maximum BigInt size exceeded")))
	}
	return new(big.Int).Exp(x, y, nil)
}

// BigIntDiv computes x / y on bigints, the quotient truncated toward zero that
// BigInt division takes. A zero divisor throws the RangeError JavaScript raises
// rather than letting big.Int.Quo panic, so the error is catchable in a try the way
// the language means it: 1n / 0n throws, it does not crash the program.
func BigIntDiv(x, y *big.Int) *big.Int {
	if y.Sign() == 0 {
		Throw(NewRangeError(FromGoString("Division by zero")))
	}
	return new(big.Int).Quo(x, y)
}

// BigIntRem computes x % y on bigints, the remainder that keeps the sign of the
// dividend. A zero divisor throws the same RangeError division does, since the
// remainder is undefined there and big.Int.Rem would otherwise panic.
func BigIntRem(x, y *big.Int) *big.Int {
	if y.Sign() == 0 {
		Throw(NewRangeError(FromGoString("Division by zero")))
	}
	return new(big.Int).Rem(x, y)
}

// bigIntWidth reads the bit width the asIntN and asUintN statics take as their
// first argument. The width is a number, and JavaScript runs it through ToIndex: a
// fractional value truncates toward zero, NaN reads as 0, and a negative,
// infinite, or past-2^53 value throws the RangeError ToIndex raises. A width past
// the size cap would build a modulus too large to hold, so it throws the same size
// RangeError the shift and exponent operators do rather than exhaust memory.
func bigIntWidth(bits float64) int {
	if math.IsNaN(bits) {
		return 0
	}
	w := math.Trunc(bits)
	if w < 0 || math.IsInf(w, 0) || w > 1<<53-1 {
		Throw(NewRangeError(FromGoString("Invalid value: not (convertible to) a safe integer")))
	}
	if w > maxBigIntBits {
		Throw(NewRangeError(FromGoString("Maximum BigInt size exceeded")))
	}
	return int(w)
}

// BigIntAsUintN wraps a bigint to the unsigned integer of the given bit width, the
// lowering of BigInt.asUintN(bits, x). The result is x modulo 2^bits, taken as the
// Euclidean remainder so it lands in [0, 2^bits) whatever the sign of x. A width of
// zero wraps everything to 0n, and a non-negative x that already fits the width
// passes through without building the modulus.
func BigIntAsUintN(bits float64, x *big.Int) *big.Int {
	n := bigIntWidth(bits)
	if n == 0 {
		return new(big.Int)
	}
	if x.Sign() >= 0 && x.BitLen() <= n {
		return new(big.Int).Set(x)
	}
	mod := new(big.Int).Lsh(big.NewInt(1), uint(n))
	return new(big.Int).Mod(x, mod)
}

// BigIntAsIntN wraps a bigint to the signed two's-complement integer of the given
// bit width, the lowering of BigInt.asIntN(bits, x). It reads the unsigned wrap
// first, then folds the top half of the range down by one modulus so the result
// lands in [-2^(bits-1), 2^(bits-1)); asIntN(8, 255n) is -1n. A width of zero wraps
// everything to 0n.
func BigIntAsIntN(bits float64, x *big.Int) *big.Int {
	n := bigIntWidth(bits)
	if n == 0 {
		return new(big.Int)
	}
	u := BigIntAsUintN(bits, x)
	half := new(big.Int).Lsh(big.NewInt(1), uint(n-1))
	if u.Cmp(half) >= 0 {
		u.Sub(u, new(big.Int).Lsh(big.NewInt(1), uint(n)))
	}
	return u
}

// BigIntLsh computes x << n on bigints. A negative count shifts the other way,
// the JavaScript rule that makes x << -1n mean x >> 1n, and a shift that would
// build a result past the size cap throws the size RangeError.
func BigIntLsh(x, n *big.Int) *big.Int {
	if n.Sign() < 0 {
		return bigRshMag(x, new(big.Int).Neg(n))
	}
	return bigLshMag(x, n)
}

// BigIntRsh computes x >> n on bigints, the arithmetic (floor) shift JavaScript
// defines, so -7n >> 1n is -4n. A negative count shifts the other way.
func BigIntRsh(x, n *big.Int) *big.Int {
	if n.Sign() < 0 {
		return bigLshMag(x, new(big.Int).Neg(n))
	}
	return bigRshMag(x, n)
}

// bigLshMag is the left shift by a non-negative count. Zero shifts to zero no
// matter the count; any other value is capped so the result stays under
// maxBigIntBits.
func bigLshMag(x, n *big.Int) *big.Int {
	if x.Sign() == 0 {
		return new(big.Int)
	}
	if !n.IsInt64() || n.Int64()+int64(x.BitLen()) > maxBigIntBits {
		Throw(NewRangeError(FromGoString("Maximum BigInt size exceeded")))
	}
	return new(big.Int).Lsh(x, uint(n.Int64()))
}

// bigRshMag is the arithmetic right shift by a non-negative count. A count past
// every bit of x needs no big shift at all: the floor result is 0 for a
// non-negative x and -1 for a negative one.
func bigRshMag(x, n *big.Int) *big.Int {
	if !n.IsInt64() || n.Int64() > int64(x.BitLen()) {
		if x.Sign() < 0 {
			return big.NewInt(-1)
		}
		return new(big.Int)
	}
	return new(big.Int).Rsh(x, uint(n.Int64()))
}
