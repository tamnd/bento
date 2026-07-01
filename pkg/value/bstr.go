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

// IndexOf returns the code-unit index of the first occurrence of search at or
// after an optional start position, or -1 if it does not occur, matching
// String.prototype.indexOf. The position is optional, so the method is variadic:
// the lowered call passes exactly the arguments the source did. A position is run
// through the substring clamp (ToInteger, then into [0, length], with NaN going to
// 0), and the scan begins there. The search is code-unit-wise over the UTF-16 view
// so it agrees with JavaScript on astral text and lone surrogates, and an empty
// search string matches at the clamped position the way JavaScript defines it. The
// result is a float64 because it is a JavaScript number.
func (s BStr) IndexOf(search BStr, position ...float64) float64 {
	hay, needle := s.units(), search.units()
	start := 0
	if len(position) >= 1 {
		start = clampIndex(position[0], len(hay))
	}
	if len(needle) == 0 {
		return float64(start)
	}
	for i := start; i+len(needle) <= len(hay); i++ {
		if matchAt(hay, needle, i) {
			return float64(i)
		}
	}
	return -1
}

// LastIndexOf returns the code-unit index of the last occurrence of search that
// begins at or before an optional position, or -1 if it does not occur, matching
// String.prototype.lastIndexOf. It scans backward, so it reports the greatest
// matching index rather than the least. The position defaults to the end and,
// unlike indexOf, a NaN position also means the end (the specification coerces a
// missing or NaN position to +Infinity here), so only a real number narrows the
// window. An empty search string matches at the clamped position. The search is
// code-unit-wise so it agrees with JavaScript on astral text and lone surrogates.
func (s BStr) LastIndexOf(search BStr, position ...float64) float64 {
	hay, needle := s.units(), search.units()
	start := len(hay)
	if len(position) >= 1 && !math.IsNaN(position[0]) {
		start = clampIndex(position[0], len(hay))
	}
	if len(needle) == 0 {
		return float64(start)
	}
	if start > len(hay)-len(needle) {
		start = len(hay) - len(needle)
	}
	for i := start; i >= 0; i-- {
		if matchAt(hay, needle, i) {
			return float64(i)
		}
	}
	return -1
}

// Includes reports whether search occurs at or after an optional start position,
// matching String.prototype.includes. It is defined in terms of IndexOf, so it
// shares the code-unit search, the optional position, and the empty-string rule
// (an empty search is found at the clamped position, so Includes returns true).
func (s BStr) Includes(search BStr, position ...float64) bool {
	return s.IndexOf(search, position...) >= 0
}

// StartsWith reports whether s has prefix starting at an optional position,
// matching String.prototype.startsWith. The position is optional, so the method
// is variadic; it is clamped into [0, length] and the prefix is matched there. It
// compares code units so it agrees with JavaScript on astral text, and a prefix
// that would run past the end is not a match.
func (s BStr) StartsWith(prefix BStr, position ...float64) bool {
	hay, needle := s.units(), prefix.units()
	start := 0
	if len(position) >= 1 {
		start = clampIndex(position[0], len(hay))
	}
	if start+len(needle) > len(hay) {
		return false
	}
	return matchAt(hay, needle, start)
}

// EndsWith reports whether s has suffix ending at an optional end position,
// matching String.prototype.endsWith. Where startsWith takes the index the match
// begins at, endsWith takes the index it ends at, which defaults to the length;
// the suffix is matched in the window that ends there. The end position is clamped
// into [0, length], and a suffix longer than that window is not a match. It
// compares code units so it agrees with JavaScript on astral text.
func (s BStr) EndsWith(suffix BStr, endPosition ...float64) bool {
	hay, needle := s.units(), suffix.units()
	end := len(hay)
	if len(endPosition) >= 1 {
		end = clampIndex(endPosition[0], len(hay))
	}
	start := end - len(needle)
	if start < 0 {
		return false
	}
	return matchAt(hay, needle, start)
}

// matchAt reports whether needle appears in hay starting at index i. The caller
// guarantees i+len(needle) <= len(hay), so the inner loop needs no bound check
// beyond the slice itself.
func matchAt(hay, needle []uint16, i int) bool {
	for j := range needle {
		if hay[i+j] != needle[j] {
			return false
		}
	}
	return true
}

