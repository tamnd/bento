package value

import (
	"math"
	"math/big"
	"testing"
)

// TestBigIntFromInt64String proves an int64-built bigint renders as its decimal
// digits with no suffix, so String(b) and a template read the bare number.
func TestBigIntFromInt64String(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{9007199254740993, "9007199254740993"}, // past Number.MAX_SAFE_INTEGER, exact
		{-9223372036854775808, "-9223372036854775808"},
	} {
		v := BigIntFromInt64(tc.n)
		if v.Kind() != KindBigInt {
			t.Fatalf("BigIntFromInt64(%d) kind = %v, want KindBigInt", tc.n, v.Kind())
		}
		if got := v.bigint().String(); got != tc.want {
			t.Errorf("BigIntFromInt64(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// TestBigIntFromStringExact proves a decimal string past the 64-bit range keeps
// every digit, the whole point of an arbitrary-precision integer.
func TestBigIntFromStringExact(t *testing.T) {
	const big = "123456789012345678901234567890"
	v, ok := BigIntFromString(big)
	if !ok {
		t.Fatalf("BigIntFromString(%q) reported not ok", big)
	}
	if got := v.bigint().String(); got != big {
		t.Errorf("BigIntFromString(%q) = %q, want the same digits", big, got)
	}
}

// TestBigIntFromStringRejectsNonInteger proves a non-integer string is refused, so
// the lowerer reports a SyntaxError rather than boxing a bad value.
func TestBigIntFromStringRejectsNonInteger(t *testing.T) {
	for _, s := range []string{"", "1.5", "0x10", "12n", "abc", " 3"} {
		if _, ok := BigIntFromString(s); ok {
			t.Errorf("BigIntFromString(%q) accepted a non-integer", s)
		}
	}
}

// TestBigIntToString proves ToString on a bigint is its digits, no "n" suffix,
// matching JavaScript String(10n) === "10".
func TestBigIntToString(t *testing.T) {
	if got := ToString(BigIntFromInt64(10)).ToGoString(); got != "10" {
		t.Errorf("ToString(10n) = %q, want \"10\"", got)
	}
}

// TestBigIntToBoolean proves 0n is the only falsy bigint, matching JavaScript.
func TestBigIntToBoolean(t *testing.T) {
	if ToBoolean(BigIntFromInt64(0)) {
		t.Error("ToBoolean(0n) = true, want false")
	}
	for _, n := range []int64{1, -1, 42} {
		if !ToBoolean(BigIntFromInt64(n)) {
			t.Errorf("ToBoolean(%dn) = false, want true", n)
		}
	}
}

// TestBigIntAddBigInt proves bigint + bigint adds and stays a bigint, exact past
// the safe-integer range.
func TestBigIntAddBigInt(t *testing.T) {
	a, _ := BigIntFromString("9007199254740993")
	b := BigIntFromInt64(2)
	sum := Add(a, b)
	if sum.Kind() != KindBigInt {
		t.Fatalf("bigint + bigint kind = %v, want KindBigInt", sum.Kind())
	}
	if got := sum.bigint().String(); got != "9007199254740995" {
		t.Errorf("9007199254740993n + 2n = %q, want 9007199254740995", got)
	}
}

// TestBigIntAddString proves bigint + string concatenates through the bigint's
// digit form, matching 10n + "x" === "10x".
func TestBigIntAddString(t *testing.T) {
	got := Add(BigIntFromInt64(10), StringValue(FromGoString("x")))
	if got.Kind() != KindString {
		t.Fatalf("bigint + string kind = %v, want KindString", got.Kind())
	}
	if s := got.str().ToGoString(); s != "10x" {
		t.Errorf("10n + \"x\" = %q, want \"10x\"", s)
	}
}

// TestBigIntAddNumberThrows proves mixing a bigint and a number in + is a
// TypeError, never a silent narrowing, matching 1n + 1 throwing.
func TestBigIntAddNumberThrows(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("1n + 1 did not throw")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("1n + 1 threw %T, want *Error", r)
		}
		if e.Name().ToGoString() != "TypeError" {
			t.Errorf("1n + 1 threw %q, want TypeError", e.Name().ToGoString())
		}
	}()
	Add(BigIntFromInt64(1), Number(1))
}

// TestBigIntToNumberThrows proves the arithmetic ToNumber coercion throws on a
// bigint rather than looping or silently converting, matching the abstract op.
func TestBigIntToNumberThrows(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("ToNumber(bigint) did not throw")
		}
	}()
	ToNumber(BigIntFromInt64(3))
}

