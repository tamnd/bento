package value

import (
	"encoding/binary"
	"math"
	"math/big"
)

// DataView is bento's runtime representation of a JavaScript DataView, the view
// that reads and writes an ArrayBuffer at arbitrary byte offsets with an explicit
// endianness, independent of any element alignment (25 §25.3). Unlike a typed
// array, which pins one element width and reads through a naturally aligned slice,
// a DataView carries no element type: every get and set names its own width and
// byte order at the call, so one view over a buffer can read a big-endian int32 at
// byte 1 and a little-endian float64 at byte 5 over the same bytes a typed array
// would see.
//
// Like every other view it is not the storage: it records the ArrayBuffer it reads,
// the byte offset it starts at, and its byte length, so it aliases the buffer's
// bytes and observes writes made through the buffer or any other view of it. The
// byte length is consulted against the buffer's live state through liveByteLength
// rather than frozen at construction, because the buffer can be detached or resized
// while the view points at it: a detached buffer, or a shrink that puts the view's
// range past the buffer's new end, turns the view out of bounds, which every access
// reports as the TypeError the spec throws.
type DataView struct {
	buffer         *ArrayBuffer
	byteOffset     int
	byteLength     int
	lengthTracking bool
}

// NewDataView builds a DataView over an existing ArrayBuffer, the lowering of new
// DataView(buffer, byteOffset, byteLength) and its shorter forms (25 §25.3.2). The
// byte offset defaults to zero when the call omits it and the byte length runs from
// the offset to the end of the buffer when omitted. The offset is a ToIndex value,
// so a negative or too-large offset is a RangeError; a detached buffer is a
// TypeError; and an offset or an explicit length that runs past the buffer is a
// RangeError, the throws the spec raises at construction.
func NewDataView(buffer *ArrayBuffer, byteOffset float64, byteLength ...float64) *DataView {
	off := toDataViewIndex(byteOffset)
	if buffer.detached {
		Throw(NewTypeError(FromGoString("Cannot construct a DataView over a detached ArrayBuffer")))
	}
	bufLen := len(buffer.data)
	if off > bufLen {
		Throw(NewRangeError(FromGoString("DataView byte offset is out of bounds")))
	}
	d := &DataView{buffer: buffer, byteOffset: off}
	if len(byteLength) > 0 {
		n := toDataViewIndex(byteLength[0])
		if off+n > bufLen {
			Throw(NewRangeError(FromGoString("DataView byte length is out of bounds")))
		}
		d.byteLength = n
	} else {
		d.byteLength = bufLen - off
		// A view with no explicit length over a resizable buffer tracks the buffer's
		// length, so a later resize changes the span it reports (25 §25.3.2). Over a
		// fixed buffer the length is frozen at the offset-to-end span computed above.
		if buffer.resizable {
			d.lengthTracking = true
		}
	}
	return d
}

// liveByteLength is the view's span in bytes as of this access and whether the view
// is out of bounds, the pair every getter and setter consults before it touches
// storage. A detached buffer, or an offset that a shrink has left past the buffer's
// new end, puts the view out of bounds; the spec's IsViewOutOfBounds then makes
// every access a TypeError, so the boolean carries that state up to the caller. A
// length-tracking view over a resizable buffer spans from its offset to the
// buffer's current end, so a resize changes the span it reports rather than putting
// it out of bounds. A fixed-length view whose range a shrink has dropped is out of
// bounds until a later grow restores it.
func (d *DataView) liveByteLength() (int, bool) {
	if d.buffer.detached {
		return 0, true
	}
	bufLen := len(d.buffer.data)
	if d.byteOffset > bufLen {
		return 0, true
	}
	if d.lengthTracking {
		return bufLen - d.byteOffset, false
	}
	if d.byteOffset+d.byteLength > bufLen {
		return 0, true
	}
	return d.byteLength, false
}

