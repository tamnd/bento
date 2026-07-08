// This file owns JSON.stringify for the compiled side: turning a lowered value
// (a BStr, a number, a boolean, a *Array, or a generated object struct) into the
// exact text V8 produces, byte for byte, so a stringify in bento and the same
// stringify in Node print the same string (10_value_model, JSON serialization).
//
// The encoder is a reflection walk rather than a method on each type, because the
// value it serializes is statically typed at the call site but heterogeneous
// across the tree: a record holds numbers, strings, booleans, arrays, and nested
// objects, and JSON.stringify(x) sees all of them through one dynamic edge. The
// walk stays in this package, so it can read a *Array's elements through an
// unexported hook and a BStr's code units directly, neither of which crosses the
// package boundary.

package value

import (
	"reflect"
	"strings"
)

// jsonArray is the hook a *Array exposes to the encoder so the walk can read its
// elements without reaching through the unexported backing slice by reflection.
// It is unexported, so only this package implements and consumes it, which keeps
// the array's internals private while still letting the serializer see them.
type jsonArray interface {
	jsonElements() []any
}

// jsonArmer is the hook a generated tagged-sum union exposes so the encoder reads
// its active member rather than reflecting the struct's unexported arm fields into
// an empty object. The method is exported because the generated union type lives in
// the compiled program's own package, so an unexported method there could not
// satisfy this interface; JSONArm returns the arm the tag selects, boxed as any, and
// the walk recurses into it as the value the union holds.
type jsonArmer interface {
	JSONArm() any
}

// jsonElements returns the array's elements boxed as a slice of any, in order, so
// the JSON walk can recurse into each without knowing the element type. The box
// is the same value the element already is (a BStr, a float64, a struct), so no
// conversion happens here beyond widening to the interface.
func (a *Array[T]) jsonElements() []any {
	out := make([]any, len(a.elems))
	for i, e := range a.elems {
		out[i] = e
	}
	return out
}

// JSONStringify serializes a value to the text JSON.stringify produces, with no
// indentation and keys in insertion order, matching V8 exactly. It is the top of
// the reflection walk: the argument arrives boxed as any because the call site is
// the one dynamic edge in an otherwise statically typed program, and the walk
// dispatches on the concrete type from there.
func JSONStringify(v any) BStr {
	var b strings.Builder
	encodeJSON(&b, v)
	return FromGoString(b.String())
}

// encodeJSON writes one value's JSON text to b. It dispatches on the concrete Go
// type the lowering produces for each JavaScript type: a BStr is a quoted string,
// a float64 is a JavaScript number, a bool is true or false, a *Array is a
// bracketed list, and anything else is a generated object struct walked by
// reflection. The order matches how the compiler lowers each JavaScript value, so
// every shape the AOT path can build has exactly one arm here.
func encodeJSON(b *strings.Builder, v any) {
	switch x := v.(type) {
	case BStr:
		encodeJSONString(b, x)
	case float64:
		// A finite number formats through the same shortest round-trip the Number
		// to string coercion uses, so JSON and String(x) agree. JSON.stringify writes
		// a non-finite number (NaN, Infinity) as null; NumberToString would spell it
		// "NaN", so that case is handled here before delegating.
		if x != x || x > maxFinite || x < -maxFinite {
			b.WriteString("null")
			return
		}
		b.WriteString(NumberToString(x).ToGoString())
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case jsonArray:
		b.WriteByte('[')
		for i, e := range x.jsonElements() {
			if i > 0 {
				b.WriteByte(',')
			}
			if jsonUndefinedGo(e) {
				// A function element has no JSON form; an array renders such an element
				// as null the way SerializeJSONProperty does, not by dropping it.
				b.WriteString("null")
				continue
			}
			encodeJSON(b, e)
		}
		b.WriteByte(']')
	case Value:
		encodeBoxedJSON(b, x)
	case jsonArmer:
		// A tagged-sum union carries its value in whichever arm its tag selects, and
		// the arm fields are unexported machinery, so reflecting the struct would write
		// an empty object. The generated type hands back its active arm here, and the
		// walk serializes that member as the value the union holds.
		encodeJSON(b, x.JSONArm())
	default:
		if jsonUndefinedGo(v) {
			// A top-level function has no JSON form, so JSON.stringify(function(){})
			// is undefined, which the typed call site models as the string being
			// absent; nothing is written and no reflection is attempted, so a func
			// value does not reach NumField and panic.
			return
		}
		encodeJSONObject(b, reflect.ValueOf(v))
	}
}

