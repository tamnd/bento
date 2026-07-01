// Package value is bento's shared value model: the Go types that represent
// JavaScript values on both the compiled (typed) and interpreted (dynamic)
// sides, so a value computed by lowered Go and a value computed by the engine
// are the same object. It implements 10_value_model.md.
//
// This file owns the string type. JavaScript strings are sequences of UTF-16
// code units, and Go strings are UTF-8 byte sequences, so the two do not line
// up: a JavaScript string can hold a lone surrogate that is not valid Unicode
// and cannot round-trip through UTF-8. So a lowered `string` is not a Go string;
// it is a BStr, a UTF-16 string with a UTF-8 fast path for the common case where
// the text is valid UTF-8 that never needs the code-unit view (05_type_lowering
// section 5, 10_value_model section 5.4).
package value

import (
	"math"
	"unicode/utf16"
)

// BStr is a JavaScript string: a sequence of UTF-16 code units. It keeps two
// coherent views. Most strings are valid UTF-8 (they came from source, JSON, or
// the network), so the common case stores the UTF-8 bytes and never allocates
// the code-unit slice; utf16 is populated only when the string holds a lone
// surrogate that UTF-8 cannot represent, or when an operation forces the
// code-unit view. lengthU16 is always the UTF-16 code-unit count, which is what
// String.prototype.length reports, so .length is O(1) whichever view is live.
//
// A BStr is immutable, like a JavaScript string, so it is passed and shared by
// value with no defensive copy and no locking.
type BStr struct {
	utf8      string   // valid-UTF-8 fast path; used when utf16 is nil
	utf16     []uint16 // populated only when utf8 cannot represent the value
	lengthU16 int      // length in UTF-16 code units (String.prototype.length)
}

// FromGoString builds a BStr from a Go string. A Go string is UTF-8, which is
// always representable as UTF-16, so the result keeps the UTF-8 fast path and
// only counts code units; no surrogate array is allocated. Invalid UTF-8 bytes
// in the input are counted as the U+FFFD replacement each rune decode yields, so
// the length matches what materializing the string would produce. This is the
// transcode a Go value takes when it crosses into the JavaScript world
// (05_type_lowering section 5, bento.FromGoString).
func FromGoString(s string) BStr {
	return BStr{utf8: s, lengthU16: countUTF16Units(s)}
}

// FromUTF16 builds a BStr from raw UTF-16 code units, the constructor for a
// value that may contain a lone surrogate. If the units happen to be a valid
// UTF-8-representable string this still keeps the code-unit view, because the
// caller reached for this path precisely when the UTF-8 fast path would not be
// safe; callers with UTF-8 in hand use FromGoString. The units are copied so a
// later mutation of the caller's slice cannot change an immutable string.
func FromUTF16(units []uint16) BStr {
	cp := make([]uint16, len(units))
	copy(cp, units)
	return BStr{utf16: cp, lengthU16: len(cp)}
}

// Length returns the number of UTF-16 code units, String.prototype.length. It is
// a JavaScript number, so it is returned as a float64 to match the lowered type
// of a number without a conversion at the use site.
func (s BStr) Length() float64 {
	return float64(s.lengthU16)
}

// CharCodeAt returns the UTF-16 code unit at index i as a number, matching
// String.prototype.charCodeAt. The index is coerced to an integer the way
// JavaScript does (NaN becomes 0, a fraction truncates toward zero), and an
// index outside [0, length) yields NaN, not a zero or a panic. The result is a
// float64 because the code unit is a JavaScript number and JavaScript has no
// character type. The range test is done on the float before the int conversion
// so a huge index cannot overflow int on the way in.
func (s BStr) CharCodeAt(i float64) float64 {
	if math.IsNaN(i) {
		i = 0
	}
	i = math.Trunc(i)
	if i < 0 || i >= float64(s.lengthU16) {
		return math.NaN()
	}
	return float64(s.units()[int(i)])
}

