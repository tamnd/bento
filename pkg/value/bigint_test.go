package value

import "testing"

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