// jsonUndefinedGo reports whether a statically typed Go value serializes as
// JSON-undefined, the counterpart to jsonUndefinedValue on the boxed side. The AOT
// path builds a function value as a Go func, which has no JSON form: an array folds
// it to null, an object omits its key, and a top-level function stringifies to
// undefined. A nil interface has no value to serialize and is treated the same.
func jsonUndefinedGo(v any) bool {
	if v == nil {
		return true
	}
	return reflect.ValueOf(v).Kind() == reflect.Func
}

// encodeBoxedJSON writes a boxed dynamic Value as JSON text, the JSON.stringify
// path for a value whose shape is only known at runtime, which for the AOT path is
// a JSON.parse result flowing back through a stringify. It dispatches on the
// runtime kind the way the static walk dispatches on the Go type, and it follows
// the specification's SerializeJSONProperty: a string, number, boolean, and null
// render as themselves, an array folds a JSON-undefined element to null, and an
// object serializes its own properties in insertion order with a JSON-undefined
// value omitting the key. A number that is not finite renders as null, matching the
// static number arm.
func encodeBoxedJSON(b *strings.Builder, v Value) {
	switch v.kind {
	case KindNull:
		b.WriteString("null")
	case KindBool:
		if v.AsBool() {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case KindNumber:
		f := v.AsNumber()
		if f != f || f > maxFinite || f < -maxFinite {
			b.WriteString("null")
			return
		}
		b.WriteString(NumberToString(f).ToGoString())
	case KindString:
		encodeJSONString(b, v.str())
	case KindArray:
		o := v.object()
		b.WriteByte('[')
		for i, e := range o.elems {
			if i > 0 {
				b.WriteByte(',')
			}
			if jsonUndefinedValue(e) {
				b.WriteString("null")
				continue
			}
			encodeBoxedJSON(b, e)
		}
		b.WriteByte(']')
	case KindObject:
		o := v.object()
		b.WriteByte('{')
		first := true
		for i := range o.keys {
			val := o.vals[i]
			if jsonUndefinedValue(val) {
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			encodeJSONString(b, o.keys[i])
			b.WriteByte(':')
			encodeBoxedJSON(b, val)
		}
		b.WriteByte('}')
	default:
		// undefined, symbol, and function have no JSON form; a top-level such value
		// stringifies to undefined, which the typed call site models as the string
		// being absent, so nothing is written here.
	}
}

// jsonUndefinedValue reports whether a boxed value serializes as JSON-undefined,
// the values SerializeJSONProperty drops: undefined itself, a function, and a
// symbol. An array turns these into null and an object omits the property.
func jsonUndefinedValue(v Value) bool {
	return v.kind == KindUndefined || v.kind == KindFunc || v.kind == KindSymbol
}

// encodeJSONObject writes a generated object struct as a JSON object. The keys
// come from each field's json struct tag, which carries the original JavaScript
// property name the lowering stamped on the field, so the serialized key is the
// source key and not the capitalized Go field name. Fields are visited in
// definition order, which the compiler emits in source order, so the object's
// keys come out in insertion order the way JavaScript's do.
func encodeJSONObject(b *strings.Builder, rv reflect.Value) {
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	b.WriteByte('{')
	first := true
	encodeJSONFields(b, rv, &first)
	b.WriteByte('}')
}

// encodeJSONFields writes one struct's fields as JSON members, sharing the
// comma state with its caller so an embedded struct's fields interleave with
// the outer struct's as one flat object. A derived class embeds its base, and
// in JavaScript every inherited field is an own property of the instance (the
// base constructor assigns onto this), so the base's fields flatten into the
// same object, in base-first order because the embedded field sits first. An
// unexported field is compiler machinery (the vtable pointer), not a source
// property, and is skipped.
func encodeJSONFields(b *strings.Builder, rv reflect.Value, first *bool) {
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if f.Anonymous {
			encodeJSONFields(b, rv.Field(i), first)
			continue
		}
		val := rv.Field(i).Interface()
		if jsonUndefinedGo(val) {
			// A function-valued property has no JSON form, so the object omits the
			// key rather than reflecting the func and faulting on NumField.
			continue
		}
		key := f.Tag.Get("json")
		if key == "" {
			key = f.Name
		}
		if !*first {
			b.WriteByte(',')
		}
		*first = false
		encodeJSONString(b, FromGoString(key))
		b.WriteByte(':')
		encodeJSON(b, val)
	}
}

// encodeJSONString writes a JavaScript string as a JSON string literal, escaping
// exactly what the specification's well-formed JSON.stringify does and nothing
// more. It walks the UTF-16 code units, not the UTF-8 bytes, so an unpaired
// surrogate is escaped as a \u sequence (well-formed stringify, ES2019) while a
// valid surrogate pair is written as the character it encodes. It deliberately
// does not escape <, >, or & the way Go's encoding/json does, because V8 leaves
// them literal, and matching V8 is the whole point.
func encodeJSONString(b *strings.Builder, s BStr) {
	units := s.units()
	b.WriteByte('"')
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u == '"':
			b.WriteString(`\"`)
		case u == '\\':
			b.WriteString(`\\`)
		case u == 0x08:
			b.WriteString(`\b`)
		case u == 0x09:
			b.WriteString(`\t`)
		case u == 0x0A:
			b.WriteString(`\n`)
		case u == 0x0C:
			b.WriteString(`\f`)
		case u == 0x0D:
			b.WriteString(`\r`)
		case u < 0x20:
			b.WriteString(`\u`)
			writeHex4(b, u)
		case u >= 0xD800 && u <= 0xDBFF:
			if i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF {
				b.WriteRune(decodeSurrogatePair(u, units[i+1]))
				i++
			} else {
				b.WriteString(`\u`)
				writeHex4(b, u)
			}
		case u >= 0xDC00 && u <= 0xDFFF:
			b.WriteString(`\u`)
			writeHex4(b, u)
		default:
			b.WriteRune(rune(u))
		}
	}
	b.WriteByte('"')
}

// decodeSurrogatePair combines a high and low UTF-16 surrogate into the code
// point they encode, the inverse of the pair split, so a character outside the
// Basic Multilingual Plane is written as itself rather than as two escapes.
func decodeSurrogatePair(hi, lo uint16) rune {
	return ((rune(hi) - 0xD800) << 10) + (rune(lo) - 0xDC00) + 0x10000
}

// writeHex4 writes a code unit as four lowercase hexadecimal digits, the form a
// \u escape takes. V8 emits lowercase here, so the digits are lowercase to match.
func writeHex4(b *strings.Builder, u uint16) {
	const hex = "0123456789abcdef"
	b.WriteByte(hex[(u>>12)&0xF])
	b.WriteByte(hex[(u>>8)&0xF])
	b.WriteByte(hex[(u>>4)&0xF])
	b.WriteByte(hex[u&0xF])
}

// maxFinite is the largest finite float64 magnitude, the boundary JSON.stringify
// uses to fold a non-finite number to null. It is math.MaxFloat64 spelled without
// importing math for a single constant.
const maxFinite = 1.7976931348623157e308