// Slice returns the substring between two code-unit indices, matching
// String.prototype.slice. Both arguments are optional (start defaults to 0, end
// to the length), which is why the method is variadic: the lowered call passes
// exactly the arguments the source did, and the count selects the defaults. A
// negative index counts from the end, an index past the end clamps to the end,
// and a start at or after the end yields the empty string. Working on the
// code-unit view means a slice can land between the halves of an astral
// character and return a lone surrogate, exactly as JavaScript does.
func (s BStr) Slice(args ...float64) BStr {
	start, end := 0, s.lengthU16
	if len(args) >= 1 {
		start = relIndex(args[0], s.lengthU16)
	}
	if len(args) >= 2 {
		end = relIndex(args[1], s.lengthU16)
	}
	if start >= end {
		return BStr{}
	}
	return s.sub(start, end)
}

// Substring returns the substring between two code-unit indices, matching
// String.prototype.substring. It differs from Slice in its edge handling: a
// negative or NaN argument becomes 0 rather than counting from the end, and if
// start is greater than end the two are swapped rather than yielding the empty
// string. It is variadic for the same reason Slice is.
func (s BStr) Substring(args ...float64) BStr {
	start, end := 0, s.lengthU16
	if len(args) >= 1 {
		start = clampIndex(args[0], s.lengthU16)
	}
	if len(args) >= 2 {
		end = clampIndex(args[1], s.lengthU16)
	}
	if start > end {
		start, end = end, start
	}
	return s.sub(start, end)
}

// sub returns the code units in [start, end) as a new string. The caller has
// already clamped both bounds into [0, length] and ordered them, so the slice is
// always valid. It goes through FromUTF16 because a substring can split an astral
// pair into a lone surrogate that the UTF-8 backing could not hold.
func (s BStr) sub(start, end int) BStr {
	return FromUTF16(s.units()[start:end])
}

// toInteger applies the JavaScript ToInteger coercion an index argument gets: NaN
// becomes 0 and every other value truncates toward zero.
func toInteger(n float64) float64 {
	if math.IsNaN(n) {
		return 0
	}
	return math.Trunc(n)
}

// relIndex resolves a slice index that may be negative: after ToInteger, a
// negative index counts back from the end and clamps at 0, and a positive index
// clamps at the length. This is the String.prototype.slice index rule.
func relIndex(i float64, length int) int {
	n := toInteger(i)
	l := float64(length)
	if n < 0 {
		if n += l; n < 0 {
			n = 0
		}
	} else if n > l {
		n = l
	}
	return int(n)
}

// clampIndex resolves a substring index, which has no negative-from-end rule: a
// negative or NaN index becomes 0 and a large one clamps at the length. This is
// the String.prototype.substring index rule.
func clampIndex(i float64, length int) int {
	n := toInteger(i)
	l := float64(length)
	if n < 0 {
		n = 0
	} else if n > l {
		n = l
	}
	return int(n)
}

// Trim removes leading and trailing whitespace, matching String.prototype.trim.
// The whitespace set is the exact ECMAScript one (WhiteSpace plus
// LineTerminator, isStringWhiteSpace below), not Go's unicode.IsSpace, which
// disagrees at the edges (Go counts U+0085 as space and not U+FEFF, JavaScript
// does the reverse). When nothing is trimmed the receiver is returned unchanged
// so a string with no surrounding whitespace keeps its backing and allocates
// nothing.
func (s BStr) Trim() BStr {
	u := s.units()
	i, j := 0, len(u)
	for i < j && isStringWhiteSpace(u[i]) {
		i++
	}
	for j > i && isStringWhiteSpace(u[j-1]) {
		j--
	}
	if i == 0 && j == len(u) {
		return s
	}
	return FromUTF16(u[i:j])
}

// TrimStart removes leading whitespace only, matching String.prototype.trimStart,
// over the same whitespace set as Trim.
func (s BStr) TrimStart() BStr {
	u := s.units()
	i := 0
	for i < len(u) && isStringWhiteSpace(u[i]) {
		i++
	}
	if i == 0 {
		return s
	}
	return FromUTF16(u[i:])
}

