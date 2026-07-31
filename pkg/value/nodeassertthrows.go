package value

// This file is assert.throws and assert.doesNotThrow, the two assert methods whose
// subject is a function rather than a value. Everything else in the module compares
// what it was handed; these two run code and then decide whether what came out of it
// is the failure the test was expecting.
//
// The decision is the whole content. Node accepts four kinds of expectation and each
// means something different:
//
//	assert.throws(fn, TypeError)             // an error class
//	assert.throws(fn, /pattern/)             // a regexp over String(thrown)
//	assert.throws(fn, { code: 'ENOENT' })    // a validation object, key by key
//	assert.throws(fn, (e) => e.x === 1)      // a validation function returning true
//
// plus a string third form that is not an expectation at all: assert.throws(fn, 'msg')
// passes the message, and node refuses it as the second of three arguments because a
// caller who writes both means one of them as the expectation and node cannot tell
// which.
//
// The port is node's lib/assert.js: getActual, expectsError, expectedException,
// compareExceptionKey, the Comparison placeholder, hasMatchingError and expectsNoError,
// each here under an assert-prefixed name. rejects and doesNotReject are the same two
// over a promise and stay out until promises box.
//
// Three things read differently in bento than in node and each is marked where it
// happens. A caught error is a *Error the runtime models by name, so
// `thrown instanceof TypeError` is a name comparison and node's "an error with
// identical name but a different prototype" case cannot arise. An error constructor
// named as a value is a non-callable function value carrying its name, which is what
// tells an error class apart from a validation function here, where node reads
// prototypes. And a bento boxed error carries name and message as enumerable own
// properties, so a validation object that is itself an error may name them twice.

// assertThrows is assert.throws. The function is called, whatever it raised is caught,
// and the expectation decides. Node spells this as expectsError(throws, getActual(fn),
// ...args) and the argument count after the function matters twice over, so the rest is
// passed along as a slice rather than as two optional values.
func assertThrows(args []Value) Value {
	caught, threw := assertGetActual(Arg(args, 0))
	assertExpectsError("throws", caught, threw, assertRestArgs(args))
	return Undefined
}

// assertDoesNotThrow is assert.doesNotThrow. A function that raises nothing passes; one
// that raises something either fails the assertion, when the error is the kind the call
// named, or is rethrown untouched, when it is not. That second half is what makes the
// method worth having over a bare call: doesNotThrow(fn, TypeError) says "a TypeError
// here is a bug in the code under test" and lets every other error through as itself.
func assertDoesNotThrow(args []Value) Value {
	caught, threw := assertGetActual(Arg(args, 0))
	assertExpectsNoError("doesNotThrow", caught, threw, assertRestArgs(args))
	return Undefined
}

// assertRestArgs is the arguments after the function, node's ...args. Its length is
// read rather than only its contents: two arguments after the function is what makes a
// string expectation an error rather than a message.
func assertRestArgs(args []Value) []Value {
	if len(args) <= 1 {
		return nil
	}
	return args[1:]
}

// assertGetActual is node's getActual: call the function and report what it raised.
// The caught error is returned as the runtime's *Error rather than as a boxed value so
// that doesNotThrow can rethrow the very error it caught, identity intact, and so the
// name comparison an error class asks for reads the error itself.
//
// A Go runtime panic is not a JavaScript throw, and Caught re-panics it, so a bug in
// the runtime surfaces as itself instead of as a test that passed.
func assertGetActual(fn Value) (caught *Error, threw bool) {
	if fn.kind != KindFunc {
		Throw(invalidArgType("fn", "function", fn))
	}
	defer func() {
		if r := recover(); r != nil {
			caught, threw = Caught(r), true
		}
	}()
	fn.Call()
	return nil, false
}

