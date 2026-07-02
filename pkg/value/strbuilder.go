package value

import "unicode/utf16"

// StrBuilder is a reusable builder for a string the compiler assembles from a
// template literal or a chain of + whose parts include a coerced number or
// boolean. The lowerer hoists one builder per such site above the loop that runs
// it and reuses it every iteration, so the scratch buffer is allocated once and
// grows to its steady-state size, and each build costs only the one allocation of
// its result. That is the win over the straightforward lowering, which coerces
// each interpolated number to its own String(x) BStr before joining: those
// intermediate strings are gone because a number or boolean part formats straight
// into the builder's buffer.
//
// The result is an independent copy, so the builder is safe wherever the value
// goes: it can be stored, returned, or compared, and the builder is free to reuse
// its buffer on the next iteration with no aliasing to reason about. Each template
// site gets its own builder, so a template nested inside another never resets the
// buffer the outer build is still filling.
//
// The common build stays on a UTF-8 byte buffer, the fast path that keeps a whole
// template a plain byte append and lets a later .charCodeAt read a byte directly.
// A build only leaves that path when a part carries a lone surrogate the UTF-8
// view cannot hold, at which point the builder transcodes what it has to code
// units and finishes on the UTF-16 view, so the result is correct for every
// string while paying the wider representation only when the input needs it.
type StrBuilder struct {
	buf   []byte   // UTF-8 bytes while every part is UTF-8-representable
	units []uint16 // code units, used only once a part forces the surrogate view
	n     int      // UTF-16 code unit count of the build so far
	wide  bool     // true once a lone surrogate moved the build onto units
}

// Reset clears the builder for a fresh build and returns it so the caller can
// chain the appends and the terminal Done in one expression. It keeps both
// buffers' capacity, which is the point: after the first few iterations neither
// append reallocates and the build allocates nothing but its result.
func (a *StrBuilder) Reset() *StrBuilder {
	a.buf = a.buf[:0]
	a.units = a.units[:0]
	a.n = 0
	a.wide = false
	return a
}

// goWide moves the UTF-8 bytes accumulated so far onto the code-unit buffer, the
// one-time switch taken the first time a piece carries a lone surrogate. Ranging a
// byte slice converted to a string decodes without allocating (the compiler elides
// the conversion in a range), so the switch itself is a straight transcode.
func (a *StrBuilder) goWide() {
	a.units = a.units[:0]
	for _, r := range string(a.buf) {
		a.units = utf16.AppendRune(a.units, r)
	}
	a.wide = true
}

// Lit appends a compile-time literal part, the head or a between-part of a
// template or a string-literal operand, with its precomputed UTF-16 length. A
// literal part that carries a lone surrogate is passed through Units instead, so
// the byte append here is always valid UTF-8; if an earlier runtime part already
// went wide the bytes are widened to code units.
func (a *StrBuilder) Lit(s string, units int) *StrBuilder {
	if a.wide {
		for _, r := range s {
			a.units = utf16.AppendRune(a.units, r)
		}
	} else {
		a.buf = append(a.buf, s...)
	}
	a.n += units
	return a
}

// Units appends a compile-time literal part that carries a lone surrogate, the
// rare template or string part the UTF-8 view cannot hold. It forces the wide
// switch so the surrogate survives and copies the units verbatim. The lowerer
// routes a literal here only when it holds an unpaired surrogate; every other
// literal takes the cheaper byte path through Lit.
func (a *StrBuilder) Units(u []uint16) *StrBuilder {
	if !a.wide {
		a.goWide()
	}
	a.units = append(a.units, u...)
	a.n += len(u)
	return a
}

// Str appends a runtime string's code units. A string on the UTF-8 fast path is
// appended as bytes, or transcoded rune by rune when the build has already gone
// wide; a string that holds a lone surrogate forces the wide switch and copies its
// code units verbatim so the surrogate survives.
func (a *StrBuilder) Str(s BStr) *StrBuilder {
	f := s.flat()
	if f.utf16 != nil {
		if !a.wide {
			a.goWide()
		}
		a.units = append(a.units, f.utf16...)
		a.n += f.lengthU16
		return a
	}
	if a.wide {
		for _, r := range f.utf8 {
			a.units = utf16.AppendRune(a.units, r)
		}
	} else {
		a.buf = append(a.buf, f.utf8...)
	}
	a.n += f.lengthU16
	return a
}

// Num appends String(x) of a number, formatted through the same Number::toString
// the value model uses into a stack scratch. The decimal is ASCII, so it is a
// straight byte append (or a widen once wide).
func (a *StrBuilder) Num(x float64) *StrBuilder {
	var scratch [40]byte
	return a.ascii(appendNumberDecimal(scratch[:0], x))
}

// Bool appends "true" or "false", String(b) of a boolean.
func (a *StrBuilder) Bool(b bool) *StrBuilder {
	if b {
		return a.ascii(trueBytes)
	}
	return a.ascii(falseBytes)
}

var (
	trueBytes  = []byte("true")
	falseBytes = []byte("false")
)

// ascii appends an all-ASCII byte run (a formatted number or a boolean word),
// where one byte is one code unit, taking the byte path or the widen by the same
// rule as the other appends.
func (a *StrBuilder) ascii(b []byte) *StrBuilder {
	if a.wide {
		for _, c := range b {
			a.units = append(a.units, uint16(c))
		}
	} else {
		a.buf = append(a.buf, b...)
	}
	a.n += len(b)
	return a
}

// Done returns the built string as an independent BStr, copied out of the
// builder's buffer so the builder can safely reuse that buffer on the next build.
// A build that never went wide returns the UTF-8 view, so a later .charCodeAt on
// an all-ASCII result reads a byte with no code-unit materialization; a wide build
// returns the code-unit view through FromUTF16, which makes the owning copy.
func (a *StrBuilder) Done() BStr {
	if a.wide {
		return FromUTF16(a.units)
	}
	return BStr{utf8: string(a.buf), lengthU16: a.n}
}
