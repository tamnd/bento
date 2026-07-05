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
	"strings"
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
	utf8      string    // valid-UTF-8 fast path; used when utf16 is nil and rope is nil
	utf16     []uint16  // populated only when utf8 cannot represent the value
	lengthU16 int       // length in UTF-16 code units (String.prototype.length)
	rope      *ropeNode // set for a lazy concatenation; utf8 and utf16 are empty until flattened
}

// ropeNode is a deferred concatenation of two strings. Concat of a growing string
// builds one of these in O(1) instead of copying, so a string built up over a
// loop (s += ...) or a join is linear in its final length rather than quadratic.
// The content is materialized on first demand and cached on the node, which every
// copy of the BStr shares because it holds the same pointer, so a rope flattens at
// most once however many times it is read. bento's compiled programs are
// single-threaded today, so the cache needs no lock; a concurrent runtime would
// guard flat.
type ropeNode struct {
	left, right BStr
	flat        *BStr // cached materialized form, nil until first resolve
}

// concatFlatMax is the largest result Concat copies eagerly. Below it a copy is
// cheaper than a rope node and keeps the common small concatenation (a template,
// a coerced operand) on a plain backing so its next read needs no flatten. At or
// above it, or when either side is already a rope, Concat defers so an
// accumulation loop stays linear.
const concatFlatMax = 64

// flat returns an equivalent string with no pending rope, materializing and
// caching the concatenation the first time it is needed. A string that is not a
// rope is returned unchanged, the single cheap branch every field-reading method
// takes on the common path. Length does not call this, so .length on a rope stays
// O(1).
func (s BStr) flat() BStr {
	if s.rope == nil {
		return s
	}
	if s.rope.flat != nil {
		return *s.rope.flat
	}
	leaves := appendLeaves(make([]BStr, 0, 16), s)
	utf8Only := true
	total := 0
	for i := range leaves {
		if leaves[i].utf16 != nil {
			utf8Only = false
		}
		total += leaves[i].lengthU16
	}
	var out BStr
	if utf8Only {
		var b strings.Builder
		b.Grow(total)
		for i := range leaves {
			b.WriteString(leaves[i].utf8)
		}
		out = BStr{utf8: b.String(), lengthU16: total}
	} else {
		units := make([]uint16, 0, total)
		for i := range leaves {
			units = leaves[i].appendUnits(units)
		}
		out = BStr{utf16: units, lengthU16: len(units)}
	}
	s.rope.flat = &out
	return out
}

