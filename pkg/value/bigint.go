package value

import (
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

// Int is the arbitrary-precision value, for a caller that needs the underlying
// math/big.Int, such as the bridge marshaling a bigint back to a Go integer.
func (b *BigInt) Int() *big.Int { return &b.i }

// String renders a bigint as its decimal digits with no suffix, the value String(b)
// and a `${b}` template produce: a bigint's ToString is just its digits, the "n"
// suffix belongs only to how console.log inspects a value, not to string coercion.
func (b *BigInt) String() string { return b.i.String() }

// IsZero reports whether the bigint is 0n, the falsy bigint.
func (b *BigInt) IsZero() bool { return b.i.Sign() == 0 }
