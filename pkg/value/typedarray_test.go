package value

import "testing"

// TestTypedArrayZeroedLength proves each numeric family member constructs from a
// length as that many zero elements read back as the Number 0.
func TestTypedArrayZeroedLength(t *testing.T) {
	cases := []struct {
		name string
		len  float64
	}{
		{"Int8", NewInt8Array(3).Len()},
		{"Uint8Clamped", NewUint8ClampedArray(3).Len()},
		{"Int16", NewInt16Array(3).Len()},
		{"Uint16", NewUint16Array(3).Len()},
		{"Int32", NewInt32Array(3).Len()},
		{"Uint32", NewUint32Array(3).Len()},
		{"Float32", NewFloat32Array(3).Len()},
		{"Float64", NewFloat64Array(3).Len()},
	}
	for _, c := range cases {
		if c.len != 3 {
			t.Errorf("New%sArray(3).Len = %v, want 3", c.name, c.len)
		}
	}
	a := NewInt32Array(2)
	if a.At(0) != 0 || a.At(1) != 0 {
		t.Errorf("fresh Int32Array elements = %v %v, want 0 0", a.At(0), a.At(1))
	}
}

// TestTypedArrayBadLengthClamps proves a negative or not-a-number length yields an
// empty array rather than panicking, the same covered-subset rule the byte buffer
// takes.
func TestTypedArrayBadLengthClamps(t *testing.T) {
	if got := NewFloat64Array(-5).Len(); got != 0 {
		t.Errorf("NewFloat64Array(-5).Len = %v, want 0", got)
	}
	if got := NewInt16Array(nanValue()).Len(); got != 0 {
		t.Errorf("NewInt16Array(NaN).Len = %v, want 0", got)
	}
}

// TestInt8ArrayWraps proves ToInt8: a value reduces modulo 256 and the top half of
// the range reads as negative, so 128 stores -128, 255 stores -1, and 3.9 stores 3.
func TestInt8ArrayWraps(t *testing.T) {
	a := Int8ArrayOf(0, 127, 128, 255, 256, -1, 3.9)
	want := []float64{0, 127, -128, -1, 0, -1, 3}
	assertElems(t, a.Len(), a.At, want)
}

// TestUint8ClampedArrayClamps proves ToUint8Clamp: below zero clamps to 0, above
// 255 clamps to 255, and a value between rounds half to even, so 0.5 stores 0 and
// 1.5 stores 2.
func TestUint8ClampedArrayClamps(t *testing.T) {
	a := Uint8ClampedArrayOf(-1, 0, 0.5, 1.5, 2.5, 127.6, 255, 256, 300)
	want := []float64{0, 0, 0, 2, 2, 128, 255, 255, 255}
	assertElems(t, a.Len(), a.At, want)
}

// TestInt16AndUint16Wrap proves ToInt16 and ToUint16 reduce modulo 65536, one into
// signed range and one into unsigned.
func TestInt16AndUint16Wrap(t *testing.T) {
	s := Int16ArrayOf(32767, 32768, 65535, 65536, -1)
	assertElems(t, s.Len(), s.At, []float64{32767, -32768, -1, 0, -1})
	u := Uint16ArrayOf(0, 65535, 65536, 65537, -1)
	assertElems(t, u.Len(), u.At, []float64{0, 65535, 0, 1, 65535})
}

// TestInt32AndUint32Wrap proves ToInt32 and ToUint32 reduce modulo 2^32, the width
// the bitwise operators and these arrays share.
func TestInt32AndUint32Wrap(t *testing.T) {
	s := Int32ArrayOf(2147483647, 2147483648, 4294967295, 4294967296, -1)
	assertElems(t, s.Len(), s.At, []float64{2147483647, -2147483648, -1, 0, -1})
	u := Uint32ArrayOf(0, 4294967295, 4294967296, 4294967297, -1)
	assertElems(t, u.Len(), u.At, []float64{0, 4294967295, 0, 1, 4294967295})
}

// TestFloat32ArrayRoundsToSingle proves a Float32Array store rounds to single
// precision, so a value with no exact float32 shows the rounding when read back,
// while a Float64Array keeps the Number exactly.
func TestFloat32ArrayRoundsToSingle(t *testing.T) {
	f32 := NewFloat32Array(1)
	f32.SetAt(0, 0.1)
	if f32.At(0) == 0.1 {
		t.Errorf("Float32Array stored 0.1 exactly, want single-precision rounding")
	}
	if got := f32.At(0); got < 0.09999 || got > 0.10001 {
		t.Errorf("Float32Array 0.1 read back = %v, want near 0.1", got)
	}
	f64 := NewFloat64Array(1)
	f64.SetAt(0, 0.1)
	if f64.At(0) != 0.1 {
		t.Errorf("Float64Array 0.1 read back = %v, want 0.1 exactly", f64.At(0))
	}
}

// TestTypedArraySetAtCoercesAndBounds proves a write coerces with the element
// kind's store rule and a write past the end is dropped rather than growing the
// array, the same out-of-range rule the byte buffer takes.
func TestTypedArraySetAtCoercesAndBounds(t *testing.T) {
	a := NewInt8Array(2)
	a.SetAt(0, 300) // 300 mod 256 = 44
	a.SetAt(1, -1)  // reads back as -1
	a.SetAt(5, 7)   // past the end, dropped
	assertElems(t, a.Len(), a.At, []float64{44, -1})
}

// TestTypedArrayReadOutOfRange proves an index outside the array reads as 0, the
// covered-subset behavior the byte buffer documents.
func TestTypedArrayReadOutOfRange(t *testing.T) {
	a := Int32ArrayOf(10, 20)
	if got := a.At(5); got != 0 {
		t.Errorf("At(5) on a length-2 array = %v, want 0", got)
	}
	if got := a.At(-1); got != 0 {
		t.Errorf("At(-1) = %v, want 0", got)
	}
}

// assertElems checks a typed array's length and every element against a want list
// read back through At, the shared body of the family's element assertions.
func assertElems(t *testing.T, gotLen float64, at func(float64) float64, want []float64) {
	t.Helper()
	if gotLen != float64(len(want)) {
		t.Fatalf("Len = %v, want %d", gotLen, len(want))
	}
	for i, w := range want {
		if got := at(float64(i)); got != w {
			t.Errorf("At(%d) = %v, want %v", i, got, w)
		}
	}
}