// bigThrown runs fn, which must throw, and returns the *Error it threw. It is
// the harness for the conversion and operator helpers whose JavaScript behavior
// is a RangeError or SyntaxError on a bad input.
func bigThrown(t *testing.T, what string, fn func()) *Error {
	t.Helper()
	var caught *Error
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			e, ok := r.(*Error)
			if !ok {
				t.Fatalf("%s threw %T, want *Error", what, r)
			}
			caught = e
		}()
		fn()
	}()
	if caught == nil {
		t.Fatalf("%s did not throw", what)
	}
	return caught
}

// TestNumberToBigIntExact proves an integral number converts exactly, including
// an integer past int64 where the conversion must go through the full float64
// magnitude rather than a truncating cast.
func TestNumberToBigIntExact(t *testing.T) {
	for _, tc := range []struct {
		f    float64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{9007199254740992, "9007199254740992"}, // 2^53
		{1e21, "1000000000000000000000"},
		{-1e21, "-1000000000000000000000"},
	} {
		if got := NumberToBigInt(tc.f).String(); got != tc.want {
			t.Errorf("NumberToBigInt(%v) = %s, want %s", tc.f, got, tc.want)
		}
	}
}

// TestNumberToBigIntThrows proves the non-integral numbers throw the RangeError
// JavaScript raises, since a bigint has no way to hold them.
func TestNumberToBigIntThrows(t *testing.T) {
	for _, f := range []float64{1.5, -0.25, math.NaN(), math.Inf(1), math.Inf(-1)} {
		e := bigThrown(t, "NumberToBigInt", func() { NumberToBigInt(f) })
		if e.Name().ToGoString() != "RangeError" {
			t.Errorf("NumberToBigInt(%v) threw %s, want RangeError", f, e.Name().ToGoString())
		}
	}
}

// TestStringToBigIntGrammar proves the ECMAScript StringToBigInt grammar: trimmed
// whitespace, the empty remainder as 0n, one sign on a decimal form, and the
// unsigned radix prefixes.
func TestStringToBigIntGrammar(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want string
	}{
		{"", "0"},
		{"   ", "0"},
		{"  42  ", "42"},
		{"-7", "-7"},
		{"+8", "8"},
		{"0x10", "16"},
		{"0X1f", "31"},
		{"0o17", "15"},
		{"0b101", "5"},
		{"123456789012345678901234567890", "123456789012345678901234567890"},
	} {
		if got := StringToBigInt(FromGoString(tc.s)).String(); got != tc.want {
			t.Errorf("StringToBigInt(%q) = %s, want %s", tc.s, got, tc.want)
		}
	}
}

// TestStringToBigIntRejects proves everything outside the grammar throws the
// SyntaxError JavaScript raises: a fraction, an exponent, a digit separator, a
// signed or empty radix form, a stray character. Unlike Number(s) there is no NaN
// to fall back to.
func TestStringToBigIntRejects(t *testing.T) {
	for _, s := range []string{"nope", "1.5", "1e3", "1_000", "0x", "-0x10", "0x-1", "12n", "1 2", "Infinity"} {
		e := bigThrown(t, "StringToBigInt", func() { StringToBigInt(FromGoString(s)) })
		if e.Name().ToGoString() != "SyntaxError" {
			t.Errorf("StringToBigInt(%q) threw %s, want SyntaxError", s, e.Name().ToGoString())
		}
	}
}

// TestBoolToBigInt proves the boolean conversion: true is 1n, false is 0n.
func TestBoolToBigInt(t *testing.T) {
	if got := BoolToBigInt(true).String(); got != "1" {
		t.Errorf("BoolToBigInt(true) = %s, want 1", got)
	}
	if got := BoolToBigInt(false).String(); got != "0" {
		t.Errorf("BoolToBigInt(false) = %s, want 0", got)
	}
}

// TestBigIntToNumberRounds proves Number(b) rounds to the nearest float64 the way
// JavaScript does: 2^53+1 ties to even and loses its low bit, and a magnitude past
// the float64 range becomes an infinity.
func TestBigIntToNumberRounds(t *testing.T) {
	odd, _ := new(big.Int).SetString("9007199254740993", 10) // 2^53 + 1
	if got := BigIntToNumber(odd); got != 9007199254740992 {
		t.Errorf("Number(2^53+1) = %v, want 9007199254740992", got)
	}
	if got := BigIntToNumber(big.NewInt(-7)); got != -7 {
		t.Errorf("Number(-7n) = %v, want -7", got)
	}
	huge := new(big.Int).Exp(big.NewInt(10), big.NewInt(400), nil)
	if got := BigIntToNumber(huge); !math.IsInf(got, 1) {
		t.Errorf("Number(10^400) = %v, want +Inf", got)
	}
	if got := BigIntToNumber(new(big.Int).Neg(huge)); !math.IsInf(got, -1) {
		t.Errorf("Number(-10^400) = %v, want -Inf", got)
	}
}

