package value

import "testing"

// TestBigInt64ArrayReadWrite proves the signed bigint array stores and reads back a
// value, wraps a write past the signed 64-bit range to two's-complement, and drops
// a fractional or out-of-range write.
func TestBigInt64ArrayReadWrite(t *testing.T) {
	a := NewBigInt64Array(3)
	a.SetAt(0, bi("42"))
	a.SetAt(1, bi("-7"))
	a.SetAt(2, bi("9223372036854775808")) // 2^63 wraps to -2^63
	a.SetAt(1.5, bi("99"))                // non-canonical, dropped
	a.SetAt(3, bi("99"))                  // out of range, dropped
	want := []string{"42", "-7", "-9223372036854775808"}
	for i, w := range want {
		if got := a.At(float64(i)).String(); got != w {
			t.Errorf("At(%d) = %s, want %s", i, got, w)
		}
	}
	if got := a.Len(); got != 3 {
		t.Errorf("Len() = %v, want 3", got)
	}
}

// TestBigUint64ArrayWrapsUnsigned proves the unsigned bigint array wraps a negative
// write up into the unsigned range and holds the full 64-bit magnitude a signed
// int64 could not.
func TestBigUint64ArrayWrapsUnsigned(t *testing.T) {
	a := NewBigUint64Array(2)
	a.SetAt(0, bi("-1"))                   // wraps to 2^64-1
	a.SetAt(1, bi("18446744073709551615")) // 2^64-1 stays
	if got := a.At(0).String(); got != "18446744073709551615" {
		t.Errorf("At(0) = %s, want 18446744073709551615", got)
	}
	if got := a.At(1).String(); got != "18446744073709551615" {
		t.Errorf("At(1) = %s, want 18446744073709551615", got)
	}
}

// TestBigIntArrayFromList proves the Of constructor fills a fresh buffer from a list
// of bigints, coercing each by the element kind's store rule.
func TestBigIntArrayFromList(t *testing.T) {
	a := BigInt64ArrayOf(bi("1"), bi("2"), bi("-3"))
	want := []string{"1", "2", "-3"}
	for i, w := range want {
		if got := a.At(float64(i)).String(); got != w {
			t.Errorf("At(%d) = %s, want %s", i, got, w)
		}
	}
}

// TestBigIntArraySharedViewsObserveWrites proves two bigint views over one buffer
// alias the same eight-byte elements: a signed -1n written through the BigInt64Array
// reads as 2^64-1 through the BigUint64Array over the same buffer.
func TestBigIntArraySharedViewsObserveWrites(t *testing.T) {
	buf := NewArrayBuffer(16)
	s := BigInt64ArrayView(buf, 0)
	u := BigUint64ArrayView(buf, 0)
	if got := s.Len(); got != 2 {
		t.Fatalf("signed view Len() = %v, want 2", got)
	}
	s.SetAt(0, bi("-1"))
	if got := u.At(0).String(); got != "18446744073709551615" {
		t.Errorf("unsigned view reads %s through the shared buffer, want 18446744073709551615", got)
	}
	u.SetAt(1, bi("9223372036854775808")) // 2^63 through unsigned reads negative through signed
	if got := s.At(1).String(); got != "-9223372036854775808" {
		t.Errorf("signed view reads %s through the shared buffer, want -9223372036854775808", got)
	}
}

// TestBigIntArrayGeometryGetters proves the view geometry getters report the byte
// offset, span, element width, and shared buffer a bigint view holds.
func TestBigIntArrayGeometryGetters(t *testing.T) {
	buf := NewArrayBuffer(32)
	a := BigInt64ArrayView(buf, 8, 2)
	if got := a.ByteOffset(); got != 8 {
		t.Errorf("ByteOffset() = %v, want 8", got)
	}
	if got := a.ByteLength(); got != 16 {
		t.Errorf("ByteLength() = %v, want 16", got)
	}
	if got := a.BytesPerElement(); got != 8 {
		t.Errorf("BytesPerElement() = %v, want 8", got)
	}
	if a.Buffer() != buf {
		t.Errorf("Buffer() did not return the aliased buffer")
	}
	if got := a.Buffer().ByteLength(); got != 32 {
		t.Errorf("Buffer().ByteLength() = %v, want 32", got)
	}
}

// TestBigIntArrayGetIndexBoxesUndefined proves the boxed read answers a boxed bigint
// for a canonical in-range index and undefined for an out-of-range or non-canonical
// one, the value a dynamic consumer sees.
func TestBigIntArrayGetIndexBoxesUndefined(t *testing.T) {
	a := BigInt64ArrayOf(bi("10"), bi("20"))
	if got := a.GetIndex(0); got.Kind() != KindBigInt || got.bigint().String() != "10" {
		t.Errorf("GetIndex(0) = %v, want the boxed bigint 10", got)
	}
	for _, i := range []float64{2, -1, 1.5} {
		if got := a.GetIndex(i); got.Kind() != KindUndefined {
			t.Errorf("GetIndex(%v) = %v, want undefined", i, got)
		}
	}
}
