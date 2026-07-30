package value

// This file is require('assert'), the module a Node test is written against. It is
// the module the deep-comparison and message work of the two files beside it was for:
// assert is where a program meets both.
//
// The module is callable. `assert(value)` is `assert.ok(value)`, because node's
// module.exports is the ok function itself with the other methods hung off it, so the
// value require hands back is a function with properties. That shape is what a bento
// proxy over a function target gives: the callable is the target, the methods are its
// own properties, and the honest-stub trap over it still refuses a member this port
// does not carry.
//
// What is here is every method whose failure is a comparison: ok, equal, notEqual,
// strictEqual, notStrictEqual, the four deep forms, ifError, match, doesNotMatch and
// fail. What is not here is the three that need machinery this port does not have
// yet, and each throws the honest-stub error naming itself rather than answering
// undefined:
//
//   - throws and doesNotThrow need to call a function and inspect what it raised
//     against an error class, a validation object or a predicate, which is error-class
//     identity in the boxed world and is its own slice.
//   - rejects and doesNotReject are those two over a promise.
//   - partialDeepStrictEqual needs the partial comparison mode, which needs boxed Map
//     and Set to be worth having.
//   - CallTracker is deprecated in Node and is not planned.
//   - The Assert class carries the options (diff: 'full', skipPrototype, strict) that
//     the methods here read as their defaults, and needs a constructible function.
//   - AssertionError as a constructor a program can call or use with instanceof needs
//     a class whose instances are errors, which the error box does not do yet. The
//     error a failure throws carries name 'AssertionError' and code 'ERR_ASSERTION',
//     so a catch that branches on either reads what Node's does.

// newAssertModules builds require('assert') and require('assert/strict') together and
// returns the one the caller asked for. They are built as a pair because each names
// the other: assert.strict is the strict module, and the strict module's ok is the
// plain one, so neither can be finished before the other exists. Both are registered
// so that whichever specifier a program requires first, the two keep the single
// identity Node gives them.
//
// The strict module is the same module with equal, notEqual, deepEqual and notDeepEqual
// pointed at their strict counterparts. A test written against it gets === and
// deep-strict comparisons throughout, which is what makes it the recommended entry
// point. That is the whole difference between the two.
func newAssertModules(name string) Value {
	plainTarget := assertCallable("ok")
	assertSetMethods(plainTarget)
	plain := newPartialModule("assert", plainTarget)

	// The strict module carries the plain module's own methods, the same function values
	// rather than copies, so assert.strict.deepStrictEqual and assert.deepStrictEqual are
	// one function the way Node's ObjectAssign leaves them. Only the four loose names are
	// replaced, each by its strict counterpart.
	strictTarget := assertCallable("strict")
	for _, m := range assertMethodNames {
		strictTarget.Set(FromGoString(m), plainTarget.Get(FromGoString(m)))
	}
	for loose, strictName := range map[string]string{
		"equal":        "strictEqual",
		"notEqual":     "notStrictEqual",
		"deepEqual":    "deepStrictEqual",
		"notDeepEqual": "notDeepStrictEqual",
	} {
		strictTarget.Set(FromGoString(loose), plainTarget.Get(FromGoString(strictName)))
	}
	strict := newPartialModule("assert/strict", strictTarget)

	// assert.ok is the module itself in Node, which is why assert.ok.ok is also the
	// module and why a program that destructures { ok } gets something it can call. The
	// strict module's ok is the plain module rather than itself, since ok has no strict
	// form to differ in, and its strict points back at itself, so
	// require('assert').strict.strict is still the strict module. Every one of these is
	// the wrapped module rather than the bare callable, so the honest-stub trap survives
	// a member reached through any of the paths.
	plainTarget.Set(FromGoString("ok"), plain)
	plainTarget.Set(FromGoString("strict"), strict)
	strictTarget.Set(FromGoString("ok"), plain)
	strictTarget.Set(FromGoString("strict"), strict)

	builtinModuleCache["assert"] = plain
	builtinModuleCache["assert/strict"] = strict
	if name == "assert/strict" {
		return strict
	}
	return plain
}

// assertCallable builds the function half of a module: calling it is assert.ok. The
// name is the one Node gives the function, "ok" for the module and "strict" for the
// strict variant, so f.name reads the way it does there.
func assertCallable(name string) Value {
	return WithName(NewFunc(func(args []Value) Value {
		assertOk(args)
		return Undefined
	}), name)
}

