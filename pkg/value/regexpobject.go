package value

// This file bridges a *RegExp into the dynamic value.Value world: the box a regexp
// takes when it flows into an any binding, an array element, or any other slot that
// holds a Value rather than a concrete *RegExp. The box is an ordinary object value
// whose Object carries the regexp in its regexp field, so typeof reports "object",
// the value is truthy, and two boxes of the same regexp compare equal by identity, all
// for free from the object kind; only the string form and the own-property reads route
// to the regexp.

// RegExpValue boxes a *RegExp into a dynamic value. The box is a KindObject value, so
// it is an object everywhere the value model asks its kind, and it keeps the live
// regexp reachable through the object's regexp field, so a read off the box sees the
// same lastIndex a concrete read would and String(box) renders the literal form.
func RegExpValue(re *RegExp) Value {
	return objectValue(&Object{kind: KindObject, regexp: re})
}

// asRegExp returns the regexp a value boxes, or nil when the value is not a regexp
// box, the probe the string and read paths make before their ordinary object handling.
func (v Value) asRegExp() *RegExp {
	if v.kind != KindObject {
		return nil
	}
	return v.object().regexp
}

// regexpGet reads an own property off a boxed regexp, mirroring the concrete accessors
// the statically-typed path emits: .source and .flags report the pattern text and the
// canonical flag run, each flag its own boolean, and .lastIndex the resume offset. A
// name that is not a regexp own property reports ok=false, so the caller climbs the
// ordinary prototype chain and answers undefined for a miss the way a plain object
// read does. The method properties (.test, .exec) read a callable bound to the live
// regexp, so a dynamic re.test(s) or re.exec(s) matches through the same *RegExp.
func regexpGet(re *RegExp, name string) (Value, bool) {
	switch name {
	case "source":
		return StringValue(re.Source()), true
	case "flags":
		return StringValue(re.Flags()), true
	case "global":
		return Bool(re.Global()), true
	case "ignoreCase":
		return Bool(re.IgnoreCase()), true
	case "multiline":
		return Bool(re.Multiline()), true
	case "dotAll":
		return Bool(re.DotAll()), true
	case "unicode":
		return Bool(re.Unicode()), true
	case "unicodeSets":
		return Bool(re.UnicodeSets()), true
	case "sticky":
		return Bool(re.Sticky()), true
	case "hasIndices":
		return Bool(re.HasIndices()), true
	case "lastIndex":
		return Number(re.LastIndex()), true
	case "test":
		// A read of .test off the box yields a callable bound to the regexp, so a dynamic
		// re.test(s) matches through the same *RegExp the concrete path uses: the subject
		// coerces to a string the way the built-in does, and a global regexp advances its
		// own lastIndex because the closure captures the one live regexp.
		return NewFunc(func(args []Value) Value {
			return Bool(re.Test(subjectArg(args)))
		}), true
	case "exec":
		// A read of .exec off the box yields a callable bound to the regexp; it returns
		// the match array or null the concrete Exec produces, already a boxed value.
		return NewFunc(func(args []Value) Value {
			return re.Exec(subjectArg(args))
		}), true
	}
	return Undefined, false
}

// subjectArg coerces the first argument of a boxed regexp's test or exec to the
// subject string, the way the built-ins run ToString on their argument. A call with no
// argument matches against "undefined", the string the missing argument coerces to,
// matching the engine.
func subjectArg(args []Value) BStr {
	if len(args) == 0 {
		return ToString(Undefined)
	}
	return ToString(args[0])
}