// TrimEnd removes trailing whitespace only, matching String.prototype.trimEnd,
// over the same whitespace set as Trim.
func (s BStr) TrimEnd() BStr {
	u := s.units()
	j := len(u)
	for j > 0 && isStringWhiteSpace(u[j-1]) {
		j--
	}
	if j == len(u) {
		return s
	}
	return FromUTF16(u[:j])
}

// isStringWhiteSpace reports whether a code unit is trimmed by String.prototype
// .trim, the union of the ECMAScript WhiteSpace set (tab, vertical tab, form
// feed, space, no-break space, zero-width no-break space, and the Space_Separator
// category) and the LineTerminator set (LF, CR, line separator, paragraph
// separator). Every one of these is in the Basic Multilingual Plane, so a single
// uint16 test is exact; there is no astral whitespace to miss.
func isStringWhiteSpace(u uint16) bool {
	switch u {
	case 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x20, 0xA0,
		0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	// U+2000..U+200A are the remaining Space_Separator code points.
	return u >= 0x2000 && u <= 0x200A
}

// PadStart pads the front of s with pad until the result is targetLength code
// units long, matching String.prototype.padStart. targetLength is coerced to an
// integer JavaScript-style (NaN becomes 0, a fraction truncates), and if it is
// not longer than s the receiver is returned unchanged, which also covers a
// negative target. The pad string is optional and defaults to a single space;
// an explicitly empty pad string produces no filler, so the receiver comes back
// unchanged. The filler is the pad repeated and then truncated to the exact fill
// length on the code-unit view, so truncating in the middle of an astral pad can
// emit a lone surrogate, exactly as JavaScript does.
func (s BStr) PadStart(targetLength float64, pad ...BStr) BStr {
	return s.padTo(targetLength, pad, true)
}

// PadEnd pads the end of s with pad until the result is targetLength code units
// long, matching String.prototype.padEnd. It shares every rule with PadStart and
// differs only in appending the filler after s rather than before it.
func (s BStr) PadEnd(targetLength float64, pad ...BStr) BStr {
	return s.padTo(targetLength, pad, false)
}

// padTo is the shared body of PadStart and PadEnd; atStart selects which side the
// filler goes on. The out slice is freshly allocated and owned here, so it backs
// the result directly without the defensive copy FromUTF16 would add.
func (s BStr) padTo(targetLength float64, pad []BStr, atStart bool) BStr {
	if toInteger(targetLength) <= float64(s.lengthU16) {
		return s
	}
	target := int(toInteger(targetLength))
	pu := padUnits(pad)
	if len(pu) == 0 {
		return s
	}
	fillLen := target - s.lengthU16
	filler := make([]uint16, 0, fillLen)
	for len(filler) < fillLen {
		filler = append(filler, pu...)
	}
	filler = filler[:fillLen]
	out := make([]uint16, 0, target)
	if atStart {
		out = append(out, filler...)
		out = append(out, s.units()...)
	} else {
		out = append(out, s.units()...)
		out = append(out, filler...)
	}
	return BStr{utf16: out, lengthU16: len(out)}
}

// padUnits returns the code units the pad argument contributes: the caller's pad
// string when one was passed, or a single space (U+0020), the default pad
// String.prototype.padStart and padEnd use when the argument is absent.
func padUnits(pad []BStr) []uint16 {
	if len(pad) == 0 {
		return []uint16{0x20}
	}
	return pad[0].units()
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

// ConcatN returns the receiver followed by every argument in order, the lowering
// of str.concat(a, b, ...). It stays on the UTF-8 fast path while the receiver and
// every argument seen so far are UTF-8, appending bytes; the first argument that
// carries a raw code-unit backing switches the whole result to the code-unit form
// so a lone surrogate survives, matching how Concat picks a backing. With no
// arguments it returns the receiver unchanged, which is what concat() with no
// arguments does.
func (s BStr) ConcatN(rest ...BStr) BStr {
	out := s
	for _, r := range rest {
		out = Concat(out, r)
	}
	return out
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