// assertMethodNames are the module's methods in the order Node's own assert defines
// them, which is the order a program enumerating the module sees and the list the
// strict module copies.
var assertMethodNames = []string{
	"fail", "equal", "notEqual", "deepEqual", "notDeepEqual",
	"deepStrictEqual", "notDeepStrictEqual", "strictEqual", "notStrictEqual",
	"match", "doesNotMatch", "ifError",
}

// assertSetMethods hangs the module's methods off the plain module's callable.
func assertSetMethods(mod Value) {
	impls := map[string]func([]Value) Value{
		"fail":               assertFailMethod,
		"equal":              assertEqual,
		"notEqual":           assertNotEqual,
		"deepEqual":          assertDeepEqual,
		"notDeepEqual":       assertNotDeepEqual,
		"deepStrictEqual":    assertDeepStrictEqual,
		"notDeepStrictEqual": assertNotDeepStrictEqual,
		"strictEqual":        assertStrictEqual,
		"notStrictEqual":     assertNotStrictEqual,
		"match":              func(args []Value) Value { return assertMatch(args, true) },
		"doesNotMatch":       func(args []Value) Value { return assertMatch(args, false) },
		"ifError":            assertIfError,
	}
	for _, name := range assertMethodNames {
		mod.Set(FromGoString(name), WithName(NewFunc(impls[name]), name))
	}
}

// assertMissingArgs is the error a comparison method raises when it was called with
// fewer than two arguments, Node's ERR_MISSING_ARGS. It is not an assertion failure:
// assert.strictEqual(x) is a mistake in the test rather than a fact about x, and
// comparing against an undefined that was never passed would hide it.
func assertMissingArgs(args []Value) {
	if len(args) < 2 {
		Throw(NewNodeError("TypeError", "ERR_MISSING_ARGS",
			FromGoString(`The "actual" and "expected" arguments must be specified`)))
	}
}

// assertMessageArg reads the optional message argument the way the methods do: a
// message is present when it was passed and is neither undefined nor null, which is
// Node's `message == null` test.
func assertMessageArg(args []Value, i int) (Value, bool) {
	if len(args) <= i {
		return Undefined, false
	}
	m := args[i]
	if m.kind == KindUndefined || m.kind == KindNull {
		return Undefined, false
	}
	return m, true
}

// assertIsError reports whether a value is an error, the isError test Node makes
// before treating a message argument as an error to rethrow. A boxed error is an
// ordinary object carrying the error it came from, which is the brand read here.
func assertIsError(v Value) bool {
	return v.kind == KindObject && v.object().err != nil
}

// assertInnerFail is Node's innerFail: it raises the assertion failure, unless the
// message the caller passed is itself an error, in which case that error is thrown
// instead. Passing an error as the message is how a test says "fail with this error",
// and wrapping it in an AssertionError would bury it.
func assertInnerFail(actual, expected Value, operator string, message Value, hasMessage bool) {
	if hasMessage && assertIsError(message) {
		Throw(message.object().err)
	}
	Throw(NewAssertionError(actual, expected, operator, message, hasMessage))
}

// assertOk is Node's ok, reached both as assert(value) and as assert.ok(value): the
// bare truthiness test.
//
// Node's generated message quotes the source text of the expression that was falsy,
// read back from the stack and the file it came from. A compiled binary has no source
// to read, so the message is the one Node itself falls back to when it cannot find
// the source: the operator form, "false == true". A message that quotes the source
// needs the compiler to pass the text of the argument down, which is a lowering
// change and its own slice.
func assertOk(args []Value) {
	value := Arg(args, 0)
	if ToBoolean(value) {
		return
	}
	message, hasMessage := assertMessageArg(args, 1)
	if len(args) == 0 {
		// A call with no argument at all is a mistake in the test rather than a falsy
		// value, and Node says so instead of printing a diff of undefined against true.
		e := NewAssertionError(Undefined, Bool(true), "==",
			StringValue(FromGoString("No value argument passed to `assert.ok()`")), true)
		e.SetProperty("generatedMessage", Bool(true))
		Throw(e)
	}
	if hasMessage && assertIsError(message) {
		Throw(message.object().err)
	}
	Throw(NewAssertionError(value, Bool(true), "==", message, hasMessage))
}

