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

// TestTypedArrayViewsABuffer proves a typed array is a view over an ArrayBuffer:
// its backing buffer is sized to hold the elements, and a write through the view
// reaches the buffer's own bytes in little-endian order rather than a private copy.
func TestTypedArrayViewsABuffer(t *testing.T) {
	a := NewInt32Array(4)
	if a.buffer == nil {
		t.Fatal("Int32Array has no backing buffer")
	}
	if got := a.buffer.ByteLength(); got != 16 {
		t.Errorf("Int32Array(4) buffer byte length = %v, want 16", got)
	}
	a.SetAt(0, 1)
	bytes := a.buffer.Bytes()
	if bytes[0] != 1 || bytes[1] != 0 || bytes[2] != 0 || bytes[3] != 0 {
		t.Errorf("view write did not reach the buffer bytes little-endian: %v", bytes[:4])
	}
}

// TestTypedArrayViewOverBuffer proves the ArrayBuffer overload: a view takes its
// length from the buffer when omitted, honors a byte offset, and takes an explicit
// length when given. Two views over one buffer observe each other's writes because
// they alias the same bytes.
func TestTypedArrayViewOverBuffer(t *testing.T) {
	buf := NewArrayBuffer(16)
	whole := Int32ArrayView(buf, 0)
	if whole.Len() != 4 {
		t.Errorf("Int32ArrayView(buf, 0).Len = %v, want 4", whole.Len())
	}
	offset := Int32ArrayView(buf, 8)
	if offset.Len() != 2 {
		t.Errorf("Int32ArrayView(buf, 8).Len = %v, want 2", offset.Len())
	}
	capped := Int32ArrayView(buf, 4, 2)
	if capped.Len() != 2 {
		t.Errorf("Int32ArrayView(buf, 4, 2).Len = %v, want 2", capped.Len())
	}
	// The offset view starts at byte 8, which is element 2 of the whole view.
	whole.SetAt(2, 1234)
	if got := offset.At(0); got != 1234 {
		t.Errorf("offset view did not observe the whole view's write: %v, want 1234", got)
	}
}

// TestTypedArrayGeometryGetters proves the instance getters read off the view:
// Buffer is the aliased backing store, ByteOffset the byte the view starts at, and
// ByteLength its span in bytes, the element count times the element width. A view
// at a byte offset over a shared buffer reports that offset and its own span, not
// the whole buffer's.
func TestTypedArrayGeometryGetters(t *testing.T) {
	buf := NewArrayBuffer(16)
	view := Int32ArrayView(buf, 4, 2)
	if view.Buffer() != buf {
		t.Error("view.Buffer did not return the shared backing buffer")
	}
	if got := view.ByteOffset(); got != 4 {
		t.Errorf("view.ByteOffset = %v, want 4", got)
	}
	if got := view.ByteLength(); got != 8 {
		t.Errorf("view.ByteLength = %v, want 8", got)
	}
	if got := view.Buffer().ByteLength(); got != 16 {
		t.Errorf("view.Buffer().ByteLength = %v, want 16", got)
	}
	if got := view.Len(); got != 2 {
		t.Errorf("view.Len = %v, want 2", got)
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