// TestBigIntToBool proves the typed-side truthiness: only 0n is false.
func TestBigIntToBool(t *testing.T) {
	if BigIntToBool(new(big.Int)) {
		t.Error("Boolean(0n) = true, want false")
	}
	if !BigIntToBool(big.NewInt(-1)) {
		t.Error("Boolean(-1n) = false, want true")
	}
}

// TestBigIntPow proves ** on bigints: the plain powers, the 0n ** 0n === 1n edge,
// a negative base, and the two throws, a negative exponent and a result past the
// size cap, both RangeErrors and the second raised before any giant allocation.
func TestBigIntPow(t *testing.T) {
	for _, tc := range []struct {
		x, y int64
		want string
	}{
		{2, 10, "1024"},
		{7, 0, "1"},
		{0, 0, "1"},
		{-2, 3, "-8"},
		{1, 1 << 30, "1"}, // |x| <= 1 is exempt from the size cap
	} {
		if got := BigIntPow(big.NewInt(tc.x), big.NewInt(tc.y)).String(); got != tc.want {
			t.Errorf("%dn ** %dn = %s, want %s", tc.x, tc.y, got, tc.want)
		}
	}
	e := bigThrown(t, "2n ** -1n", func() { BigIntPow(big.NewInt(2), big.NewInt(-1)) })
	if e.Name().ToGoString() != "RangeError" || e.Message().ToGoString() != "Exponent must be non-negative" {
		t.Errorf("2n ** -1n threw %s: %s, want the negative-exponent RangeError", e.Name().ToGoString(), e.Message().ToGoString())
	}
	e = bigThrown(t, "2n ** 2^30n", func() { BigIntPow(big.NewInt(2), big.NewInt(1<<30)) })
	if e.Name().ToGoString() != "RangeError" || e.Message().ToGoString() != "Maximum BigInt size exceeded" {
		t.Errorf("2n ** 2^30n threw %s: %s, want the size-cap RangeError", e.Name().ToGoString(), e.Message().ToGoString())
	}
}

// TestBigIntShifts proves the JavaScript shift semantics: a negative count
// reverses direction, >> is the arithmetic floor shift so -7n >> 1n is -4n, a
// right shift past every bit floors to 0 or -1 by sign even when the count does
// not fit an int64, and a left shift that would clear the size cap throws before
// allocating.
func TestBigIntShifts(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  *big.Int
		want string
	}{
		{"1n << 10n", BigIntLsh(big.NewInt(1), big.NewInt(10)), "1024"},
		{"8n << -1n", BigIntLsh(big.NewInt(8), big.NewInt(-1)), "4"},
		{"0n << 2^40n", BigIntLsh(new(big.Int), big.NewInt(1<<40)), "0"},
		{"7n >> 1n", BigIntRsh(big.NewInt(7), big.NewInt(1)), "3"},
		{"-7n >> 1n", BigIntRsh(big.NewInt(-7), big.NewInt(1)), "-4"},
		{"5n >> 100n", BigIntRsh(big.NewInt(5), big.NewInt(100)), "0"},
		{"-1n >> 100n", BigIntRsh(big.NewInt(-1), big.NewInt(100)), "-1"},
		{"3n >> -2n", BigIntRsh(big.NewInt(3), big.NewInt(-2)), "12"},
	} {
		if tc.got.String() != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, tc.got.String(), tc.want)
		}
	}
	// A count too big for int64 still right-shifts to the floor values by sign.
	hugeCount := new(big.Int).Lsh(big.NewInt(1), 80)
	if got := BigIntRsh(big.NewInt(-9), hugeCount).String(); got != "-1" {
		t.Errorf("-9n >> 2^80n = %s, want -1", got)
	}
	if got := BigIntRsh(big.NewInt(9), hugeCount).String(); got != "0" {
		t.Errorf("9n >> 2^80n = %s, want 0", got)
	}
	e := bigThrown(t, "1n << 2^30n", func() { BigIntLsh(big.NewInt(1), big.NewInt(1<<30)) })
	if e.Name().ToGoString() != "RangeError" || e.Message().ToGoString() != "Maximum BigInt size exceeded" {
		t.Errorf("1n << 2^30n threw %s: %s, want the size-cap RangeError", e.Name().ToGoString(), e.Message().ToGoString())
	}
	e = bigThrown(t, "1n << 2^80n", func() { BigIntLsh(big.NewInt(1), hugeCount) })
	if e.Name().ToGoString() != "RangeError" {
		t.Errorf("1n << 2^80n threw %s, want RangeError", e.Name().ToGoString())
	}
}

