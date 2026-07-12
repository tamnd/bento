// This file owns the indented form of JSON.stringify: the output a numeric or
// string space argument produces (10_value_model, JSON serialization). It mirrors
// the compact walk in json.go but writes a newline and the running indent before
// each object member and array element and a space after each key's colon, the
// exact shape V8 produces. The compact walk stays the canonical leaf encoder:
// this file reuses encodeJSONString and the JSON-undefined helpers and only
// re-implements the object and array framing the gap changes, so the compact path
// every existing fixture pins is left byte for byte as it was.

package value

import (
	"reflect"
	"strings"
	"unicode/utf16"
)

// JSONStringifyIndentNum is JSON.stringify(v, null, space) with a numeric space:
// the gap is that many spaces, clamped to ten and floored through ToInteger, and a
// space below one produces the compact form with no indentation, exactly as the
// specification's SerializeJSONProperty computes the gap.
func JSONStringifyIndentNum(v any, space float64) BStr {
	n := toInteger(space)
	if n < 1 {
		return JSONStringify(v)
	}
	if n > 10 {
		n = 10
	}
	return jsonStringifyGap(v, strings.Repeat(" ", int(n)))
}

// JSONStringifyIndentStr is JSON.stringify(v, null, space) with a string space:
// the gap is the first ten code units of the string, and an empty string produces
// the compact form, matching how the specification truncates a string gap.
func JSONStringifyIndentStr(v any, space BStr) BStr {
	units := space.units()
	if len(units) == 0 {
		return JSONStringify(v)
	}
	if len(units) > 10 {
		units = units[:10]
	}
	return jsonStringifyGap(v, string(utf16.Decode(units)))
}

// jsonStringifyGap serializes v with a non-empty indentation gap, the shared entry
// the numeric and string forms reach once they have computed the gap.
func jsonStringifyGap(v any, gap string) BStr {
	var b strings.Builder
	e := jsonIndenter{gap: gap}
	e.encode(&b, v, "")
	return FromGoString(b.String())
}

// jsonIndenter carries the fixed gap through the indented walk. The current
// indent is passed down as a parameter rather than held here, since it grows one
// gap deeper at each nesting level and unwinds as the walk returns.
type jsonIndenter struct{ gap string }

// encode writes one value's indented JSON to b at the given indent. The leaf arms
// match encodeJSON exactly, since a scalar's text does not change with a gap; only
// the object and array arms add the newline-and-indent framing.
func (e jsonIndenter) encode(b *strings.Builder, v any, indent string) {
	switch x := v.(type) {
	case BStr:
		encodeJSONString(b, x)
	case float64:
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
		e.encodeArray(b, x.jsonElements(), indent)
	case Value:
		e.encodeBoxed(b, x, indent)
	case jsonArmer:
		e.encode(b, x.JSONArm(), indent)
	default:
		if jsonUndefinedGo(v) {
			return
		}
		if r, ok := jsonToJSONGo(v); ok {
			e.encode(b, r, indent)
			return
		}
		e.encodeObject(b, reflect.ValueOf(v), indent)
	}
}

// encodeArray writes a bracketed list with each element on its own indented line,
// a JSON-undefined element folded to null the way the compact array arm folds it.
// An empty array stays [] with no interior newline, matching V8.
func (e jsonIndenter) encodeArray(b *strings.Builder, elems []any, indent string) {
	if len(elems) == 0 {
		b.WriteString("[]")
		return
	}
	inner := indent + e.gap
	b.WriteByte('[')
	for i, el := range elems {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
		b.WriteString(inner)
		if jsonUndefinedGo(el) {
			b.WriteString("null")
			continue
		}
		e.encode(b, el, inner)
	}
	b.WriteByte('\n')
	b.WriteString(indent)
	b.WriteByte(']')
}

// encodeObject writes a generated object struct as an indented object, its fields
// each on their own line. An object with no serialized member stays {} with no
// interior newline.
func (e jsonIndenter) encodeObject(b *strings.Builder, rv reflect.Value, indent string) {
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	inner := indent + e.gap
	b.WriteByte('{')
	first := true
	e.encodeFields(b, rv, inner, &first)
	if !first {
		b.WriteByte('\n')
		b.WriteString(indent)
	}
	b.WriteByte('}')
}

// encodeFields writes one struct's fields as indented members, sharing the first
// flag with its caller so an embedded base's fields interleave into the same
// object at the same depth. It mirrors encodeJSONFields' field selection: an
// unexported field is machinery and skipped, an anonymous field flattens, an
// optional property omits an absent value and unwraps a present one, and a
// function-valued property has no JSON form and omits its key.
func (e jsonIndenter) encodeFields(b *strings.Builder, rv reflect.Value, inner string, first *bool) {
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if f.Anonymous {
			e.encodeFields(b, rv.Field(i), inner, first)
			continue
		}
		val := rv.Field(i).Interface()
		if opt, ok := val.(jsonOptional); ok {
			present := false
			val, present = opt.jsonOptField()
			if !present {
				continue
			}
		}
		if jsonUndefinedGo(val) {
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
		b.WriteByte('\n')
		b.WriteString(inner)
		encodeJSONString(b, FromGoString(key))
		b.WriteString(": ")
		e.encode(b, val, inner)
	}
}

// encodeBoxed writes a boxed dynamic Value as indented JSON, the indented mirror
// of encodeBoxedJSON for a value whose shape is only known at runtime, such as a
// JSON.parse result flowing back through an indented stringify. It follows the
// same SerializeJSONProperty rules: a JSON-undefined array element folds to null
// and a JSON-undefined object property omits its key.
func (e jsonIndenter) encodeBoxed(b *strings.Builder, v Value, indent string) {
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
		if len(o.elems) == 0 {
			b.WriteString("[]")
			return
		}
		inner := indent + e.gap
		b.WriteByte('[')
		for i, el := range o.elems {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
			b.WriteString(inner)
			if jsonUndefinedValue(el) {
				b.WriteString("null")
				continue
			}
			e.encodeBoxed(b, el, inner)
		}
		b.WriteByte('\n')
		b.WriteString(indent)
		b.WriteByte(']')
	case KindObject:
		o := v.object()
		inner := indent + e.gap
		b.WriteByte('{')
		first := true
		for i := range o.keys {
			if !o.descs[i].enumerable {
				continue
			}
			val := o.descs[i].read(v)
			if jsonUndefinedValue(val) {
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteByte('\n')
			b.WriteString(inner)
			encodeJSONString(b, o.keys[i])
			b.WriteString(": ")
			e.encodeBoxed(b, val, inner)
		}
		if !first {
			b.WriteByte('\n')
			b.WriteString(indent)
		}
		b.WriteByte('}')
	default:
		// undefined, symbol, and function have no JSON form; nothing is written.
	}
}