// CharAt returns the one-code-unit string at index i, matching
// String.prototype.charAt. The index is coerced to an integer the same way
// CharCodeAt coerces it, and an index outside [0, length) yields the empty
// string rather than a panic. The single code unit may be a lone surrogate (half
// of an astral character), so the result is built from the raw unit through
// FromUTF16, which preserves a surrogate that FromGoString could not, keeping
// charAt(0) of an astral character the exact high surrogate JavaScript returns.
func (s BStr) CharAt(i float64) BStr {
	if math.IsNaN(i) {
		i = 0
	}
	i = math.Trunc(i)
	if i < 0 || i >= float64(s.lengthU16) {
		return BStr{}
	}
	return FromUTF16([]uint16{s.units()[int(i)]})
}

// Concat returns the concatenation of a and b, the lowering of `a + b` when both
// are strings. It picks the backing form once: if both sides are on the UTF-8
// fast path the result stays UTF-8 with a single byte copy, and otherwise the
// result materializes the code-unit view of each side and appends, so a lone
// surrogate on either side survives. Go's own `+` is never used on the BStr
// struct, and Go string concat is never used as the lowering, because that would
// be UTF-8 semantics on a UTF-16 string (05_type_lowering section 5).
func Concat(a, b BStr) BStr {
	if a.utf16 == nil && b.utf16 == nil {
		return BStr{utf8: a.utf8 + b.utf8, lengthU16: a.lengthU16 + b.lengthU16}
	}
	units := make([]uint16, 0, a.lengthU16+b.lengthU16)
	units = a.appendUnits(units)
	units = b.appendUnits(units)
	return BStr{utf16: units, lengthU16: len(units)}
}

// Equal reports whether a and b are the same string, code unit for code unit,
// which is JavaScript string === and == on two strings. When both are on the
// UTF-8 fast path the bytes compare directly, since equal UTF-8 means equal code
// units; otherwise the code-unit views compare, so two strings that differ only
// in how they are backed still compare equal.
func (a BStr) Equal(b BStr) bool {
	if a.lengthU16 != b.lengthU16 {
		return false
	}
	if a.utf16 == nil && b.utf16 == nil {
		return a.utf8 == b.utf8
	}
	ua, ub := a.units(), b.units()
	for i := range ua {
		if ua[i] != ub[i] {
			return false
		}
	}
	return true
}

// ToGoString returns the UTF-8 view of the string, the transcode a lowered
// string takes when it is handed to a Go library or node: API that wants a Go
// string (05_type_lowering section 5, bento.ToGoString). A string that holds a
// lone surrogate cannot be represented in UTF-8, so each unpaired surrogate
// becomes the U+FFFD replacement, the same lossy mapping the platform makes when
// such a string is written to a byte sink.
func (s BStr) ToGoString() string {
	if s.utf16 == nil {
		return s.utf8
	}
	return string(utf16.Decode(s.utf16))
}

// units returns the UTF-16 code units of the string, materializing them from the
// UTF-8 fast path when needed. It does not cache the result on the value,
// because a BStr is passed by value; a caller that needs the units repeatedly
// holds them itself.
func (s BStr) units() []uint16 {
	if s.utf16 != nil {
		return s.utf16
	}
	return utf16.Encode([]rune(s.utf8))
}

// appendUnits appends the string's code units to dst and returns the extended
// slice, the building block Concat uses so neither side is materialized into its
// own temporary.
func (s BStr) appendUnits(dst []uint16) []uint16 {
	return append(dst, s.units()...)
}

// countUTF16Units counts the UTF-16 code units a UTF-8 string encodes to,
// without allocating the code-unit slice: a rune outside the Basic Multilingual
// Plane takes two units (a surrogate pair) and every other rune takes one. This
// is what makes Length O(1) for the UTF-8 fast path.
func countUTF16Units(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}