// TestBigIntDivRem proves / and % on bigints: the quotient truncates toward zero and
// the remainder keeps the sign of the dividend, and a zero divisor throws a catchable
// RangeError rather than panicking the way big.Int.Quo and Rem do on their own.
func TestBigIntDivRem(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  *big.Int
		want string
	}{
		{"10n / 3n", BigIntDiv(big.NewInt(10), big.NewInt(3)), "3"},
		{"-10n / 3n", BigIntDiv(big.NewInt(-10), big.NewInt(3)), "-3"},
		{"10n % 3n", BigIntRem(big.NewInt(10), big.NewInt(3)), "1"},
		{"-7n % 3n", BigIntRem(big.NewInt(-7), big.NewInt(3)), "-1"},
	} {
		if tc.got.String() != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, tc.got.String(), tc.want)
		}
	}
	e := bigThrown(t, "1n / 0n", func() { BigIntDiv(big.NewInt(1), new(big.Int)) })
	if e.Name().ToGoString() != "RangeError" || e.Message().ToGoString() != "Division by zero" {
		t.Errorf("1n / 0n threw %s: %s, want the divide-by-zero RangeError", e.Name().ToGoString(), e.Message().ToGoString())
	}
	e = bigThrown(t, "1n % 0n", func() { BigIntRem(big.NewInt(1), new(big.Int)) })
	if e.Name().ToGoString() != "RangeError" || e.Message().ToGoString() != "Division by zero" {
		t.Errorf("1n %% 0n threw %s: %s, want the divide-by-zero RangeError", e.Name().ToGoString(), e.Message().ToGoString())
	}
}

// bi parses a decimal string as a *big.Int for the wrap tests, panicking on a bad
// literal since the test author writes them.
func bi(s string) *big.Int {
	i, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bi: bad literal " + s)
	}
	return i
}

// TestBigIntAsUintN proves the unsigned wrap lands in [0, 2^bits): a value that
// fits passes through, a value past the width wraps down by the modulus, and a
// negative value wraps up into range, so asUintN(16, -1n) is 65535n.
func TestBigIntAsUintN(t *testing.T) {
	for _, tc := range []struct {
		bits float64
		x    string
		want string
	}{
		{16, "0", "0"},
		{16, "65535", "65535"},
		{16, "65536", "0"},
		{16, "70000", "4464"},
		{16, "-1", "65535"},
		{8, "255", "255"},
		{8, "256", "0"},
		{0, "123", "0"},
		{4, "-1", "15"},
	} {
		if got := BigIntAsUintN(tc.bits, bi(tc.x)); got.String() != tc.want {
			t.Errorf("BigIntAsUintN(%v, %s) = %s, want %s", tc.bits, tc.x, got, tc.want)
		}
	}
}

// TestBigIntAsIntN proves the signed wrap lands in [-2^(bits-1), 2^(bits-1)): the
// top half of the unsigned range folds to the negative half, so asIntN(8, 255n) is
// -1n and asIntN(8, 128n) is -128n.
func TestBigIntAsIntN(t *testing.T) {
	for _, tc := range []struct {
		bits float64
		x    string
		want string
	}{
		{8, "127", "127"},
		{8, "128", "-128"},
		{8, "255", "-1"},
		{8, "256", "0"},
		{8, "-1", "-1"},
		{16, "-1", "-1"},
		{0, "123", "0"},
		{1, "1", "-1"},
		{1, "0", "0"},
	} {
		if got := BigIntAsIntN(tc.bits, bi(tc.x)); got.String() != tc.want {
			t.Errorf("BigIntAsIntN(%v, %s) = %s, want %s", tc.bits, tc.x, got, tc.want)
		}
	}
}

// TestBigIntWidthTruncatesAndRejects proves the width runs through ToIndex: a
// fractional width truncates toward zero and a negative width throws a RangeError.
func TestBigIntWidthTruncatesAndRejects(t *testing.T) {
	if got := BigIntAsUintN(8.9, bi("255")); got.String() != "255" {
		t.Errorf("fractional width did not truncate: got %s, want 255", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a negative width should throw a RangeError")
		}
	}()
	BigIntAsUintN(-1, bi("1"))
}

// TestBigIntToStringRadix proves a bigint renders in the named base with the same
// lowercase digits and leading minus V8 uses, and that a radix outside [2, 36]
// throws a RangeError.
func TestBigIntToStringRadix(t *testing.T) {
	for _, tc := range []struct {
		x     string
		radix float64
		want  string
	}{
		{"255", 10, "255"},
		{"255", 16, "ff"},
		{"255", 2, "11111111"},
		{"-255", 16, "-ff"},
		{"35", 36, "z"},
		{"255", 16.9, "ff"}, // radix truncates toward zero
	} {
		if got := BigIntToStringRadix(bi(tc.x), tc.radix).ToGoString(); got != tc.want {
			t.Errorf("BigIntToStringRadix(%s, %v) = %q, want %q", tc.x, tc.radix, got, tc.want)
		}
	}
	for _, bad := range []float64{1, 0, 37, 100} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("radix %v should throw a RangeError", bad)
				}
			}()
			BigIntToStringRadix(bi("5"), bad)
		}()
	}
}
