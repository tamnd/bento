package value

// This file is the runtime side of the CommonJS module-wrapper globals. The
// lowerer models module, exports, __dirname, and __filename inline (they are a
// value object and two compile-time strings), but require is a live function the
// compiled module holds and calls, so it lives here as a callable value.

// RequireFunc returns the CommonJS require function as a callable value, the box a
// module's require binding takes. Until the module system lands (roadmap G0.3),
// every call throws the error Node raises for a specifier it cannot resolve, so a
// program that only probes require works (typeof require is "function", and
// require can be passed around and stored) while an actual require fails honestly
// rather than resolving to a silent wrong value. The specifier is coerced to a
// string the way Node coerces its argument, and the message matches Node's exactly
// so a test that asserts on err.message compares equal.
func RequireFunc() Value {
	return NewFunc(func(args []Value) Value {
		specifier := ToString(Arg(args, 0)).ToGoString()
		Throw(NewError(FromGoString("Cannot find module '" + specifier + "'")))
		return Undefined
	})
}
