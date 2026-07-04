// This file owns String.prototype.normalize, the Unicode normalization method
// (08_string). Normalization rewrites a string into one of the four canonical or
// compatibility forms so that two strings that a reader would call equal compare
// equal code point for code point. Go's normalization lives in
// golang.org/x/text/unicode/norm and works on UTF-8, while a bento string is
// UTF-16, so the method crosses to UTF-8, normalizes, and crosses back.

package value

import (
	"unicode/utf16"

	"golang.org/x/text/unicode/norm"
)

// normForm maps a normalization form name to the Go normalizer, or reports that
// the name is not one of the four the specification allows. A bad name is a
// RangeError at the call site, so the lookup returns the failure rather than
// panicking here.
func normForm(name string) (norm.Form, bool) {
	switch name {
	case "NFC":
		return norm.NFC, true
	case "NFD":
		return norm.NFD, true
	case "NFKC":
		return norm.NFKC, true
	case "NFKD":
		return norm.NFKD, true
	}
	return 0, false
}

// Normalize returns the string in the requested Unicode normalization form,
// matching String.prototype.normalize. The form defaults to NFC when the argument
// is omitted, and a form that is not one of NFC, NFD, NFKC, or NFKD is a
// RangeError, the same throw the method raises in the engine.
//
// A well-formed string crosses to UTF-8, normalizes, and crosses back in one step.
// A string that holds a lone surrogate cannot round-trip through UTF-8, since the
// crossing would replace the surrogate with U+FFFD, so it takes a slower path that
// normalizes each maximal well-formed run and keeps every lone surrogate between
// the runs untouched. That is faithful because a lone surrogate is a starter with
// combining class zero, so it always sits on a normalization boundary and never
// combines with the text around it.
func (s BStr) Normalize(form ...BStr) BStr {
	name := "NFC"
	if len(form) > 0 {
		name = form[0].ToGoString()
	}
	f, ok := normForm(name)
	if !ok {
		Throw(NewRangeError(FromGoString("The normalization form should be one of NFC, NFD, NFKC, NFKD.")))
	}
	if s.IsWellFormed() {
		return FromGoString(f.String(s.ToGoString()))
	}
	units := s.units()
	out := make([]uint16, 0, len(units))
	run := make([]uint16, 0, len(units))
	flush := func() {
		if len(run) == 0 {
			return
		}
		out = append(out, utf16.Encode([]rune(f.String(string(utf16.Decode(run)))))...)
		run = run[:0]
	}
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u < 0xD800 || u > 0xDFFF:
			run = append(run, u)
		case u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			run = append(run, u, units[i+1])
			i++
		default:
			flush()
			out = append(out, u)
		}
	}
	flush()
	return FromUTF16(out)
}