// assertExpectsError is node's expectsError: the argument checking, the
// nothing-was-thrown failure, and then the comparison. Node reaches the last through
// expectedException; the two are split the same way here.
//
// The string case is the one to read twice. A string second argument is the message,
// not an expectation, so it moves; but if the caller also passed a message then two
// arguments claim to be one and node refuses the call outright. It also refuses a
// string that is identical to the thrown error's message, because
// assert.throws(fn, 'boom') reads as "expect the message boom" and does not check it,
// so a passing call would be a test that asserts nothing.
func assertExpectsError(operator string, caught *Error, threw bool, rest []Value) {
	expected, message := Arg(rest, 0), Arg(rest, 1)
	var actual Value
	if threw {
		actual = caught.ToValue()
	}

	switch {
	case expected.kind == KindString:
		if len(rest) >= 2 {
			Throw(assertInvalidErrorArg(expected))
		}
		if threw {
			if assertIsObject(actual) {
				if msg := actual.Get(FromGoString("message")); StrictEquals(msg, expected) {
					Throw(assertAmbiguousArgument(`The error message "` +
						ToString(msg).ToGoString() + `" is identical to the message.`))
				}
			} else if StrictEquals(actual, expected) {
				Throw(assertAmbiguousArgument(`The error "` +
					ToString(actual).ToGoString() + `" is identical to the message.`))
			}
		}
		message, expected = expected, Undefined
	case expected.kind != KindUndefined && expected.kind != KindNull &&
		!assertIsObject(expected) && expected.kind != KindFunc:
		// A number, a boolean or a symbol cannot express an expectation, and coercing one
		// would compare against something the caller did not write.
		Throw(assertInvalidErrorArg(expected))
	}

	if !threw {
		// The expectation names itself in the failure when it can: an error class or a
		// validation object with a name reads as "Missing expected exception (TypeError)",
		// which is the whole message in most test output.
		details := ""
		if assertIsObject(expected) || expected.kind == KindFunc {
			if name := expected.Get(FromGoString("name")); ToBoolean(name) {
				details += " (" + ToString(name).ToGoString() + ")"
			}
		}
		if ToBoolean(message) {
			details += ": " + ToString(message).ToGoString()
		} else {
			details += "."
		}
		assertInnerFail(Undefined, expected, operator,
			StringValue(FromGoString("Missing expected exception"+details)), true)
	}

	// No expectation left means the call only asked that something be thrown, which it
	// was. This is where assert.throws(fn) and assert.throws(fn, 'message') end.
	if !ToBoolean(expected) {
		return
	}
	assertExpectedException(actual, expected, message, operator)
}

// assertExpectedException is node's expectedException: the four expectation kinds. It
// returns on a match and throws otherwise, and the message it throws is generated here
// unless the caller wrote one, which is what generatedMessage on the failure reports.
func assertExpectedException(actual, expected, message Value, operator string) {
	generatedMessage := false
	throwError := false

	if expected.kind != KindFunc {
		switch {
		case expected.asRegExp() != nil:
			// A regexp is matched against the string form of what was thrown, which for an
			// error is "TypeError: bad" rather than its message, so /bad/ and /^TypeError/
			// both match and /^bad/ does not.
			str := ToString(actual)
			if expected.asRegExp().Test(str) {
				return
			}
			if !ToBoolean(message) {
				generatedMessage = true
				message = StringValue(FromGoString("The input did not match the regular expression " +
					NodeInspect(expected).ToGoString() + ". Input:\n\n" +
					NodeInspect(StringValue(str)).ToGoString() + "\n"))
			}
			throwError = true
		case !assertIsObject(actual):
			// A validation object cannot be compared key by key against a thrown primitive,
			// so the failure is the deep-equal diff of the two, which shows the caller both
			// what they asked for and that what arrived has no properties at all. The message
			// is the diff's, and only the operator says which method reported it.
			e := NewAssertionError(actual, expected, "deepStrictEqual", message, ToBoolean(message))
			e.SetProperty("operator", StringValue(FromGoString(operator)))
			Throw(e)
		default:
			assertValidationObject(actual, expected, message, operator)
			return
		}
	} else if name, ok := assertErrorCtorName(expected); ok {
		// An error class matches when the thrown error is one, which the runtime decides by
		// name: Error matches every error, and a specific name matches that name. Node
		// decides it by prototype and so can catch an error whose name matches while its
		// prototype does not, a case that cannot arise here.
		if e := assertErrorBehind(actual); e != nil && e.IsA(name) {
			return
		}
		if !ToBoolean(message) {
			generatedMessage = true
			message = StringValue(FromGoString(assertInstanceOfMessage(actual, name)))
		}
		throwError = true
	} else {
		// A validation function has to return true, not merely something truthy, because a
		// predicate that falls off its end returns undefined and a test written that way
		// would pass without checking anything.
		res := expected.Call(actual)
		if !StrictEquals(res, Bool(true)) {
			if !ToBoolean(message) {
				generatedMessage = true
				text := "The "
				if name := functionNameFor(expected); name != "" {
					text += `"` + name + `" `
				}
				text += `validation function is expected to return "true". Received ` +
					NodeInspect(res).ToGoString()
				if e := assertErrorBehind(actual); e != nil {
					text += "\n\nCaught error:\n\n" + e.Error()
				}
				message = StringValue(FromGoString(text))
			}
			throwError = true
		}
	}

	if throwError {
		e := NewAssertionError(actual, expected, operator, message, ToBoolean(message))
		e.SetProperty("generatedMessage", Bool(generatedMessage))
		Throw(e)
	}
}