// Buffer is the ArrayBuffer the view aliases, the .buffer getter, the same backing
// store every other view of the buffer holds, so a comparison of two views' buffers
// by identity holds.
func (d *DataView) Buffer() *ArrayBuffer { return d.buffer }

// ByteOffset is the byte the view starts at within its buffer, the .byteOffset
// getter, a Number to match the property's type. An out-of-bounds view reports zero,
// the value the spec's getter returns once the view's range no longer fits.
func (d *DataView) ByteOffset() float64 {
	if _, oob := d.liveByteLength(); oob {
		return 0
	}
	return float64(d.byteOffset)
}

// ByteLength is the view's span in bytes, the .byteLength getter, a Number. It
// follows the buffer's live state: a length-tracking view reports its span over the
// buffer's current size, and an out-of-bounds view reports zero.
func (d *DataView) ByteLength() float64 {
	n, oob := d.liveByteLength()
	if oob {
		return 0
	}
	return float64(n)
}

// access resolves a get or set to the buffer index it reads or writes, running the
// three checks the spec's GetViewValue and SetViewValue share (25 §25.3.1.1 and
// §25.3.1.2). The request index is a ToIndex value, so a negative or too-large one is
// a RangeError. A detached buffer, or a resize that has left the view out of bounds,
// is a TypeError. An access whose element would run past the view's live end is a
// RangeError. On success it returns the byte index within the buffer, the view's
// offset plus the request index, at which the element's bytes lie.
func (d *DataView) access(requestIndex float64, elementSize int) int {
	getIndex := toDataViewIndex(requestIndex)
	length, oob := d.liveByteLength()
	if oob {
		Throw(NewTypeError(FromGoString("Cannot access a DataView whose buffer is detached or out of bounds")))
	}
	if getIndex+elementSize > length {
		Throw(NewRangeError(FromGoString("DataView access is out of bounds")))
	}
	return d.byteOffset + getIndex
}