// appendLeaves appends the non-empty leaf strings of a rope to out in left-to-right
// order, walking the tree iteratively so a deep left-leaning chain (which is what
// an accumulation loop builds) cannot overflow the goroutine stack. A subtree that
// is already flattened is taken as a single leaf from its cache rather than
// re-descended.
func appendLeaves(out []BStr, s BStr) []BStr {
	var stack []BStr
	for {
		for s.rope != nil && s.rope.flat == nil {
			stack = append(stack, s.rope.right)
			s = s.rope.left
		}
		if s.rope != nil {
			s = *s.rope.flat
		}
		if s.lengthU16 > 0 {
			out = append(out, s)
		}
		if len(stack) == 0 {
			return out
		}
		s = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
	}
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

// FromCharCode builds a string from UTF-16 code units, String.fromCharCode. Each
// argument is coerced to a 16-bit unsigned integer by ToUint16, the same
// truncation the specification applies, so a number outside [0, 2^16) wraps
// rather than being rejected and a fraction is dropped. The units are taken
// verbatim, which means a lone surrogate is preserved rather than replaced, so
// the result keeps the code-unit view rather than the UTF-8 fast path. It is a
// free function, not a method, because fromCharCode is a static on the String
// constructor with no string receiver.
//
// The units slice is allocated and filled here and never handed back to the
// caller, so it backs the result directly, the way padTo does, without the
// defensive copy FromUTF16 makes for a slice it does not own. That is the one
// allocation the call costs; routing through FromUTF16 would double it.
func FromCharCode(codes ...float64) BStr {
	units := make([]uint16, len(codes))
	for i, c := range codes {
		units[i] = ToUint16(c)
	}
	return BStr{utf16: units, lengthU16: len(units)}
}

// FromCodePoint builds a string from Unicode code points, String.fromCodePoint.
// Unlike fromCharCode, each argument is a full code point, not a code unit, so an
// astral point above U+FFFF is encoded as the surrogate pair that spells it in
// UTF-16 and adds two code units, not one. Each argument must be an integer in
// [0, 0x10FFFF]; a negative number, a fraction, or a value past the last code
// point is not a valid code point, so it throws a RangeError the way the
// specification requires rather than wrapping the way fromCharCode does. The
// units are taken verbatim into the code-unit view, so a lone surrogate passed
// directly is preserved. It is a free function for the same reason fromCharCode
// is: fromCodePoint is a static on the String constructor with no receiver.
func FromCodePoint(codePoints ...float64) BStr {
	units := make([]uint16, 0, len(codePoints))
	for _, c := range codePoints {
		if math.IsNaN(c) || math.Trunc(c) != c || c < 0 || c > 0x10FFFF {
			Throw(NewRangeError(FromGoString("Invalid code point " + formatCodePoint(c))))
		}
		cp := uint32(c)
		if cp <= 0xFFFF {
			units = append(units, uint16(cp))
			continue
		}
		cp -= 0x10000
		units = append(units, uint16(0xD800+(cp>>10)), uint16(0xDC00+(cp&0x3FF)))
	}
	return BStr{utf16: units, lengthU16: len(units)}
}

// formatCodePoint renders a rejected code point for the RangeError message the
// way JavaScript prints it: an integer with no fraction, a fraction as its
// shortest decimal, so String.fromCodePoint(-1) reports "Invalid code point -1".
func formatCodePoint(c float64) string {
	return NumberToString(c).ToGoString()
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
	idx := int(i)
	// ASCII fast path: when the UTF-8 view has one byte per code unit, every rune
	// is ASCII, so the code unit at idx is the byte at idx and no code-unit slice
	// has to be built. len(utf8) == lengthU16 is that all-ASCII test, since any
	// multi-byte rune has more bytes than the one or two code units it encodes to.
	// This matters because CharCodeAt on a UTF-8 string otherwise routes through
	// units(), which materializes the whole UTF-16 slice for a single lookup, so a
	// per-character scan of a built or formatted string would re-encode it on every
	// index. A rope or a raw code-unit backing takes the general path below.
	if s.rope == nil && s.utf16 == nil && len(s.utf8) == s.lengthU16 {
		return float64(s.utf8[idx])
	}
	return float64(s.units()[idx])
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

// AtOpt returns the one-code-unit string at the relative index i, matching
// String.prototype.at. Its declared type is string | undefined, so it returns an
// Opt[BStr]: a present optional holding the single-code-unit string when the
// resolved index falls in range, and the undefined optional otherwise. The index
// is coerced to an integer the same way CharAt coerces it, then a negative index
// counts back from the end. An index still outside [0, length) reads as the
// undefined optional rather than the empty string CharAt yields, matching at's
// out-of-range result. The single unit may be a lone surrogate, so it is rebuilt
// through FromUTF16, which preserves a surrogate FromGoString could not.
func (s BStr) AtOpt(i float64) Opt[BStr] {
	if math.IsNaN(i) {
		i = 0
	}
	i = math.Trunc(i)
	if i < 0 {
		i += float64(s.lengthU16)
	}
	if i < 0 || i >= float64(s.lengthU16) {
		return None[BStr]()
	}
	return Some(FromUTF16([]uint16{s.units()[int(i)]}))
}

// CodePointAtOpt returns the Unicode code point that begins at code-unit index i
// as a number, matching String.prototype.codePointAt. Its declared type is number
// | undefined, so it returns an Opt[float64]: the undefined optional when the
// index is outside [0, length), and the present optional otherwise. When the unit
// at i is a high surrogate and the next unit is a low surrogate, the two combine
// into the single astral code point they encode, which is the difference from
// charCodeAt; a high surrogate with no following low half, a lone low surrogate,
// or any BMP unit reads as that unit's own value. The index is coerced to an
// integer the same way charCodeAt coerces it, and the range test is on the float
// before the int conversion so a huge index cannot overflow int on the way in.
func (s BStr) CodePointAtOpt(i float64) Opt[float64] {
	if math.IsNaN(i) {
		i = 0
	}
	i = math.Trunc(i)
	if i < 0 || i >= float64(s.lengthU16) {
		return None[float64]()
	}
	idx := int(i)
	units := s.units()
	first := units[idx]
	// A high surrogate followed by a low surrogate is one astral code point; any
	// other unit, including an unpaired surrogate, stands for itself.
	if first >= 0xD800 && first <= 0xDBFF && idx+1 < len(units) {
		second := units[idx+1]
		if second >= 0xDC00 && second <= 0xDFFF {
			cp := (int(first)-0xD800)*0x400 + (int(second) - 0xDC00) + 0x10000
			return Some(float64(cp))
		}
	}
	return Some(float64(first))
}

// CodePoints returns the string's Unicode code points in order, each as its own
// one- or two-code-unit string. This is what a for...of loop over a string
// iterates: JavaScript steps a string by code point, so a surrogate pair (an
// astral character) yields as a single two-unit element and every other unit,
// including a lone surrogate, yields as its own one-unit element. It is the string
// counterpart of an array's Elems, the slice a ranged Go loop walks. A pair is
// rebuilt through FromUTF16 so a lone surrogate that UTF-8 cannot represent is
// preserved rather than replaced.
func (s BStr) CodePoints() []BStr {
	units := s.units()
	out := make([]BStr, 0, len(units))
	for i := 0; i < len(units); i++ {
		first := units[i]
		// A high surrogate followed by a low surrogate is one astral code point of
		// two units; any other unit, including an unpaired surrogate, stands alone.
		if first >= 0xD800 && first <= 0xDBFF && i+1 < len(units) {
			if second := units[i+1]; second >= 0xDC00 && second <= 0xDFFF {
				out = append(out, FromUTF16([]uint16{first, second}))
				i++
				continue
			}
		}
		out = append(out, FromUTF16([]uint16{first}))
	}
	return out
}

// IsWellFormed reports whether the string contains no lone surrogate code unit,
// matching String.prototype.isWellFormed. In a well-formed string every high
// surrogate (D800..DBFF) is immediately followed by a low surrogate (DC00..DFFF)
// and every low surrogate follows a high one; a surrogate that breaks that pairing
// is lone and makes the string ill-formed. A string on the UTF-8 fast path cannot
// hold a lone surrogate, because UTF-8 cannot encode one, so it answers true with
// no scan; only a string with a raw code-unit backing is walked.
func (s BStr) IsWellFormed() bool {
	s = s.flat()
	if s.utf16 == nil {
		return true
	}
	units := s.utf16
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u < 0xD800 || u > 0xDFFF {
			continue
		}
		if u >= 0xDC00 {
			// A low surrogate with no high surrogate before it is lone.
			return false
		}
		// A high surrogate must be paired with a low surrogate in the next unit.
		if i+1 >= len(units) || units[i+1] < 0xDC00 || units[i+1] > 0xDFFF {
			return false
		}
		i++
	}
	return true
}

// ToWellFormed returns the string with every lone surrogate replaced by U+FFFD,
// matching String.prototype.toWellFormed. A well-formed string is returned
// unchanged, so it keeps its backing and costs no allocation; only a string that
// holds a lone surrogate is rebuilt, with each unpaired surrogate swapped for the
// replacement character and every valid pair copied through untouched.
func (s BStr) ToWellFormed() BStr {
	s = s.flat()
	if s.utf16 == nil || s.IsWellFormed() {
		return s
	}
	units := s.utf16
	out := make([]uint16, 0, len(units))
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u < 0xD800 || u > 0xDFFF {
			out = append(out, u)
			continue
		}
		if u < 0xDC00 && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF {
			out = append(out, u, units[i+1])
			i++
			continue
		}
		out = append(out, 0xFFFD)
	}
	return BStr{utf16: out, lengthU16: len(out)}
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

// Replace returns s with the first occurrence of search replaced by the
// expansion of replacement, matching String.prototype.replace called with two
// strings. (The regexp pattern and the replacer-function forms are a later
// slice.) The search is code-unit-wise, so it agrees with JavaScript on astral
// text and lone surrogates, and when search does not occur the receiver is
// returned unchanged with no allocation. An empty search matches at the front,
// so the replacement is inserted there. The replacement string carries the
// ECMAScript substitution patterns: see appendSubstitution.
func (s BStr) Replace(search, replacement BStr) BStr {
	// Fast path: receiver, search, and replacement are all valid UTF-8, the search
	// is non-empty, and the replacement carries no substitution pattern ($& and its
	// kin). UTF-8 is self-synchronizing, so a byte search finds the same first match
	// a code-unit search would, and with no $ pattern the replacement is a literal
	// splice, which strings.Replace with a count of one does directly on bytes and
	// keeps the result on the UTF-8 fast path. This skips materializing the whole
	// receiver into a code-unit slice, the cost that dominated the strings workload.
	if sf, ff := s.flat(), search.flat(); sf.utf16 == nil && ff.utf16 == nil && ff.lengthU16 > 0 {
		if rf := replacement.flat(); rf.utf16 == nil {
			pos := strings.Index(sf.utf8, ff.utf8)
			if pos < 0 {
				return s
			}
			if !strings.ContainsRune(rf.utf8, '$') {
				return FromGoString(strings.Replace(sf.utf8, ff.utf8, rf.utf8, 1))
			}
			// A replacement carrying $ patterns still splices on bytes: $&, $` and $'
			// copy whole byte ranges of the receiver and $$ a literal $, so every piece
			// stays valid UTF-8 and the code-unit path's utf16.Encode of the receiver,
			// the cost that dominated the replace workload, is never paid.
			out := make([]byte, 0, len(sf.utf8)+len(rf.utf8))
			out = append(out, sf.utf8[:pos]...)
			out = appendSubstitutionBytes(out, sf.utf8, pos, len(ff.utf8), rf.utf8)
			out = append(out, sf.utf8[pos+len(ff.utf8):]...)
			return FromGoString(string(out))
		}
	}
	hay, needle := s.units(), search.units()
	pos := indexOfUnits(hay, needle, 0)
	if pos < 0 {
		return s
	}
	out := make([]uint16, 0, len(hay)+len(replacement.units()))
	out = append(out, hay[:pos]...)
	out = appendSubstitution(out, hay, pos, len(needle), replacement.units())
	out = append(out, hay[pos+len(needle):]...)
	return FromUTF16(out)
}

// ReplaceAll returns s with every non-overlapping occurrence of search replaced
// by the expansion of replacement, matching String.prototype.replaceAll called
// with two strings. It shares Replace's code-unit search and substitution rules.
// An empty search matches at every position including both ends, so the
// replacement is woven between every code unit, matching how JavaScript expands
// "abc".replaceAll("", "-") to "-a-b-c-". When search does not occur the receiver
// is returned unchanged.
func (s BStr) ReplaceAll(search, replacement BStr) BStr {
	// Fast path, the same conditions Replace takes: all three strings valid UTF-8, a
	// non-empty search, and a replacement with no substitution pattern. Under those
	// strings.ReplaceAll on bytes produces the exact code units the loop below would,
	// without materializing the receiver into a code-unit slice. The empty-search
	// weave and the $ patterns still fall through to the code-unit path.
	if sf, ff := s.flat(), search.flat(); sf.utf16 == nil && ff.utf16 == nil && ff.lengthU16 > 0 {
		if rf := replacement.flat(); rf.utf16 == nil {
			if !strings.Contains(sf.utf8, ff.utf8) {
				return s
			}
			if !strings.ContainsRune(rf.utf8, '$') {
				return FromGoString(strings.ReplaceAll(sf.utf8, ff.utf8, rf.utf8))
			}
			// A $-carrying replacement splices on bytes at every match, the same
			// byte-range copies Replace does, so the receiver is never materialized
			// into code units. The search is non-empty here, so there is no zero-width
			// weave, which the code-unit path below still owns.
			hay, needle := sf.utf8, ff.utf8
			out := make([]byte, 0, len(hay))
			at := 0
			for {
				pos := strings.Index(hay[at:], needle)
				if pos < 0 {
					break
				}
				pos += at
				out = append(out, hay[at:pos]...)
				out = appendSubstitutionBytes(out, hay, pos, len(needle), rf.utf8)
				at = pos + len(needle)
			}
			out = append(out, hay[at:]...)
			return FromGoString(string(out))
		}
	}
	hay, needle, repl := s.units(), search.units(), replacement.units()
	out := make([]uint16, 0, len(hay))
	i := 0
	matched := false
	for {
		pos := indexOfUnits(hay, needle, i)
		if pos < 0 {
			break
		}
		matched = true
		out = append(out, hay[i:pos]...)
		out = appendSubstitution(out, hay, pos, len(needle), repl)
		if len(needle) == 0 {
			// A zero-width match cannot advance on its own, so emit the code unit at
			// the match and step one past it; that also puts a final replacement past
			// the last unit before the scan runs off the end.
			if pos < len(hay) {
				out = append(out, hay[pos])
			}
			i = pos + 1
		} else {
			i = pos + len(needle)
		}
		if i > len(hay) {
			break
		}
	}
	if !matched {
		return s
	}
	out = append(out, hay[min(i, len(hay)):]...)
	return FromUTF16(out)
}

// Split divides the string on each occurrence of a string separator and returns
// the pieces, String.prototype.split(separator) in its string-separator form
// (the regexp form and the optional limit are not modeled here; the compiler
// hands those back). The pieces are cut on code-unit boundaries, so a separator
// that occurs at the start or end yields an empty leading or trailing piece and a
// separator that does not occur yields the whole string as the one piece, exactly
// as JavaScript does. An empty separator splits into single code units, and the
// empty string split by an empty separator is the empty array, the two edges the
// specification calls out.
func (s BStr) Split(sep BStr) *Array[BStr] {
	s, sep = s.flat(), sep.flat()
	// Fast path: the receiver and the separator are both valid UTF-8 and the
	// separator is non-empty. UTF-8 is self-synchronizing, so a byte-level split
	// lands on exactly the boundaries a code-unit split would and every piece is
	// itself valid UTF-8, which keeps each piece on the UTF-8 fast path.
	if s.utf16 == nil && sep.utf16 == nil && sep.lengthU16 > 0 {
		hay, sp := s.utf8, sep.utf8
		// Size the result exactly, then cut the pieces as substrings of hay, which
		// share its bytes with no copy. Building the []BStr directly avoids the
		// intermediate []string strings.Split would allocate and the second copy
		// NewArray would make, the two allocations that dominated the split cost.
		out := make([]BStr, 0, strings.Count(hay, sp)+1)
		// A pure-ASCII receiver (every byte is one code unit) lets each piece take its
		// byte length as its code-unit length with no rescan, the common case here.
		ascii := s.lengthU16 == len(hay)
		start := 0
		for {
			idx := strings.Index(hay[start:], sp)
			if idx < 0 {
				break
			}
			piece := hay[start : start+idx]
			out = append(out, asciiOrCounted(piece, ascii))
			start += idx + len(sp)
		}
		out = append(out, asciiOrCounted(hay[start:], ascii))
		return &Array[BStr]{elems: out}
	}
	su := s.units()
	if sep.lengthU16 == 0 {
		// An empty separator with an empty receiver is the empty array; otherwise it
		// splits into one string per code unit, so a lone surrogate becomes its own
		// one-unit piece rather than being paired.
		out := make([]BStr, len(su))
		for i, u := range su {
			out[i] = FromUTF16([]uint16{u})
		}
		return &Array[BStr]{elems: out}
	}
	pu := sep.units()
	var out []BStr
	start := 0
	for i := 0; i+len(pu) <= len(su); {
		if matchAt(su, pu, i) {
			out = append(out, FromUTF16(su[start:i]))
			i += len(pu)
			start = i
		} else {
			i++
		}
	}
	out = append(out, FromUTF16(su[start:]))
	return &Array[BStr]{elems: out}
}

// asciiOrCounted wraps a UTF-8 substring as a BStr on the fast path. When the
// caller already knows the whole source is ASCII, the code-unit length is the byte
// length and no rescan is needed; otherwise it counts the units. This is the split
// hot path's shortcut past a per-piece countUTF16Units walk.
func asciiOrCounted(piece string, ascii bool) BStr {
	if ascii {
		return BStr{utf8: piece, lengthU16: len(piece)}
	}
	return BStr{utf8: piece, lengthU16: countUTF16Units(piece)}
}

// indexOfUnits returns the first index at or after start where needle occurs in
// hay, or -1 if it does not. An empty needle matches at start, the zero-width
// match the replace methods weave a replacement into.
func indexOfUnits(hay, needle []uint16, start int) int {
	if len(needle) == 0 {
		return start
	}
	for i := start; i+len(needle) <= len(hay); i++ {
		if matchAt(hay, needle, i) {
			return i
		}
	}
	return -1
}

// appendSubstitution appends replacement to out, expanding the ECMAScript
// substitution patterns a string replace understands: $$ becomes a literal $, $&
// the matched code units, $` the code units before the match, and $' the code
// units after it. A $ before a digit or a <name> refers to a capture group,
// which a string search has none of, so it is left verbatim, as is a trailing $
// with nothing after it. pos and mlen locate the match within hay.
func appendSubstitution(out, hay []uint16, pos, mlen int, replacement []uint16) []uint16 {
	for i := 0; i < len(replacement); i++ {
		c := replacement[i]
		if c != '$' || i+1 >= len(replacement) {
			out = append(out, c)
			continue
		}
		switch replacement[i+1] {
		case '$':
			out = append(out, '$')
			i++
		case '&':
			out = append(out, hay[pos:pos+mlen]...)
			i++
		case '`':
			out = append(out, hay[:pos]...)
			i++
		case '\'':
			out = append(out, hay[pos+mlen:]...)
			i++
		default:
			// $ before a digit or a name is a capture reference a string search does
			// not fill, so the $ stays literal and the next character is emitted on
			// the following iteration as itself.
			out = append(out, c)
		}
	}
	return out
}

// appendSubstitutionBytes is the byte-wise twin of appendSubstitution for the
// UTF-8 fast path: it appends replacement to out, expanding the same ECMAScript
// patterns ($$ to a literal $, $& the matched bytes, $` the bytes before the
// match, $' the bytes after it) and leaving a $ before a digit, a name, or the
// end verbatim. hay is valid UTF-8 and pos and mlen land on code-point
// boundaries of a valid-UTF-8 match, so every copied range is itself valid UTF-8
// and the result agrees with the code-unit expansion byte for byte. The pattern
// bytes ($, &, backtick, quote) are ASCII, so a multibyte lead byte never trips
// the switch.
func appendSubstitutionBytes(out []byte, hay string, pos, mlen int, replacement string) []byte {
	for i := 0; i < len(replacement); i++ {
		c := replacement[i]
		if c != '$' || i+1 >= len(replacement) {
			out = append(out, c)
			continue
		}
		switch replacement[i+1] {
		case '$':
			out = append(out, '$')
			i++
		case '&':
			out = append(out, hay[pos:pos+mlen]...)
			i++
		case '`':
			out = append(out, hay[:pos]...)
			i++
		case '\'':
			out = append(out, hay[pos+mlen:]...)
			i++
		default:
			out = append(out, c)
		}
	}
	return out
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

// Substr returns a run of code units of a given length starting at an index,
// matching the legacy String.prototype.substr. It differs from Slice and
// Substring in taking a start and a count rather than two bounds: a negative
// start counts from the end and clamps at 0, an omitted length runs to the end
// of the string, and a negative or zero length yields the empty string. It is
// variadic so one signature covers the one- and two-argument forms, and it works
// on the code-unit view so a cut can land inside an astral pair and return a lone
// surrogate the way JavaScript does.
func (s BStr) Substr(args ...float64) BStr {
	size := s.lengthU16
	start := 0
	if len(args) >= 1 {
		n := toInteger(args[0])
		switch {
		case n < 0:
			if v := float64(size) + n; v > 0 {
				start = int(v)
			}
		case n > float64(size):
			start = size
		default:
			start = int(n)
		}
	}
	count := size - start
	if len(args) >= 2 {
		if n := toInteger(args[1]); n < 0 {
			count = 0
		} else if n < float64(count) {
			count = int(n)
		}
	}
	if count <= 0 {
		return BStr{}
	}
	return s.sub(start, start+count)
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

// isStringWhiteSpaceRune is the rune form of isStringWhiteSpace, for trimming on
// the UTF-8 fast path where the string is a sequence of runes rather than code
// units. Every ECMAScript StrWhiteSpace code point is in the Basic Multilingual
// Plane, so a rune is whitespace exactly when its low 16 bits are, and a rune
// above the BMP is never whitespace. Keeping this as a thin wrapper over the
// code-unit test means the two views can never drift apart.
func isStringWhiteSpaceRune(r rune) bool {
	return r >= 0 && r <= 0xFFFF && isStringWhiteSpace(uint16(r))
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

// Repeat returns s concatenated count times, String.prototype.repeat. count is
// coerced to an integer the way JavaScript does (a fraction truncates toward
// zero, NaN becomes 0), and a count that is negative or not finite is a
// RangeError in JavaScript, which bento surfaces as a panic because the compiled
// program has no exception machinery yet (a later slice lowers throw). The type
// checker and the lowerer only admit a non-negative integer literal count today,
// so the panic is unreachable from lowered code and guards only a direct runtime
// caller. A count of zero, or an empty receiver, yields the empty string, and a
// count of one returns the receiver unchanged. The UTF-8 fast path is kept when
// the receiver is on it, since repeating valid UTF-8 stays valid UTF-8.
func (s BStr) Repeat(count float64) BStr {
	n := toInteger(count)
	if n < 0 || math.IsInf(n, 0) {
		panic("String.prototype.repeat: count out of range")
	}
	c := int(n)
	if c == 0 || s.lengthU16 == 0 {
		return BStr{lengthU16: 0}
	}
	if c == 1 {
		return s
	}
	s = s.flat()
	if s.utf16 == nil {
		return BStr{utf8: strings.Repeat(s.utf8, c), lengthU16: s.lengthU16 * c}
	}
	out := make([]uint16, 0, s.lengthU16*c)
	for i := 0; i < c; i++ {
		out = append(out, s.utf16...)
	}
	return BStr{utf16: out, lengthU16: len(out)}
}

// Concat returns the concatenation of a and b, the lowering of `a + b` when both
// are strings. It picks the backing form once: if both sides are on the UTF-8
// fast path the result stays UTF-8 with a single byte copy, and otherwise the
// result materializes the code-unit view of each side and appends, so a lone
// surrogate on either side survives. Go's own `+` is never used on the BStr
// struct, and Go string concat is never used as the lowering, because that would
// be UTF-8 semantics on a UTF-16 string (05_type_lowering section 5).
func Concat(a, b BStr) BStr {
	// Concatenating onto the empty string is the first step of every accumulation
	// loop; return the other side untouched so it neither copies nor ropes.
	if a.lengthU16 == 0 {
		return b
	}
	if b.lengthU16 == 0 {
		return a
	}
	// A small result of two plain strings copies eagerly: the copy is cheap and the
	// result stays on a direct backing, so its next read needs no flatten. Anything
	// larger, or a side that is already a rope, defers into a rope node so a chain of
	// concatenations is linear in the final length instead of quadratic.
	if a.rope == nil && b.rope == nil && a.lengthU16+b.lengthU16 <= concatFlatMax {
		if a.utf16 == nil && b.utf16 == nil {
			return BStr{utf8: a.utf8 + b.utf8, lengthU16: a.lengthU16 + b.lengthU16}
		}
		units := make([]uint16, 0, a.lengthU16+b.lengthU16)
		units = a.appendUnits(units)
		units = b.appendUnits(units)
		return BStr{utf16: units, lengthU16: len(units)}
	}
	return BStr{rope: &ropeNode{left: a, right: b}, lengthU16: a.lengthU16 + b.lengthU16}
}

// ConcatN returns the receiver followed by every argument in order, the lowering
// of str.concat(a, b, ...). It stays on the UTF-8 fast path while the receiver and
// every argument seen so far are UTF-8, appending bytes; the first argument that
// carries a raw code-unit backing switches the whole result to the code-unit form
// so a lone surrogate survives, matching how Concat picks a backing. With no
// arguments it returns the receiver unchanged, which is what concat() with no
// arguments does.
func (s BStr) ConcatN(rest ...BStr) BStr {
	if len(rest) == 0 {
		return s
	}
	// Fast path: when the receiver and every argument are on the UTF-8 fast path
	// with no pending rope, build the whole result in one pass. A template literal
	// lowers to one ConcatN of its head, its substitutions, and its tails, so this
	// is an N-piece join that would otherwise fold pairwise through Concat and
	// allocate a fresh string on every join; one strings.Builder over all the
	// pieces allocates once instead. An accumulation loop (s += ...) does not reach
	// here, since that lowers to binary Concat, which keeps its rope so the loop
	// stays linear; ConcatN is the one-shot join, where a single flat build is the
	// cheaper shape than a rope that the next read would flatten anyway.
	allUTF8 := s.rope == nil && s.utf16 == nil
	if allUTF8 {
		for i := range rest {
			if rest[i].rope != nil || rest[i].utf16 != nil {
				allUTF8 = false
				break
			}
		}
	}
	if allUTF8 {
		total := s.lengthU16
		byteLen := len(s.utf8)
		for i := range rest {
			total += rest[i].lengthU16
			byteLen += len(rest[i].utf8)
		}
		var b strings.Builder
		b.Grow(byteLen)
		b.WriteString(s.utf8)
		for i := range rest {
			b.WriteString(rest[i].utf8)
		}
		return BStr{utf8: b.String(), lengthU16: total}
	}
	out := s
	for _, r := range rest {
		out = Concat(out, r)
	}
	return out
}

// Compare returns -1, 0, or 1 as a orders before, equal to, or after b, comparing
// code unit by code unit, which is how JavaScript's relational operators (<, <=,
// >, >=) order two strings (the Abstract Relational Comparison over the UTF-16
// views). The comparison is on code units, not code points or runes, so it matches
// JavaScript even where a Go string < would differ: an astral character sits above
// U+E000..U+FFFF in code-point order but its leading surrogate (U+D800..U+DBFF)
// orders below them, and JavaScript compares the surrogate. The shorter string
// orders first when it is a prefix of the longer.
func (a BStr) Compare(b BStr) int {
	ua, ub := a.units(), b.units()
	n := min(len(ub), len(ua))
	for i := range n {
		if ua[i] != ub[i] {
			if ua[i] < ub[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ua) < len(ub):
		return -1
	case len(ua) > len(ub):
		return 1
	default:
		return 0
	}
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
	a, b = a.flat(), b.flat()
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
	s = s.flat()
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
	s = s.flat()
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