// assertInstanceOfMessage is the failure for an error class that did not match. What
// arrived is named rather than printed when it is an error, since a class comparison is
// about the class, and its message follows underneath because that is what identifies
// which error it was. A thrown non-error is printed instead, at depth -1, so a thrown
// object reads as [Object] rather than as its contents.
func assertInstanceOfMessage(actual Value, name string) string {
	text := `The error is expected to be an instance of "` + name + `". Received `
	if e := assertErrorBehind(actual); e != nil {
		text += `"` + e.ErrorName() + `"`
		if msg := e.ErrorMessage(); msg != "" {
			text += "\n\nError message:\n\n" + msg
		}
		return text
	}
	o := defaultInspectOptions()
	o.depth = -1
	return text + `"` + inspectWith(o, actual) + `"`
}

// assertValidationObject is node's validation-object branch: every own enumerable key
// of the expectation is compared against the same key on what was thrown. An expectation
// that is itself an error also compares name and message, since those are what an error
// is, and an empty expectation is refused because it would assert nothing.
//
// A key whose expectation is a regexp and whose actual value is a string matches by
// pattern rather than by equality, so { message: /timed out/ } is the readable way to
// assert a message.
func assertValidationObject(actual, expected, message Value, operator string) {
	keys := expected.OwnEnumerableKeys().Elems()
	if assertIsError(expected) {
		// bento's boxed error carries name and message as own enumerable properties, so
		// they can already be in the list; node's are non-enumerable and never are.
		keys = assertAppendKey(assertAppendKey(keys, "name"), "message")
	} else if len(keys) == 0 {
		Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_VALUE",
			FromGoString("The argument 'error' may not be an empty object. Received "+
				NodeInspect(expected).ToGoString())))
	}
	for _, key := range keys {
		if av := actual.Get(key); av.kind == KindString {
			if re := expected.Get(key).asRegExp(); re != nil && re.Test(av.str()) {
				continue
			}
		}
		assertCompareExceptionKey(actual, expected, key, message, keys, operator)
	}
}

// assertAppendKey adds a key to the comparison list unless it is already there.
func assertAppendKey(keys []BStr, name string) []BStr {
	key := FromGoString(name)
	for _, k := range keys {
		if k.Equal(key) {
			return keys
		}
	}
	return append(keys, key)
}

// assertCompareExceptionKey is node's compareExceptionKey: one key of a validation
// object against the thrown error. The failure it raises is a diff of two placeholder
// objects rather than of the error itself, because inspecting a real error prints
// everything it carries and the reader is looking for the one key that differed. The
// error's own actual and expected properties are then set back to the real values, so a
// test that catches the failure reads the error rather than the placeholder.
func assertCompareExceptionKey(actual, expected Value, key BStr, message Value, keys []BStr, operator string) {
	if actual.HasProperty(key) && DeepStrictEqual(actual.Get(key), expected.Get(key)) {
		return
	}
	if !ToBoolean(message) {
		e := NewAssertionError(assertComparison(actual, keys, Undefined),
			assertComparison(expected, keys, actual), "deepStrictEqual", Undefined, false)
		e.SetProperty("actual", actual)
		e.SetProperty("expected", expected)
		e.SetProperty("operator", StringValue(FromGoString(operator)))
		Throw(e)
	}
	assertInnerFail(actual, expected, operator, message, true)
}

// assertComparisonProto is the prototype the placeholder objects carry, and it exists
// for one reason: the constructor name is part of the message. Node's placeholder is an
// instance of a class named Comparison, so its inspected form reads "Comparison { ... }"
// and the reader can see the diff is of a comparison rather than of their own object. A
// bento object reaches the same name through a prototype carrying a named constructor,
// which is where the inspector looks.
var assertComparisonProto = newAssertComparisonProto()

func newAssertComparisonProto() Value {
	proto := NewObject()
	proto.Set(FromGoString("constructor"),
		WithName(NewFunc(func([]Value) Value { return Undefined }), "Comparison"))
	return proto
}