// dataViewOrder maps the optional littleEndian flag a get or set carries to the byte
// order it reads or writes with. The flag defaults to false, so an omitted argument
// reads big-endian, the network byte order the spec makes the default; a true flag
// reads little-endian. The single-byte accessors take no flag and never call this.
func dataViewOrder(littleEndian []bool) binary.ByteOrder {
	if len(littleEndian) > 0 && littleEndian[0] {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// GetInt8 reads the signed byte at the offset, DataView.prototype.getInt8. It is a
// single byte, so it carries no endianness, and it widens the stored int8 to the
// Number the read hands out.
func (d *DataView) GetInt8(byteOffset float64) float64 {
	bi := d.access(byteOffset, 1)
	return float64(int8(d.buffer.data[bi]))
}

// GetUint8 reads the unsigned byte at the offset, DataView.prototype.getUint8, the
// unsigned sibling of GetInt8.
func (d *DataView) GetUint8(byteOffset float64) float64 {
	bi := d.access(byteOffset, 1)
	return float64(d.buffer.data[bi])
}

// GetInt16 reads the signed 16-bit integer at the offset with the given endianness,
// DataView.prototype.getInt16, widening the result to a Number.
func (d *DataView) GetInt16(byteOffset float64, littleEndian ...bool) float64 {
	bi := d.access(byteOffset, 2)
	return float64(int16(dataViewOrder(littleEndian).Uint16(d.buffer.data[bi:])))
}

// GetUint16 reads the unsigned 16-bit integer at the offset with the given
// endianness, DataView.prototype.getUint16.
func (d *DataView) GetUint16(byteOffset float64, littleEndian ...bool) float64 {
	bi := d.access(byteOffset, 2)
	return float64(dataViewOrder(littleEndian).Uint16(d.buffer.data[bi:]))
}

// GetInt32 reads the signed 32-bit integer at the offset with the given endianness,
// DataView.prototype.getInt32, widening the result to a Number.
func (d *DataView) GetInt32(byteOffset float64, littleEndian ...bool) float64 {
	bi := d.access(byteOffset, 4)
	return float64(int32(dataViewOrder(littleEndian).Uint32(d.buffer.data[bi:])))
}

// GetUint32 reads the unsigned 32-bit integer at the offset with the given
// endianness, DataView.prototype.getUint32.
func (d *DataView) GetUint32(byteOffset float64, littleEndian ...bool) float64 {
	bi := d.access(byteOffset, 4)
	return float64(dataViewOrder(littleEndian).Uint32(d.buffer.data[bi:]))
}

// GetFloat16 reads the half-precision float at the offset with the given endianness,
// DataView.prototype.getFloat16 (25 §25.3.4), decoding the two stored bytes to the
// Number the read hands out.
func (d *DataView) GetFloat16(byteOffset float64, littleEndian ...bool) float64 {
	bi := d.access(byteOffset, 2)
	return float16ToFloat64(dataViewOrder(littleEndian).Uint16(d.buffer.data[bi:]))
}

// GetFloat32 reads the single-precision float at the offset with the given
// endianness, DataView.prototype.getFloat32, widening the stored float32 to a Number.
func (d *DataView) GetFloat32(byteOffset float64, littleEndian ...bool) float64 {
	bi := d.access(byteOffset, 4)
	return float64(math.Float32frombits(dataViewOrder(littleEndian).Uint32(d.buffer.data[bi:])))
}

// GetFloat64 reads the double-precision float at the offset with the given
// endianness, DataView.prototype.getFloat64. A Number is a float64, so the read is
// the stored value with no widening.
func (d *DataView) GetFloat64(byteOffset float64, littleEndian ...bool) float64 {
	bi := d.access(byteOffset, 8)
	return math.Float64frombits(dataViewOrder(littleEndian).Uint64(d.buffer.data[bi:]))
}

// GetBigInt64 reads the signed 64-bit integer at the offset with the given
// endianness as a bigint, DataView.prototype.getBigInt64 (25 §25.3.4). A bigint
// lowers to a *big.Int, so the read widens the stored two's-complement value into
// one rather than the Number the numeric getters return, since a 64-bit integer does
// not fit a Number without loss.
func (d *DataView) GetBigInt64(byteOffset float64, littleEndian ...bool) *big.Int {
	bi := d.access(byteOffset, 8)
	u := dataViewOrder(littleEndian).Uint64(d.buffer.data[bi:])
	return new(big.Int).SetInt64(int64(u))
}

// GetBigUint64 reads the unsigned 64-bit integer at the offset with the given
// endianness as a bigint, DataView.prototype.getBigUint64, the unsigned sibling of
// GetBigInt64 that keeps the full 64-bit magnitude a signed value could not.
func (d *DataView) GetBigUint64(byteOffset float64, littleEndian ...bool) *big.Int {
	bi := d.access(byteOffset, 8)
	u := dataViewOrder(littleEndian).Uint64(d.buffer.data[bi:])
	return new(big.Int).SetUint64(u)
}

// SetInt8 writes v as a signed byte at the offset, DataView.prototype.setInt8. The
// value is a Number the store reduces with ECMAScript ToInt8, the same modulo wrap a
// write into an Int8Array element applies, so 256 stores 0 and -1 stores -1. One byte
// carries no endianness.
func (d *DataView) SetInt8(byteOffset float64, v float64) {
	bi := d.access(byteOffset, 1)
	d.buffer.data[bi] = byte(toInt8(v))
}

// SetUint8 writes v as an unsigned byte at the offset, DataView.prototype.setUint8,
// reducing the Number with ToUint8, the unsigned sibling of SetInt8.
func (d *DataView) SetUint8(byteOffset float64, v float64) {
	bi := d.access(byteOffset, 1)
	d.buffer.data[bi] = toUint8(v)
}

// SetInt16 writes v as a signed 16-bit integer at the offset with the given
// endianness, DataView.prototype.setInt16, reducing the Number with ToInt16 before it
// lays the two bytes down in the chosen byte order.
func (d *DataView) SetInt16(byteOffset float64, v float64, littleEndian ...bool) {
	bi := d.access(byteOffset, 2)
	dataViewOrder(littleEndian).PutUint16(d.buffer.data[bi:], uint16(toInt16(v)))
}

// SetUint16 writes v as an unsigned 16-bit integer at the offset with the given
// endianness, DataView.prototype.setUint16, reducing the Number with ToUint16.
func (d *DataView) SetUint16(byteOffset float64, v float64, littleEndian ...bool) {
	bi := d.access(byteOffset, 2)
	dataViewOrder(littleEndian).PutUint16(d.buffer.data[bi:], toUint16(v))
}

// SetInt32 writes v as a signed 32-bit integer at the offset with the given
// endianness, DataView.prototype.setInt32, reducing the Number with ToInt32.
func (d *DataView) SetInt32(byteOffset float64, v float64, littleEndian ...bool) {
	bi := d.access(byteOffset, 4)
	dataViewOrder(littleEndian).PutUint32(d.buffer.data[bi:], uint32(toInt32(v)))
}

// SetUint32 writes v as an unsigned 32-bit integer at the offset with the given
// endianness, DataView.prototype.setUint32, reducing the Number with ToUint32.
func (d *DataView) SetUint32(byteOffset float64, v float64, littleEndian ...bool) {
	bi := d.access(byteOffset, 4)
	dataViewOrder(littleEndian).PutUint32(d.buffer.data[bi:], toUint32(v))
}

// SetFloat16 writes v as a half-precision float at the offset with the given
// endianness, DataView.prototype.setFloat16 (25 §25.3.4), encoding the Number to the
// two stored bytes.
func (d *DataView) SetFloat16(byteOffset float64, v float64, littleEndian ...bool) {
	bi := d.access(byteOffset, 2)
	dataViewOrder(littleEndian).PutUint16(d.buffer.data[bi:], float64ToFloat16(v))
}

// SetFloat32 writes v as a single-precision float at the offset with the given
// endianness, DataView.prototype.setFloat32, narrowing the Number to a float32 before
// it stores the four bytes.
func (d *DataView) SetFloat32(byteOffset float64, v float64, littleEndian ...bool) {
	bi := d.access(byteOffset, 4)
	dataViewOrder(littleEndian).PutUint32(d.buffer.data[bi:], math.Float32bits(float32(v)))
}

// SetFloat64 writes v as a double-precision float at the offset with the given
// endianness, DataView.prototype.setFloat64. A Number is a float64, so the store lays
// down the value's bits with no narrowing.
func (d *DataView) SetFloat64(byteOffset float64, v float64, littleEndian ...bool) {
	bi := d.access(byteOffset, 8)
	dataViewOrder(littleEndian).PutUint64(d.buffer.data[bi:], math.Float64bits(v))
}

// SetBigInt64 writes v as a signed 64-bit integer at the offset with the given
// endianness, DataView.prototype.setBigInt64 (25 §25.3.4). The value is a bigint,
// which lowers to a *big.Int; the store reduces it modulo 2^64 and lays down the low
// 64 bits, the same bytes a setBigUint64 of the congruent unsigned value would.
func (d *DataView) SetBigInt64(byteOffset float64, v *big.Int, littleEndian ...bool) {
	bi := d.access(byteOffset, 8)
	dataViewOrder(littleEndian).PutUint64(d.buffer.data[bi:], bigIntLow64(v))
}

// SetBigUint64 writes v as an unsigned 64-bit integer at the offset with the given
// endianness, DataView.prototype.setBigUint64, the unsigned sibling of SetBigInt64.
// The stored bytes are the low 64 bits of the value, so it and SetBigInt64 write the
// same bytes for congruent inputs.
func (d *DataView) SetBigUint64(byteOffset float64, v *big.Int, littleEndian ...bool) {
	bi := d.access(byteOffset, 8)
	dataViewOrder(littleEndian).PutUint64(d.buffer.data[bi:], bigIntLow64(v))
}

// bigIntLow64 reduces a bigint to the low 64 bits a 64-bit store writes, ECMAScript's
// wrap of a BigInt into a 64-bit range (the modulo step in SetValueInBuffer). The
// Euclidean modulo lands in [0, 2^64), so a negative value folds up into its
// two's-complement bit pattern, the bytes a signed 64-bit read hands back.
func bigIntLow64(x *big.Int) uint64 {
	var mod, m big.Int
	mod.Lsh(big.NewInt(1), 64)
	m.Mod(x, &mod)
	return m.Uint64()
}

// float64ToFloat16 encodes a Number as an IEEE 754 half-precision bit pattern for a
// setFloat16 store, rounding the mantissa to nearest with ties to even. The value is
// narrowed through a float32 first, then its sign, exponent, and mantissa are rebiased
// to the half's five-bit exponent: an infinity or NaN keeps its class, a magnitude
// past the half's range overflows to infinity, a magnitude below the smallest normal
// stores a subnormal or a signed zero, and every other value takes the top ten
// mantissa bits with the round bias, whose carry folds into the exponent through the
// add.
func float64ToFloat16(v float64) uint16 {
	b := math.Float32bits(float32(v))
	sign := uint16(b>>16) & 0x8000
	exp := int32(b>>23) & 0xff
	mant := b & 0x007fffff
	if exp == 0xff { // infinity or NaN keeps its class.
		if mant != 0 {
			return sign | 0x7e00 // quiet NaN
		}
		return sign | 0x7c00 // infinity
	}
	e := exp - 127 + 15 // rebias the exponent for the half's five bits.
	if e >= 0x1f {      // overflow rounds to infinity.
		return sign | 0x7c00
	}
	if e <= 0 { // subnormal, or underflow to a signed zero.
		if e < -10 {
			return sign
		}
		m := mant | 0x00800000 // restore the hidden leading one.
		shift := uint32(14 - e)
		half := uint32(1) << (shift - 1)
		rounded := (m + half + ((m >> shift) & 1) - 1) >> shift
		return sign | uint16(rounded)
	}
	half := uint32(0x00001000) // round the 13 dropped bits to nearest, ties to even.
	m := mant + half + ((mant >> 13) & 1) - 1
	return sign | (uint16(e<<10) + uint16(m>>13))
}

// float16ToFloat64 decodes an IEEE 754 half-precision bit pattern to the Number a
// getFloat16 read hands out. A zero-exponent pattern is a signed zero or a subnormal
// whose value is mant times 2^-24; an all-ones exponent is an infinity when the
// mantissa is zero and a NaN otherwise; every other pattern is a normal value,
// (1024+mant) times 2^(exp-25), the hidden leading one folded into the mantissa. It
// is computed through Ldexp rather than bit surgery so the exponent shift stays exact.
func float16ToFloat64(h uint16) float64 {
	sign := 1.0
	if h&0x8000 != 0 {
		sign = -1.0
	}
	exp := int(h>>10) & 0x1f
	mant := int(h) & 0x3ff
	switch exp {
	case 0:
		return sign * math.Ldexp(float64(mant), -24)
	case 0x1f:
		if mant == 0 {
			return math.Inf(int(sign))
		}
		return math.NaN()
	default:
		return sign * math.Ldexp(float64(1024+mant), exp-25)
	}
}

// toDataViewIndex is ECMAScript ToIndex applied to a DataView byte offset or length
// (7.1.22): a not-a-number value becomes zero, and a negative or above-2^53-1 value
// is a RangeError, the throw the spec raises for an invalid index. The value arrives
// as the float64 a Number lowers to and is truncated toward zero.
func toDataViewIndex(requestIndex float64) int {
	if requestIndex != requestIndex { // NaN coerces to 0 through ToIntegerOrInfinity.
		return 0
	}
	n := math.Trunc(requestIndex)
	if n < 0 || n > 9007199254740991 {
		Throw(NewRangeError(FromGoString("Invalid DataView index")))
	}
	return int(n)
}
