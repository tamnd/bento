// This file owns JSON.parse for the compiled side: reading JSON text into a boxed
// dynamic Value tree, the inverse of the JSONStringify walk. The result is
// dynamic because JSON.parse returns any: the shape of the parsed value is not
// known until runtime, so it lands in the boxed Value world the rest of this file
// group defines.
//
// The grammar is the JSON grammar from the specification (the same one V8
// accepts), parsed by recursive descent over the string's UTF-16 code units. A
// malformed input throws a SyntaxError, the error JSON.parse raises: a value that
// does not parse, or trailing content after the one top-level value. Well-formed
// input, which is what a stringify round-trip produces, parses exactly.

package value

import (
	"math"
	"unicode/utf16"
)

// JSONParse reads a JSON document from s and returns the boxed value it denotes,
// the value model's JSON.parse. It parses the one top-level value and requires
// only whitespace after it; a value that does not parse, or any non-whitespace
// content after it, throws the SyntaxError JavaScript raises on malformed JSON.
func JSONParse(s BStr) Value {
	p := &jsonParser{src: s.units()}
	p.skipSpace()
	v, ok := p.parseValue()
	if !ok {
		Throw(NewSyntaxError(FromGoString("Unexpected token in JSON")))
	}
	p.skipSpace()
	if p.pos != len(p.src) {
		Throw(NewSyntaxError(FromGoString("Unexpected non-whitespace character after JSON data")))
	}
	return v
}

// jsonParser holds the UTF-16 code units and the read cursor. Parsing over units
// rather than bytes keeps a string value's escapes and any non-ASCII content
// exact, the same code-unit view the serializer writes.
type jsonParser struct {
	src []uint16
	pos int
}

// parseValue parses one JSON value at the cursor, dispatching on the first
// non-space character. It reports ok == false on a malformed value so the whole
// parse fails cleanly rather than returning a partial tree.
func (p *jsonParser) parseValue() (Value, bool) {
	if p.pos >= len(p.src) {
		return Undefined, false
	}
	switch c := p.src[p.pos]; {
	case c == '{':
		return p.parseObject()
	case c == '[':
		return p.parseArray()
	case c == '"':
		s, ok := p.parseString()
		if !ok {
			return Undefined, false
		}
		return StringValue(s), true
	case c == 't':
		return p.parseLiteral("true", True)
	case c == 'f':
		return p.parseLiteral("false", False)
	case c == 'n':
		return p.parseLiteral("null", Null)
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	default:
		return Undefined, false
	}
}

// parseObject parses a brace-delimited object, reading key-string colon value
// pairs separated by commas, and builds the value in insertion order so the
// parsed object enumerates the way the source wrote it.
func (p *jsonParser) parseObject() (Value, bool) {
	p.pos++ // consume '{'
	obj := NewObject()
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == '}' {
		p.pos++
		return obj, true
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != '"' {
			return Undefined, false
		}
		key, ok := p.parseString()
		if !ok {
			return Undefined, false
		}
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != ':' {
			return Undefined, false
		}
		p.pos++ // consume ':'
		p.skipSpace()
		val, ok := p.parseValue()
		if !ok {
			return Undefined, false
		}
		obj.Set(key, val)
		p.skipSpace()
		if p.pos >= len(p.src) {
			return Undefined, false
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return obj, true
		default:
			return Undefined, false
		}
	}
}

// parseArray parses a bracket-delimited array, reading comma-separated values into
// dense element storage in order.
func (p *jsonParser) parseArray() (Value, bool) {
	p.pos++ // consume '['
	var elems []Value
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == ']' {
		p.pos++
		return NewArrayValue(elems), true
	}
	for {
		p.skipSpace()
		val, ok := p.parseValue()
		if !ok {
			return Undefined, false
		}
		elems = append(elems, val)
		p.skipSpace()
		if p.pos >= len(p.src) {
			return Undefined, false
		}
		switch p.src[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return NewArrayValue(elems), true
		default:
			return Undefined, false
		}
	}
}

