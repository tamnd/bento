// This file owns JSON.stringify with a replacer: a replacer function that is
// called for every key and value and whose return replaces the value, and a
// replacer array that whitelists which object keys are serialized
// (10_value_model, JSON serialization). Both forms follow the specification's
// SerializeJSONProperty walk, which the compact and indented encoders in json.go
// and jsonindent.go implement for the no-replacer case.
//
// A replacer operates on boxed dynamic Values: the specification hands the
// replacer the property value and takes back whatever it returns, so the value
// must be a Value the compiled arrow can read and rebuild. The statically typed
// argument is therefore lifted to a Value tree once, by jsonToValue, and the walk
// runs over that tree. The lift is confined to the replacer path, so the far more
// common no-replacer stringify keeps its direct reflection walk with no boxing.

package value

import (
	"reflect"
	"strconv"
	"strings"
)

// JSONStringifyReplacerFunc is JSON.stringify(v, replacer, space) with a function
// replacer. The value is lifted to a Value tree, the replacer is applied to the
// root and then to every key and value top-down, and the result is serialized with
// the given gap, an empty gap meaning the compact form.
func JSONStringifyReplacerFunc(v any, replacer func(BStr, Value) Value, gap string) BStr {
	s := jsonValueSerializer{gap: gap, replacer: replacer}
	return s.stringify(jsonToValue(v))
}

// JSONStringifyReplacerArray is JSON.stringify(v, keys, space) with an array
// replacer: only the keys the array lists are serialized, in the array's order,
// and a listed key an object does not have is skipped. The value is lifted to a
// Value tree and serialized with the given gap.
func JSONStringifyReplacerArray(v any, keys []BStr, gap string) BStr {
	s := jsonValueSerializer{gap: gap, keys: dedupeKeys(keys), useKeys: true}
	return s.stringify(jsonToValue(v))
}

// JSONGapNum computes the indentation gap for a numeric space: that many spaces,
// floored through ToInteger and clamped to ten, and the empty gap for a space
// below one, matching the gap the specification's SerializeJSONProperty derives.
func JSONGapNum(space float64) string {
	n := toInteger(space)
	if n < 1 {
		return ""
	}
	if n > 10 {
		n = 10
	}
	return strings.Repeat(" ", int(n))
}

// JSONGapStr computes the indentation gap for a string space: the first ten code
// units of the string, and the empty gap for an empty string.
func JSONGapStr(space BStr) string {
	units := space.units()
	if len(units) == 0 {
		return ""
	}
	if len(units) > 10 {
		units = units[:10]
	}
	return FromUTF16(units).ToGoString()
}

// dedupeKeys returns the keys with later duplicates removed, keeping first-seen
// order, the way JSON.stringify's PropertyList is built from a replacer array.
func dedupeKeys(keys []BStr) []BStr {
	out := make([]BStr, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		s := k.ToGoString()
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, k)
	}
	return out
}

// jsonValueSerializer carries the walk's fixed state: the indentation gap, the
// optional replacer function, and the optional key whitelist. It serializes a
// Value tree following SerializeJSONProperty, so its output matches the direct
// encoders for the same tree while also honoring the replacer and whitelist.
type jsonValueSerializer struct {
	gap      string
	replacer func(BStr, Value) Value
	keys     []BStr
	useKeys  bool
}

// stringify serializes the root value. The specification wraps the value in a
// holder object under the empty key and applies the replacer to it before the
// walk, so a replacer sees the root as the value of "". A root that serializes as
// JSON-undefined yields the empty string, the value model's stand-in for the
// undefined JSON.stringify returns.
func (s *jsonValueSerializer) stringify(root Value) BStr {
	root = s.apply(FromGoString(""), root)
	str, ok := s.serialize(root, "")
	if !ok {
		return BStr{}
	}
	return FromGoString(str)
}

// apply runs the replacer for one key and value, returning the value unchanged
// when there is no replacer. The holder is not threaded through because only an
// inline arrow can be a replacer here and an arrow does not bind this.
func (s *jsonValueSerializer) apply(key BStr, val Value) Value {
	if s.replacer == nil {
		return val
	}
	return s.replacer(key, val)
}

// serialize returns the JSON text of one value at the given indent and whether it
// has a JSON form; a value that serializes as undefined reports false so its
// caller can omit an object key or fold an array element to null.
func (s *jsonValueSerializer) serialize(v Value, indent string) (string, bool) {
	switch v.kind {
	case KindNull:
		return "null", true
	case KindBool:
		if v.AsBool() {
			return "true", true
		}
		return "false", true
	case KindNumber:
		f := v.AsNumber()
		if f != f || f > maxFinite || f < -maxFinite {
			return "null", true
		}
		return NumberToString(f).ToGoString(), true
	case KindString:
		var b strings.Builder
		encodeJSONString(&b, v.str())
		return b.String(), true
	case KindArray:
		return s.serializeArray(v, indent), true
	case KindObject:
		return s.serializeObject(v, indent), true
	default:
		return "", false
	}
}

