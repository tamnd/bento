package frontend

import "strings"

// decodeIdentEscapes turns the source spelling of an identifier into the name it
// denotes. ECMAScript lets an identifier carry unicode escapes, so the source
// `abc` and the source `abc` are the same name, and a program is free to
// declare a binding one way and read it the other. Lowering mangles a name into
// a Go identifier and relies on a declaration and every reference agreeing on
// the mangled spelling, so the escaped and unescaped forms have to collapse to
// one string before mangling. GetTextOfNode hands back the raw source, escapes
// and all; this decodes the two identifier escape forms, `\uHHHH` and `\u{H+}`,
// to the runes they name and passes everything else through untouched.
//
// The input is trusted to be a well-formed identifier the checker already
// accepted, so a `\u` that is not followed by a valid escape body keeps its
// backslash verbatim rather than erroring; a malformed escape cannot reach here
// from parsed source. Callers guard on a backslash being present, so the common
// escape-free identifier never enters this function.
func decodeIdentEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) || s[i+1] != 'u' {
			b.WriteByte(s[i])
			i++
			continue
		}
		// Past the "\u": either "{H+}" or exactly four hex digits.
		j := i + 2
		if j < len(s) && s[j] == '{' {
			k := j + 1
			for k < len(s) && s[k] != '}' {
				k++
			}
			if k < len(s) && k > j+1 {
				if r, ok := parseHexRune(s[j+1 : k]); ok {
					b.WriteRune(r)
					i = k + 1
					continue
				}
			}
		} else if j+4 <= len(s) {
			if r, ok := parseHexRune(s[j : j+4]); ok {
				b.WriteRune(r)
				i = j + 4
				continue
			}
		}
		// Not a well-formed escape; keep the backslash and move on.
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// parseHexRune reads a run of hex digits as a code point. It rejects an empty
// run, a non-hex digit, or a value past the Unicode maximum, so a malformed
// escape body falls back to a verbatim copy at the call site.
func parseHexRune(h string) (rune, bool) {
	if h == "" {
		return 0, false
	}
	var v int
	for i := 0; i < len(h); i++ {
		c := h[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return 0, false
		}
		v = v*16 + d
		if v > 0x10FFFF {
			return 0, false
		}
	}
	return rune(v), true
}
