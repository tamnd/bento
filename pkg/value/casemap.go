package value

import (
	"unicode/utf16"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// This file implements String.prototype.toUpperCase and toLowerCase. JavaScript
// defines them as the locale-independent full Unicode case mapping, which is more
// than Go's strings.ToUpper/ToLower: those use the simple one-to-one mapping and
// so leave "ß" alone where JavaScript uppercases it to "SS", and they do not apply
// the Final_Sigma context that lowercases a word-final capital sigma to "ς" rather
// than "σ". golang.org/x/text/cases with the undetermined language (language.Und)
// is exactly this locale-independent full mapping, verified against the engine by
// the equivalence tests, so the mapping delegates to it.
//
// The one wrinkle is the lone surrogate a BStr can hold. x/text works on UTF-8,
// which cannot carry a lone surrogate, so a string on the code-unit backing is
// case-mapped in runs: each maximal run of valid text is decoded to UTF-8, mapped,
// and re-encoded to code units, and each lone surrogate is passed through
// unchanged. A lone surrogate is uncased and is not case-ignorable, so it breaks
// the Final_Sigma context exactly the way a run boundary does, which makes the
// run-by-run mapping give the same result the whole string would.

// ToUpperCase returns the string with every character replaced by its full
// uppercase mapping (String.prototype.toUpperCase).
func (s BStr) ToUpperCase() BStr {
	return s.caseMap(cases.Upper(language.Und))
}

// ToLowerCase returns the string with every character replaced by its full
// lowercase mapping, including the Final_Sigma context
// (String.prototype.toLowerCase).
func (s BStr) ToLowerCase() BStr {
	return s.caseMap(cases.Lower(language.Und))
}

// caseMap applies a caser to the string. On the UTF-8 fast path the whole string
// maps in one call; on the code-unit backing it maps the valid runs and passes any
// lone surrogate through, so a lone surrogate survives where transcoding the whole
// string to UTF-8 first would replace it. A fresh caser is passed in per call
// because a cases.Caser may not be used concurrently.
func (s BStr) caseMap(caser cases.Caser) BStr {
	if s.utf16 == nil {
		return FromGoString(caser.String(s.utf8))
	}
	units := s.utf16
	out := make([]uint16, 0, len(units))
	i := 0
	for i < len(units) {
		if isLoneSurrogate(units, i) {
			out = append(out, units[i])
			i++
			continue
		}
		start := i
		for i < len(units) && !isLoneSurrogate(units, i) {
			if isHighSurrogate(units[i]) {
				i += 2 // a valid pair, consumed together
			} else {
				i++
			}
		}
		mapped := caser.String(string(utf16.Decode(units[start:i])))
		out = append(out, utf16.Encode([]rune(mapped))...)
	}
	return FromUTF16(out)
}

// isLoneSurrogate reports whether the code unit at i is a surrogate with no valid
// partner: a high surrogate not followed by a low one, or a low surrogate (which,
// since a valid pair is always consumed as a unit, only appears here when it has no
// preceding high half).
func isLoneSurrogate(units []uint16, i int) bool {
	u := units[i]
	if isHighSurrogate(u) {
		return i+1 >= len(units) || !isLowSurrogate(units[i+1])
	}
	return isLowSurrogate(u)
}

func isHighSurrogate(u uint16) bool { return u >= 0xD800 && u <= 0xDBFF }
func isLowSurrogate(u uint16) bool  { return u >= 0xDC00 && u <= 0xDFFF }