// assertComparison is node's Comparison: the subset of an object holding only the keys
// under comparison, so the diff shows those and nothing else. A key the object does not
// have is left out rather than shown as undefined, which is what makes a missing key
// read as a missing line in the diff.
//
// The actual argument is passed only when building the expectation's placeholder, and it
// is what keeps a matching regexp from showing as a difference: a key the pattern
// matched takes the actual string on both sides, so the diff shows the keys that
// differed instead of every regexp the caller wrote.
func assertComparison(obj Value, keys []BStr, actual Value) Value {
	out := NewObject().SetPrototype(assertComparisonProto)
	for _, key := range keys {
		if !obj.HasProperty(key) {
			continue
		}
		if actual.kind != KindUndefined {
			if av := actual.Get(key); av.kind == KindString {
				if re := obj.Get(key).asRegExp(); re != nil && re.Test(av.str()) {
					out.Set(key, av)
					continue
				}
			}
		}
		out.Set(key, obj.Get(key))
	}
	return out
}

// assertExpectsNoError is node's expectsNoError, the doesNotThrow half. An error the
// call did not name is rethrown as itself: doesNotThrow exists to say a particular
// failure would be a bug, and swallowing every other error would hide the bug it was
// not written about.
func assertExpectsNoError(operator string, caught *Error, threw bool, rest []Value) {
	if !threw {
		return
	}
	actual := caught.ToValue()
	expected, message := Arg(rest, 0), Arg(rest, 1)
	if expected.kind == KindString {
		message, expected = expected, Undefined
	}

	if !ToBoolean(expected) || assertHasMatchingError(actual, expected) {
		details := "."
		if ToBoolean(message) {
			details = ": " + ToString(message).ToGoString()
		}
		assertInnerFail(actual, expected, operator,
			StringValue(FromGoString("Got unwanted exception"+details+
				"\nActual message: \""+assertActualMessage(actual)+"\"")), true)
	}
	Throw(caught)
}

// assertActualMessage is node's `actual?.message` in the unwanted-exception failure. A
// thrown primitive has no message and reads as undefined, the way the optional chain
// spells it.
func assertActualMessage(actual Value) string {
	if actual.IsNullish() {
		return "undefined"
	}
	return ToString(actual.Get(FromGoString("message"))).ToGoString()
}

// assertHasMatchingError is node's hasMatchingError: the doesNotThrow expectation, which
// accepts only a class, a regexp or a predicate. A validation object is not accepted
// here even though throws accepts one, which is node's own asymmetry rather than an
// omission.
func assertHasMatchingError(actual, expected Value) bool {
	if expected.kind != KindFunc {
		if re := expected.asRegExp(); re != nil {
			return re.Test(ToString(actual))
		}
		Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
			FromGoString(`The "expected" argument must be of type function or an instance of RegExp. `+
				receivedDescription(expected))))
	}
	if name, ok := assertErrorCtorName(expected); ok {
		e := assertErrorBehind(actual)
		return e != nil && e.IsA(name)
	}
	return StrictEquals(expected.Call(actual), Bool(true))
}

// assertErrorCtorName reports the name of an error constructor named as a value, and
// whether the value is one at all. It is the discriminator node makes with prototypes:
// an error constructor is a function value the runtime built to carry a name and
// nothing else, so it has no call behind it, while every function a program can write
// does. A callable is therefore a validation function and a non-callable is a class.
func assertErrorCtorName(v Value) (string, bool) {
	if v.kind != KindFunc || v.object().call != nil {
		return "", false
	}
	return functionNameFor(v), true
}

// assertErrorBehind reports the runtime error a value is the box of, or nil for a value
// that is not an error. It is the handle behind assertIsError, needed because the class
// comparison and the failure message read the error's own name and message rather than
// the box's properties.
func assertErrorBehind(v Value) *Error {
	if v.kind != KindObject {
		return nil
	}
	return v.object().err
}

// assertInvalidErrorArg is the ERR_INVALID_ARG_TYPE for a second argument that cannot be
// an expectation. Node builds the sentence from the list ['Object', 'Error', 'Function',
// 'RegExp'], which its formatter renders by folding Object in with the instances; the
// rendered form is written out here rather than the list, since assert is the only
// caller and the fold has no other use yet.
func assertInvalidErrorArg(got Value) *Error {
	return NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
		FromGoString(`The "error" argument must be of type function or an instance of `+
			`Error, RegExp, or Object. `+receivedDescription(got)))
}

// assertAmbiguousArgument is the ERR_AMBIGUOUS_ARGUMENT for an expectation and a message
// that cannot be told apart.
func assertAmbiguousArgument(detail string) *Error {
	return NewNodeError("TypeError", "ERR_AMBIGUOUS_ARGUMENT",
		FromGoString(`The "error/message" argument is ambiguous. `+detail))
}