// serializeArray serializes an array, applying the replacer to each element under
// its index key and folding a JSON-undefined element to null. The whitelist does
// not apply to array indices, only to object keys, so every element is walked.
func (s *jsonValueSerializer) serializeArray(v Value, indent string) string {
	o := v.object()
	if len(o.elems) == 0 {
		return "[]"
	}
	inner := indent + s.gap
	parts := make([]string, len(o.elems))
	for i, el := range o.elems {
		el = s.apply(FromGoString(strconv.Itoa(i)), el)
		str, ok := s.serialize(el, inner)
		if !ok {
			str = "null"
		}
		parts[i] = str
	}
	return s.wrap('[', ']', parts, indent, inner)
}

// serializeObject serializes an object, taking its keys from the whitelist when
// one is set and otherwise from its own enumerable keys in insertion order. Each
// value passes through the replacer, and a value with no JSON form omits its key.
func (s *jsonValueSerializer) serializeObject(v Value, indent string) string {
	inner := indent + s.gap
	var parts []string
	for _, k := range s.objectKeys(v) {
		val := s.apply(k, v.Get(k))
		str, ok := s.serialize(val, inner)
		if !ok {
			continue
		}
		var kb strings.Builder
		encodeJSONString(&kb, k)
		sep := ":"
		if s.gap != "" {
			sep = ": "
		}
		parts = append(parts, kb.String()+sep+str)
	}
	return s.wrap('{', '}', parts, indent, inner)
}

// objectKeys returns the keys to serialize for an object: the whitelist verbatim
// when one is set, so a listed but absent key still reads as undefined and drops,
// or the object's own enumerable keys in insertion order otherwise.
func (s *jsonValueSerializer) objectKeys(v Value) []BStr {
	if s.useKeys {
		return s.keys
	}
	o := v.object()
	var keys []BStr
	for i := range o.keys {
		if o.descs[i].enumerable {
			keys = append(keys, o.keys[i])
		}
	}
	return keys
}

// wrap frames a bracketed list. With no members it is the empty pair on one line.
// With a gap it writes each member on its own line one gap deeper and closes at
// the outer indent; with no gap it is the compact comma-separated form.
func (s *jsonValueSerializer) wrap(open, close byte, parts []string, indent, inner string) string {
	if len(parts) == 0 {
		return string([]byte{open, close})
	}
	if s.gap == "" {
		return string(open) + strings.Join(parts, ",") + string(close)
	}
	return string(open) + "\n" + inner + strings.Join(parts, ",\n"+inner) + "\n" + indent + string(close)
}

// jsonToValue lifts a statically typed value to a boxed Value tree, the form the
// replacer walk needs so the compiled replacer can read each value and the walk
// can rebuild whatever it returns. It mirrors the type dispatch and field
// selection of the direct encoders: a BStr, number, and bool box to their scalar
// kinds, an array boxes each element, a tagged-sum union boxes its active arm, and
// a generated struct boxes its exported fields honoring the json tag, the optional
// hook, and anonymous flattening. A value with no JSON form boxes to undefined.
func jsonToValue(v any) Value {
	switch x := v.(type) {
	case BStr:
		return StringValue(x)
	case float64:
		return Number(x)
	case bool:
		if x {
			return True
		}
		return False
	case Value:
		return x
	case jsonArray:
		elems := x.jsonElements()
		out := make([]Value, len(elems))
		for i, e := range elems {
			out[i] = jsonToValue(e)
		}
		return NewArrayValue(out)
	case jsonArmer:
		return jsonToValue(x.JSONArm())
	default:
		if jsonUndefinedGo(v) {
			return Undefined
		}
		if r, ok := jsonToJSONGo(v); ok {
			return jsonToValue(r)
		}
		return jsonStructToValue(reflect.ValueOf(v))
	}
}

// jsonStructToValue boxes a generated object struct into a Value object, visiting
// its fields the way encodeJSONFields does: an unexported field is machinery and
// skipped, an anonymous field flattens its own fields into the same object, an
// absent optional property is dropped, and a present optional contributes the
// value it wraps.
func jsonStructToValue(rv reflect.Value) Value {
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	obj := NewObject()
	jsonStructFields(obj, rv)
	return obj
}

// jsonStructFields boxes one struct's fields into obj, flattening an embedded
// struct into the same object so a derived class's inherited fields sit alongside
// its own, in base-first order.
func jsonStructFields(obj Value, rv reflect.Value) {
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if f.Anonymous {
			jsonStructFields(obj, rv.Field(i))
			continue
		}
		val := rv.Field(i).Interface()
		if opt, ok := val.(jsonOptional); ok {
			inner, present := opt.jsonOptField()
			if !present {
				continue
			}
			val = inner
		}
		key := f.Tag.Get("json")
		if key == "" {
			key = f.Name
		}
		obj.Set(FromGoString(key), jsonToValue(val))
	}
}
