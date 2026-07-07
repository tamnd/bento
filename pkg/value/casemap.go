package value

import (
	"strings"
	"unicode"
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
//
// The one place x/text disagrees with JavaScript is the Final_Sigma before-context.
// A capital sigma lowercases to the final form "ς" only when a cased letter, then
// zero or more case-ignorable characters, sits before it, and no cased letter (past
// any case-ignorable) follows. U+0345 COMBINING GREEK YPOGEGRAMMENI is both cased
// and case-ignorable, and the algorithm consumes it as case-ignorable in the
// backward scan, so "ͅΣ" has no cased letter before the sigma and lowercases
// to "σ". x/text instead treats the leading U+0345 as the cased letter and gives
// the final "ς", so lowercasing runs the sigma decision itself and delegates the
// rest of each run to x/text.

// ToUpperCase returns the string with every character replaced by its full
// uppercase mapping (String.prototype.toUpperCase).
func (s BStr) ToUpperCase() BStr {
	caser := cases.Upper(language.Und)
	return s.caseMap(caser.String)
}

// ToLowerCase returns the string with every character replaced by its full
// lowercase mapping, including the Final_Sigma context
// (String.prototype.toLowerCase).
func (s BStr) ToLowerCase() BStr {
	caser := cases.Lower(language.Und)
	return s.caseMap(func(run string) string { return lowerRun(caser, run) })
}

// caseMap applies a per-run mapping to the string. On the UTF-8 fast path the whole
// string maps in one call; on the code-unit backing it maps the valid runs and
// passes any lone surrogate through, so a lone surrogate survives where transcoding
// the whole string to UTF-8 first would replace it. The mapping closes over a fresh
// caser per call because a cases.Caser may not be used concurrently.
func (s BStr) caseMap(mapRun func(string) string) BStr {
	s = s.flat()
	if s.utf16 == nil {
		return FromGoString(mapRun(s.utf8))
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
		mapped := mapRun(string(utf16.Decode(units[start:i])))
		out = append(out, utf16.Encode([]rune(mapped))...)
	}
	return FromUTF16(out)
}

// lowerRun lowercases one run of valid text with the correct Final_Sigma decision.
// A run holds no lone surrogate, so its ends are context breaks the way the whole
// string's ends are, and the sigma decision computed within the run matches the
// decision over the whole string. Everything but the capital sigma is left to
// x/text: the run is split at each U+03A3, the pieces between are lowercased by the
// caser, and each sigma is mapped to the final or non-final form by finalSigma.
func lowerRun(caser cases.Caser, run string) string {
	if !strings.ContainsRune(run, capitalSigma) {
		return caser.String(run)
	}
	runes := []rune(run)
	var b strings.Builder
	b.Grow(len(run))
	seg := 0
	for i, r := range runes {
		if r != capitalSigma {
			continue
		}
		if i > seg {
			b.WriteString(caser.String(string(runes[seg:i])))
		}
		if finalSigma(runes, i) {
			b.WriteRune(finalSmallSigma)
		} else {
			b.WriteRune(smallSigma)
		}
		seg = i + 1
	}
	if seg < len(runes) {
		b.WriteString(caser.String(string(runes[seg:])))
	}
	return b.String()
}

const (
	capitalSigma    = 'Σ' // Σ
	smallSigma      = 'σ' // σ
	finalSmallSigma = 'ς' // ς
)

// finalSigma reports whether the capital sigma at index i lowercases to the final
// form. The before-context holds when a cased letter, then zero or more
// case-ignorable characters, sits immediately before the sigma; the after-context
// holds when no cased letter, past zero or more case-ignorable characters, follows.
// The final form applies exactly when the before-context holds and the after-context
// does not name a following cased letter, which is the Unicode Final_Sigma rule.
func finalSigma(runes []rune, i int) bool {
	before := false
	for j := i - 1; j >= 0; j-- {
		if isCaseIgnorable(runes[j]) {
			continue
		}
		before = isCased(runes[j])
		break
	}
	if !before {
		return false
	}
	for j := i + 1; j < len(runes); j++ {
		if isCaseIgnorable(runes[j]) {
			continue
		}
		return !isCased(runes[j])
	}
	return true
}

// isCased reports whether a rune is a cased character, the Unicode derived property
// that is Lowercase, Uppercase, or a titlecase letter. Go's IsLower and IsUpper
// carry the letter categories, IsTitle carries Lt, and the two Other_ tables carry
// the cased characters outside those categories such as the roman numerals.
func isCased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) ||
		unicode.Is(unicode.Other_Uppercase, r) || unicode.Is(unicode.Other_Lowercase, r)
}

// isCaseIgnorable reports whether a rune is case-ignorable, the Unicode derived
// property a case-mapping context scans past. It is the marks and format and
// modifier categories Mn, Me, Cf, Lm, and Sk, plus the handful of punctuation the
// Word_Break property tags MidLetter, MidNumLet, or Single_Quote, which Go's unicode
// package does not expose and so is listed here.
func isCaseIgnorable(r rune) bool {
	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk) {
		return true
	}
	return wordBreakMid[r]
}

// wordBreakMid is the Word_Break MidLetter, MidNumLet, and Single_Quote set, the
// punctuation that counts as case-ignorable but carries no general category that
// would place it there. The values come from Unicode's WordBreakProperty.txt.
var wordBreakMid = map[rune]bool{
	0x0027: true, // ' Single_Quote
	0x002E: true, // . MidNumLet
	0x003A: true, // : MidLetter
	0x00B7: true, // · MidLetter
	0x0387: true, // · MidLetter
	0x05F4: true, // ״ MidLetter
	0x2018: true, // ' MidNumLet
	0x2019: true, // ' MidNumLet
	0x2024: true, // ․ MidNumLet
	0x2027: true, // ‧ MidLetter
	0xFE13: true, // ﹓ MidLetter
	0xFE52: true, // ﹒ MidNumLet
	0xFE55: true, // ﹕ MidLetter
	0xFF07: true, // ＇ MidNumLet
	0xFF0E: true, // ． MidNumLet
	0xFF1A: true, // ： MidLetter
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
