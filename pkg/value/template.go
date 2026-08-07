// This file owns the template strings object a tagged template hands its tag, and
// String.raw, the one built-in tag the language ships. Both are built over the same
// two arrays, the cooked strings and the raw ones, so the object a call site holds
// and the built-in that reads it cannot disagree about which array is which.

package value

// TemplateObject builds the template strings object a tagged template passes as its
// first argument: an array of the template's cooked literal parts carrying a raw
// property that holds the same parts undecoded. Both arrays are frozen and so is the
// object, which is what the language specifies, so a tag that writes to either drops
// the write rather than change what the next call sees.
//
// The identity of this object belongs to the call site rather than to the call: the
// spec keys its template registry on the parse node, so every evaluation of one
// tagged template hands the tag the same object, and two sites that spell the same
// text hand out two different ones. The compiler gets that for free by emitting one
// package-level var per site and initializing it here at init.
func TemplateObject(cooked, raw []Value) Value {
	obj := NewArrayValue(cooked)
	obj.Set(FromGoString("raw"), NewArrayValue(raw).Freeze())
	return obj.Freeze()
}

// StringRaw is String.raw, the built-in tag that answers the template's raw text with
// the substitutions spliced in, so the escapes the cooked strings resolved stay as
// they were written. It reads the raw array off whatever object it is given rather
// than off a template object specifically, which is what lets String.raw({ raw: parts
// }, ...) work on a hand-built object, the spelling a tag reaches for when it wants
// the built-in to do the splicing.
//
// The walk is the spec's: one raw segment, then one substitution, until the last
// segment, which is not followed by one. A substitution the caller did not pass
// contributes nothing, so a call with fewer substitutions than gaps runs the
// segments together rather than write undefined between them.
//
// The result is a BStr rather than a boxed value because String.raw's return type is
// string whatever it is given, so the emitted expression sits in a static string slot
// and boxes only where the surrounding lowering decides it should.
func StringRaw(strings Value, subs ...Value) BStr {
	raw := strings.Get(FromGoString("raw"))
	n := arrayLikeLen(raw)
	if n == 0 {
		return FromGoString("")
	}
	parts := make([]BStr, 0, 2*n)
	for i := 0; i < n; i++ {
		parts = append(parts, ToString(raw.GetIndex(float64(i))))
		if i+1 == n || i >= len(subs) {
			continue
		}
		parts = append(parts, ToString(subs[i]))
	}
	return parts[0].ConcatN(parts[1:]...)
}

// StringRawArgs is String.raw with its substitutions gathered in one array-like value
// rather than spelled out one by one, which is the shape String.raw(o, ...vals) takes
// when vals is a rest parameter: the count is a run-time fact, so there is no fixed
// argument list for the call to write. It reads the substitutions off the array the
// same way the spread would have supplied them and hands them to StringRaw, so the two
// spellings answer the same string.
func StringRawArgs(strings, subs Value) BStr {
	n := arrayLikeLen(subs)
	list := make([]Value, n)
	for i := range list {
		list[i] = subs.GetIndex(float64(i))
	}
	return StringRaw(strings, list...)
}