// assertFailMethod is assert.fail. With no argument it fails with "Failed"; with one
// it fails with that argument as the message, which is the form a test uses to mark an
// unreachable branch. Its longer form, fail(actual, expected, message, operator), is
// deprecated in Node and carried here because the operator it takes is what makes the
// message say something the other methods cannot.
func assertFailMethod(args []Value) Value {
	actual, expected := Arg(args, 0), Arg(args, 1)
	message, hasMessage := assertMessageArg(args, 2)
	operator := "fail"
	internalMessage := false

	switch {
	case len(args) <= 1 && (actual.kind == KindUndefined || actual.kind == KindNull):
		internalMessage = true
		message, hasMessage = StringValue(FromGoString("Failed")), true
	case len(args) == 1:
		message, hasMessage = actual, true
		actual = Undefined
	case len(args) == 2:
		// Node warns about the deprecated multi-argument form here. A DeprecationWarning
		// needs process.emitWarning, which is not in this port yet, so the form works and
		// says nothing.
		operator = "!="
	default:
		if op := Arg(args, 3); op.kind != KindUndefined {
			operator = ToString(op).ToGoString()
		}
	}

	if hasMessage && assertIsError(message) {
		Throw(message.object().err)
	}
	e := NewAssertionError(actual, expected, operator, message, hasMessage)
	if internalMessage {
		// The "Failed" message is the module's own, not the caller's, so the flag says
		// generated even though a message was passed to the error.
		e.SetProperty("generatedMessage", Bool(true))
	}
	Throw(e)
	return Undefined
}

// assertLooseEqual is the comparison assert.equal and assert.notEqual share. Its NaN
// clause is not what == does: NaN == NaN is false, and assert.equal(NaN, NaN) passes
// anyway, because a test that compares two computed numbers means them to be the same
// number.
func assertLooseEqual(actual, expected Value) bool {
	return LooseEquals(actual, expected) || (assertIsNaN(actual) && assertIsNaN(expected))
}

// assertEqual is assert.equal, the loose comparison.
func assertEqual(args []Value) Value {
	assertMissingArgs(args)
	actual, expected := args[0], args[1]
	message, hasMessage := assertMessageArg(args, 2)
	if !assertLooseEqual(actual, expected) {
		assertInnerFail(actual, expected, "==", message, hasMessage)
	}
	return Undefined
}

// assertNotEqual is assert.notEqual, the mirror of the above: two NaNs count as equal
// here too, so notEqual(NaN, NaN) fails.
func assertNotEqual(args []Value) Value {
	assertMissingArgs(args)
	actual, expected := args[0], args[1]
	message, hasMessage := assertMessageArg(args, 2)
	if assertLooseEqual(actual, expected) {
		assertInnerFail(actual, expected, "!=", message, hasMessage)
	}
	return Undefined
}

// assertIsNaN is Number.isNaN: a number that is not equal to itself. Node's equal
// calls it on values of any kind, and a non-number is never NaN there, so a string
// "NaN" does not pass equal against a real NaN.
func assertIsNaN(v Value) bool {
	if v.kind != KindNumber {
		return false
	}
	f := v.AsNumber()
	return f != f
}

// assertStrictEqual is assert.strictEqual. The comparison is Object.is rather than
// ===, so two NaNs are equal and the two zeroes are not, which is what makes
// strictEqual(0, -0) fail.
func assertStrictEqual(args []Value) Value {
	assertMissingArgs(args)
	actual, expected := args[0], args[1]
	message, hasMessage := assertMessageArg(args, 2)
	if !sameValue(actual, expected) {
		assertInnerFail(actual, expected, "strictEqual", message, hasMessage)
	}
	return Undefined
}

// assertNotStrictEqual is assert.notStrictEqual, Object.is inverted.
func assertNotStrictEqual(args []Value) Value {
	assertMissingArgs(args)
	actual, expected := args[0], args[1]
	message, hasMessage := assertMessageArg(args, 2)
	if sameValue(actual, expected) {
		assertInnerFail(actual, expected, "notStrictEqual", message, hasMessage)
	}
	return Undefined
}

// assertDeepEqual is assert.deepEqual, the loose deep comparison.
func assertDeepEqual(args []Value) Value {
	assertMissingArgs(args)
	actual, expected := args[0], args[1]
	message, hasMessage := assertMessageArg(args, 2)
	if !DeepEqual(actual, expected) {
		assertInnerFail(actual, expected, "deepEqual", message, hasMessage)
	}
	return Undefined
}