// parseString parses a double-quoted JSON string starting at the cursor, decoding
// the two-character escapes and the \u four-hex-digit escape into code units. A
// surrogate escape is kept as its raw unit, so a valid pair round-trips and a lone
// surrogate survives, matching what the serializer emitted.
func (p *jsonParser) parseString() (BStr, bool) {
	p.pos++ // consume opening quote
	var out []uint16
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == '"':
			p.pos++
			return FromUTF16(out), true
		case c == '\\':
			p.pos++
			if p.pos >= len(p.src) {
				return BStr{}, false
			}
			switch e := p.src[p.pos]; e {
			case '"', '\\', '/':
				out = append(out, e)
			case 'b':
				out = append(out, 0x08)
			case 'f':
				out = append(out, 0x0C)
			case 'n':
				out = append(out, 0x0A)
			case 'r':
				out = append(out, 0x0D)
			case 't':
				out = append(out, 0x09)
			case 'u':
				u, ok := p.parseHex4()
				if !ok {
					return BStr{}, false
				}
				out = append(out, u)
				continue
			default:
				return BStr{}, false
			}
			p.pos++
		case c < 0x20:
			// An unescaped control character is not legal in a JSON string.
			return BStr{}, false
		default:
			out = append(out, c)
			p.pos++
		}
	}
	return BStr{}, false
}

// parseHex4 reads the four hex digits of a \u escape and advances past them,
// returning the code unit they spell.
func (p *jsonParser) parseHex4() (uint16, bool) {
	p.pos++ // consume 'u'
	if p.pos+4 > len(p.src) {
		return 0, false
	}
	var u uint16
	for i := 0; i < 4; i++ {
		d, ok := hexDigit(p.src[p.pos+i])
		if !ok {
			return 0, false
		}
		u = u<<4 | uint16(d)
	}
	p.pos += 4
	return u, true
}

// parseNumber reads a JSON number and boxes the double it denotes. It spans the
// sign, integer, fraction, and exponent the grammar allows, then decodes the run
// through the same StringToNumber the language's own number coercion uses, so the
// parsed double is bit-for-bit what the engine produces.
func (p *jsonParser) parseNumber() (Value, bool) {
	start := p.pos
	if p.pos < len(p.src) && p.src[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.src) && isNumberByte(p.src[p.pos]) {
		p.pos++
	}
	run := p.src[start:p.pos]
	f, ok := parseJSONNumber(run)
	if !ok {
		return Undefined, false
	}
	return Number(f), true
}

// parseLiteral matches one of the three keyword literals and returns its boxed
// value, failing if the word at the cursor is not exactly the keyword.
func (p *jsonParser) parseLiteral(word string, v Value) (Value, bool) {
	if p.pos+len(word) > len(p.src) {
		return Undefined, false
	}
	for i := 0; i < len(word); i++ {
		if p.src[p.pos+i] != uint16(word[i]) {
			return Undefined, false
		}
	}
	p.pos += len(word)
	return v, true
}

// skipSpace advances past the JSON whitespace characters: space, tab, newline,
// and carriage return.
func (p *jsonParser) skipSpace() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

// isNumberByte reports whether a code unit can appear in a JSON number body after
// the optional leading sign, so parseNumber can span the run before validating it.
func isNumberByte(u uint16) bool {
	return (u >= '0' && u <= '9') || u == '.' || u == 'e' || u == 'E' || u == '+' || u == '-'
}

// hexDigit decodes a single hexadecimal digit code unit.
func hexDigit(u uint16) (int, bool) {
	switch {
	case u >= '0' && u <= '9':
		return int(u - '0'), true
	case u >= 'a' && u <= 'f':
		return int(u-'a') + 10, true
	case u >= 'A' && u <= 'F':
		return int(u-'A') + 10, true
	default:
		return 0, false
	}
}

// parseJSONNumber decodes a JSON number run to a float64. The run is ASCII by
// construction, so it decodes as the UTF-8 string of the units and reuses the
// language's StringToNumber, which is the same shortest-double parse the number
// coercion runs. An empty or malformed run reports false.
func parseJSONNumber(run []uint16) (float64, bool) {
	if len(run) == 0 {
		return math.NaN(), false
	}
	s := string(utf16.Decode(run))
	f := StringToNumber(FromGoString(s))
	if math.IsNaN(f) && s != "NaN" {
		return math.NaN(), false
	}
	return f, true
}
