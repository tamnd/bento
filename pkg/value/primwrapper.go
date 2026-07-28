package value

// This file bridges a primitive wrapper object, the thing new Number, new String, and
// new Boolean build, into the dynamic value.Value world. A wrapper is an ordinary
// object value, typeof "object" and always truthy, whose Object carries the wrapped
// primitive in its prim field. Every coercion reads that primitive back: the object
// short-circuits toPrimitive to the wrapped value, so String(new Number(5)) renders
// "5", parseFloat(new Number(1.5)) parses "1.5", and +new Boolean(false) is 0, each
// exactly as the built-in prototype's valueOf and toString would produce. Only the
// wrapper's own reads, valueOf and toString and a String wrapper's length and index
// characters, route here; every other name climbs the ordinary prototype chain.

// primData is the payload a wrapper object carries: the wrapped primitive and the
// class that built it. The class picks the right toString spelling, a Number wrapper's
// toString taking an optional radix where the others do not.
type primData struct {
	value Value  // the wrapped primitive: a Number, String, or Bool value
	class string // "Number", "String", or "Boolean"
}

// NumberObject builds the wrapper new Number(x) produces: an object wrapping ToNumber
// of its argument, or +0 when called with none. typeof is "object" and the value is
// truthy regardless of the wrapped number, both free from the object kind; coercion
// reads the wrapped number back through the toPrimitive short-circuit.
func NumberObject(args ...Value) Value {
	n := 0.0
	if len(args) > 0 {
		n = numberCtorValue(args[0])
	}
	return objectValue(&Object{kind: KindObject, prim: &primData{value: Number(n), class: "Number"}})
}

// numberCtorValue is the number the Number constructor derives from its argument:
// ToNumeric, then the real value of a BigInt. A BigInt takes the non-throwing
// BigIntToNumber the way new Number(1n) and Number(1n) both convert, where the bare
// ToNumber the other kinds use would throw a TypeError on a bigint.
func numberCtorValue(v Value) float64 {
	if v.kind == KindBigInt {
		return BigIntToNumber(&v.bigint().i)
	}
	return ToNumber(v)
}

// StringObject builds the wrapper new String(x) produces: an object wrapping ToString
// of its argument, or "" when called with none. The wrapper answers .length and its
// index characters off the wrapped string the way a boxed primitive string does.
func StringObject(args ...Value) Value {
	s := FromGoString("")
	if len(args) > 0 {
		s = ToString(args[0])
	}
	return objectValue(&Object{kind: KindObject, prim: &primData{value: StringValue(s), class: "String"}})
}

// BooleanObject builds the wrapper new Boolean(x) produces: an object wrapping
// ToBoolean of its argument, or false when called with none. The wrapper object is
// truthy even when it wraps false, the classic if (new Boolean(false)) trap, because
// its truthiness comes from being an object, not from the wrapped bool.
func BooleanObject(args ...Value) Value {
	b := false
	if len(args) > 0 {
		b = ToBoolean(args[0])
	}
	return objectValue(&Object{kind: KindObject, prim: &primData{value: Bool(b), class: "Boolean"}})
}

// asPrimWrapper returns the prim payload a value wraps, or nil when the value is not a
// wrapper object, the probe the toPrimitive and read paths make before their ordinary
// object handling.
func (v Value) asPrimWrapper() *primData {
	if v.kind != KindObject {
		return nil
	}
	return v.object().prim
}

// primWrapperGet reads an own property off a wrapper object, the reads the wrapper's
// prototype methods answer. valueOf hands back the wrapped primitive, toString renders
// it, and a String wrapper additionally answers .length and its index characters off
// the wrapped string. A name that is no wrapper own property reports ok=false, so the
// caller climbs the ordinary prototype chain and answers undefined for a miss.
func primWrapperGet(p *primData, name string) (Value, bool) {
	switch name {
	case "valueOf":
		// valueOf hands the wrapped primitive straight back, the identity a wrapper's
		// prototype valueOf performs; a dynamic (new Number(5)).valueOf() reads 5.
		return NewFunc(func(args []Value) Value {
			return p.value
		}), true
	case "toString":
		// toString renders the wrapped primitive. A Number wrapper takes an optional radix,
		// (new Number(255)).toString(16) is "ff"; the others ignore any argument.
		return NewFunc(func(args []Value) Value {
			if p.class == "Number" {
				radix := 10.0
				if len(args) > 0 && !args[0].IsUndefined() {
					radix = ToNumber(args[0])
				}
				return StringValue(NumberToStringRadixDynamic(p.value.AsNumber(), radix))
			}
			return StringValue(ToString(p.value))
		}), true
	}
	if p.class == "String" {
		s := p.value.str()
		if name == "length" {
			return Number(s.Length()), true
		}
		if idx, ok := arrayIndex(name); ok {
			ch := s.CharAt(float64(idx))
			if ch.Length() == 0 {
				return Undefined, false
			}
			return StringValue(ch), true
		}
	}
	return Undefined, false
}