// assertNotDeepEqual is assert.notDeepEqual.
func assertNotDeepEqual(args []Value) Value {
	assertMissingArgs(args)
	actual, expected := args[0], args[1]
	message, hasMessage := assertMessageArg(args, 2)
	if DeepEqual(actual, expected) {
		assertInnerFail(actual, expected, "notDeepEqual", message, hasMessage)
	}
	return Undefined
}

// assertDeepStrictEqual is assert.deepStrictEqual, the method most tests are written
// with.
func assertDeepStrictEqual(args []Value) Value {
	assertMissingArgs(args)
	actual, expected := args[0], args[1]
	message, hasMessage := assertMessageArg(args, 2)
	if !DeepStrictEqual(actual, expected) {
		assertInnerFail(actual, expected, "deepStrictEqual", message, hasMessage)
	}
	return Undefined
}

// assertNotDeepStrictEqual is assert.notDeepStrictEqual.
func assertNotDeepStrictEqual(args []Value) Value {
	assertMissingArgs(args)
	actual, expected := args[0], args[1]
	message, hasMessage := assertMessageArg(args, 2)
	if DeepStrictEqual(actual, expected) {
		assertInnerFail(actual, expected, "notDeepStrictEqual", message, hasMessage)
	}
	return Undefined
}

// assertIfError is assert.ifError, the callback-era check that an error argument is
// absent. It fails on anything other than null or undefined, including a falsy value,
// since a callback that reports a failure as 0 still reported one.
//
// Node splices the original error's stack onto the assertion's, so the failure points
// at where the error was created rather than at the assert call. A bento error carries
// no stack, so there is nothing to splice.
func assertIfError(args []Value) Value {
	err := Arg(args, 0)
	if err.kind == KindNull || err.kind == KindUndefined {
		return Undefined
	}
	message := "ifError got unwanted exception: "
	switch {
	case assertIsError(err):
		// An error's message is what identifies it; an error with none is identified by
		// its constructor instead, which for a bento error is the name it carries.
		if msg := err.object().err.ErrorMessage(); msg != "" {
			message += msg
		} else {
			message += err.object().err.ErrorName()
		}
	case assertIsObject(err) && err.Get(FromGoString("message")).kind == KindString:
		msg := err.Get(FromGoString("message"))
		if msg.str().Length() == 0 {
			ctor := err.Get(FromGoString("constructor"))
			if ctor.kind == KindFunc {
				message += ToString(ctor.Get(FromGoString("name"))).ToGoString()
			}
		} else {
			message += msg.str().ToGoString()
		}
	default:
		message += NodeInspect(err).ToGoString()
	}
	Throw(NewAssertionError(err, Null, "ifError", StringValue(FromGoString(message)), true))
	return Undefined
}

// assertMatch is assert.match and assert.doesNotMatch, which are one function in Node
// too: the same test with the sense of the result flipped. A non-string input fails
// either of them, since a value that is not a string cannot be said to match or not
// match a pattern.
func assertMatch(args []Value, want bool) Value {
	str, regexp := Arg(args, 0), Arg(args, 1)
	message, hasMessage := assertMessageArg(args, 2)
	name := "match"
	if !want {
		name = "doesNotMatch"
	}

	re := regexp.asRegExp()
	if re == nil {
		Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
			FromGoString(`The "regexp" argument must be an instance of RegExp. Received `+
				NodeInspect(regexp).ToGoString())))
	}

	matched := str.kind == KindString && re.Test(str.str())
	if str.kind == KindString && matched == want {
		return Undefined
	}

	if hasMessage && assertIsError(message) {
		Throw(message.object().err)
	}
	if !hasMessage {
		var text string
		switch {
		case str.kind != KindString:
			text = `The "string" argument must be of type string. Received type ` +
				str.TypeOf().ToGoString() + " (" + NodeInspect(str).ToGoString() + ")"
		case want:
			text = "The input did not match the regular expression " +
				NodeInspect(regexp).ToGoString() + ". Input:\n\n" + NodeInspect(str).ToGoString() + "\n"
		default:
			text = "The input was expected to not match the regular expression " +
				NodeInspect(regexp).ToGoString() + ". Input:\n\n" + NodeInspect(str).ToGoString() + "\n"
		}
		e := NewAssertionError(str, regexp, name, StringValue(FromGoString(text)), true)
		// The message was built here rather than passed in, so the flag says generated
		// even though the error was given one.
		e.SetProperty("generatedMessage", Bool(true))
		Throw(e)
	}
	Throw(NewAssertionError(str, regexp, name, message, hasMessage))
	return Undefined
}
