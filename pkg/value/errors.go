package value

import (
	"errors"
	"os"
	"unsafe"
)

// This file is the value-model side of thrown JavaScript errors. A throw in the
// typed world lowers to a Go panic carrying a thrown value, a catch recovers it,
// and an uncaught throw is reported at the program root. Modeling a throw as a Go
// panic keeps the fast path free: a function that never throws pays nothing, and
// the unwinding a throw needs is exactly what panic already does, so bento does
// not carry a second error-return convention through every lowered signature.
//
// A thrown value is not always an Error object in JavaScript, but the common and
// idiomatic case is, and the boundary raises (a range check, a failed go: call)
// raise error-shaped values too, so the runtime models a thrown error as a small
// typed value with a name and a message. The marker interface lets the top-level
// handler tell a deliberate throw from a Go runtime panic that signals a bug in
// the runtime itself, which must keep its original crash rather than be dressed up
// as a caught JavaScript error.

// Thrown marks a Go panic payload that carries a thrown JavaScript value. A
// deliberate throw (an Error the program raised, a boundary range check, a failed
// go: call) implements it; a Go runtime panic (a nil dereference, an out-of-range
// index in the runtime) does not, so the top-level handler reports the first and
// re-panics the second. The two methods are the surface a catch and the reporter
// read: the error's constructor name and its message.
type Thrown interface {
	ErrorName() string
	ErrorMessage() string
}

// Error is a thrown JavaScript error in the typed world: a constructor name and a
// message, the two properties every built-in error carries. Both are held as bento
// strings, so a catch that reads err.name or err.message gets the JavaScript string
// with no re-transcoding on the read path. It is a pointer type so a caught error
// compares by identity the way a JavaScript object does, and it implements the Go
// error interface so it round-trips through panic and recover and reads cleanly if
// it ever reaches Go-level logging.
type Error struct {
	name    BStr
	message BStr
	// cause is the original Go error behind a boundary failure, kept so a caught
	// error preserves Go's identity-based error handling (16_go_interop.md section
	// 7.7). A go: call that returns a non-nil error raises a bridge value that
	// wraps the Go error, and Caught pulls that error out and stores it here, so
	// err.is(sentinel) and err.as(target) reach the real error with errors.Is and
	// errors.As. It is nil for an error the program threw itself (new Error(...)),
	// which has no Go error behind it, and a nil cause is simply not a match for
	// any target, so Is and As stay false rather than special-case the absence.
	cause error
	// boxed is the dynamic-object view of the error, built once by ToValue and
	// reused after, so a caught error passed into the dynamic world keeps a stable
	// object identity: e === e holds because both boxings return the same *Object
	// pointer. It is nil until the first ToValue, since an error that never leaves
	// the typed paths (a catch that only reads .name or rethrows) needs no object.
	boxed *Object
	// thrown carries the original JavaScript primitive a `throw <primitive>` raised,
	// so a catch that boxes the binding into the dynamic world reads back the
	// primitive rather than the {name, message} object the runtime models a throw
	// with. JavaScript binds `throw "reason"` as the string itself, so `e === "reason"`
	// and `typeof e === "string"` must hold; the runtime keeps a name and a message on
	// every thrown value for the uncaught reporter, so it also stashes the primitive
	// here and hands it back through ToValue. It is set only for a thrown primitive;
	// an error the program threw with new Error(...) leaves hasThrown false and boxes
	// as the object.
	thrown    Value
	hasThrown bool
	// errors is the aggregated reasons an AggregateError carries, the errors property
	// Promise.any rejects with when every input rejects. It is nil for an ordinary
	// error, which has no errors property, and a non-nil slice (empty included) marks
	// the error as an aggregate, so ToValue exposes an errors array on the boxed object.
	errors []Value
}

// Name reports the error's constructor name as a bento string, the lowering of
// JavaScript's err.name.
func (e *Error) Name() BStr { return e.name }

// Message reports the error's message as a bento string, the lowering of
// JavaScript's err.message.
func (e *Error) Message() BStr { return e.message }

// ErrorName reports the error's constructor name as a Go string, the form the
// Thrown marker and the top-level reporter read.
func (e *Error) ErrorName() string { return e.name.ToGoString() }

// ErrorMessage reports the error's message as a Go string, the form the Thrown
// marker and the top-level reporter read.
func (e *Error) ErrorMessage() string { return e.message.ToGoString() }

// Error formats the error the way JavaScript's Error.prototype.toString does: the
// name, then ": " and the message when the message is non-empty, or just the name
// when it is empty.
func (e *Error) Error() string {
	name := e.name.ToGoString()
	msg := e.message.ToGoString()
	if msg == "" {
		return name
	}
	return name + ": " + msg
}

// ToBStr returns the error's JavaScript string form as a bento string, the result
// Error.prototype.toString produces: the name, then ": " and the message when the
// message is non-empty, or the name alone when it is empty. It is the coercion a
// caught error takes in a template, a concatenation, or String(err), the bento
// string form of the Go-string Error method above so the coercion needs no
// re-transcoding.
func (e *Error) ToBStr() BStr {
	if e.message.Length() == 0 {
		return e.name
	}
	return e.name.ConcatN(FromGoString(": "), e.message)
}

// ToValue boxes the error as a dynamic object value, the form a caught error
// takes when it flows into the dynamic world rather than through a typed read: it
// is passed to a helper that takes any, compared for identity, or tested for
// truthiness. The object carries the two own properties every error exposes, name
// and message, so a dynamic read of either resolves through the boxed object's Get
// the way JavaScript's own property lookup does. The object is built once and kept
// on the error, so two boxings return the same pointer and identity holds: a caught
// error stashed and compared to itself is === true, matching an object's reference
// equality. A dynamic .constructor read on the boxed form is a later slice; the
// direct thrown.constructor read stays on its own typed path.
func (e *Error) ToValue() Value {
	// A caught thrown primitive boxes back to the primitive itself, not the
	// {name, message} object: JavaScript binds `throw "reason"` as the string, so the
	// dynamic world must see the string for `e === "reason"` and typeof e to hold.
	if e.hasThrown {
		return e.thrown
	}
	if e.boxed == nil {
		keys := []BStr{FromGoString("name"), FromGoString("message")}
		descs := []descriptor{
			defaultDataProperty(StringValue(e.name)),
			defaultDataProperty(StringValue(e.message)),
		}
		// An AggregateError also exposes an errors array, the rejection reasons
		// Promise.any collected, so a catch reads err.errors and its indices the way it
		// reads name and message. A non-aggregate error leaves errors nil and carries
		// only the two base properties.
		if e.errors != nil {
			keys = append(keys, FromGoString("errors"))
			descs = append(descs, defaultDataProperty(NewArrayValue(e.errors)))
		}
		e.boxed = &Object{kind: KindObject, keys: keys, descs: descs}
	}
	return Value{kind: KindObject, ref: unsafe.Pointer(e.boxed)}
}

// IsA reports whether the error is an instance of the named built-in error
// constructor, the lowering of e instanceof Error and its TypeError and
// RangeError siblings on a caught error. Every built-in error is an Error, so the
// base name always matches; a specific name matches only the error the matching
// constructor built, which is how instanceof narrows a caught error to the
// subclass a catch handles. The runtime models the error family as one type with
// a name field rather than distinct Go types, so the test is a name comparison
// rather than a type assertion.
func (e *Error) IsA(name string) bool {
	if name == "Error" {
		return true
	}
	return e.name.ToGoString() == name
}

// NewError constructs a plain Error, the lowering of new Error(message).
func NewError(message BStr) *Error {
	return &Error{name: FromGoString("Error"), message: message}
}

// NewTypeError constructs a TypeError, the lowering of new TypeError(message) and
// the error a failed type guard raises.
func NewTypeError(message BStr) *Error {
	return &Error{name: FromGoString("TypeError"), message: message}
}

// NewRangeError constructs a RangeError, the lowering of new RangeError(message)
// and the error a numeric range check raises.
func NewRangeError(message BStr) *Error {
	return &Error{name: FromGoString("RangeError"), message: message}
}

// NewAggregateError constructs an AggregateError, the error Promise.any rejects with
// when every input rejects. It carries the rejection reasons as its errors array and a
// summary message, and marks itself an aggregate through a non-nil errors slice so the
// boxed object exposes the errors property alongside name and message.
func NewAggregateError(errors []Value, message BStr) *Error {
	return &Error{name: FromGoString("AggregateError"), message: message, errors: errors}
}

// NewSyntaxError constructs a SyntaxError, the error a runtime parse raises: a
// BigInt(s) whose string is not an integer literal, or a JSON.parse on malformed
// input once its throw path lands.
func NewSyntaxError(message BStr) *Error {
	return &Error{name: FromGoString("SyntaxError"), message: message}
}

// NewURIError constructs a URIError, the error the URI codec globals raise: an
// encodeURIComponent over a lone surrogate, or a decodeURIComponent over a
// malformed percent-escape.
func NewURIError(message BStr) *Error {
	return &Error{name: FromGoString("URIError"), message: message}
}

// NewInvalidCharacterError constructs an InvalidCharacterError, the DOMException
// the base64 globals raise: a btoa over a code unit above the Latin1 range, or an
// atob over base64 that is the wrong length or holds a character outside the
// alphabet. It is a DOMException rather than an ECMAScript error, but the runtime
// models every thrown error as a name and a message, so a catch reads err.name as
// "InvalidCharacterError" the way it would in Node.
func NewInvalidCharacterError() *Error {
	return &Error{name: FromGoString("InvalidCharacterError"), message: FromGoString("Invalid character")}
}

// Caught converts a recovered panic payload into the *Error a catch binds. A
// thrown *Error binds unchanged, so identity is preserved; a boundary Thrown (a
// go: failure, a range check) binds as an Error carrying its name and message, so a
// catch handles it like any other error. A payload that is not a thrown value is a
// Go runtime panic, a bug in the runtime rather than a program throw, so it is
// re-panicked to keep its original stack rather than be caught as a JavaScript
// error.
func Caught(r any) *Error {
	switch t := r.(type) {
	case *Error:
		return t
	case ThrownString:
		// A thrown primitive string binds as the string itself, so the catch reads a
		// primitive: `e === "reason"` and typeof e === "string" hold. The runtime still
		// keeps the string as the name so an uncaught rethrow reports it, but ToValue
		// hands back the primitive rather than the {name, message} object.
		return &Error{
			name:      FromGoString(t.ErrorName()),
			message:   FromGoString(t.ErrorMessage()),
			thrown:    StringValue(BStr(t)),
			hasThrown: true,
		}
	case ThrownValue:
		// A thrown non-error value binds as the value itself, so the catch reads the
		// primitive or object the program raised: `throw 42` binds 42 and `e === 42`
		// holds, `throw {}` binds the object. The runtime keeps the value's String form
		// as the name so an uncaught rethrow reports it, but ToValue hands back the
		// value rather than the {name, message} object, the way the string case does.
		return &Error{
			name:      FromGoString(t.ErrorName()),
			message:   FromGoString(t.ErrorMessage()),
			thrown:    t.Value(),
			hasThrown: true,
		}
	case Thrown:
		e := &Error{name: FromGoString(t.ErrorName()), message: FromGoString(t.ErrorMessage())}
		// A boundary failure that wraps a Go error (the GoError a go: call raises)
		// hands the original error through Unwrap, so the caught error keeps a live
		// handle to it and err.is/err.as can branch on identity (section 7.7). A
		// boundary Thrown with no Go error behind it (a RangeError from the number
		// check) unwraps to nil and leaves cause unset.
		if u, ok := t.(interface{ Unwrap() error }); ok {
			e.cause = u.Unwrap()
		}
		return e
	default:
		panic(r)
	}
}

// Cause reports the original Go error behind a caught boundary failure, or nil for
// an error the program threw itself. It is the value.Value-side handle behind a
// caught error's goError property (section 7.7): the underlying Go value that
// errors.Is and errors.As walk, kept alive by the caught error that holds it.
func (e *Error) Cause() error { return e.cause }

// IsGoError reports whether the caught error came from Go, the lowering of
// e instanceof GoError on a catch binding (section 7.7). A boundary failure that
// wrapped a Go error carries a cause, so it is a GoError; an error the program
// threw itself has no Go error behind it and is not. This is what narrows a catch
// binding to the GoError surface before err.is or err.as reads it.
func (e *Error) IsGoError() bool { return e.cause != nil }

// Is reports whether the caught error matches a Go sentinel, the lowering of
// err.is(target) where target is an error imported from a go: package (io.EOF is
// the canonical one, section 7.7). It defers to errors.Is against the original Go
// error, so a wrapped sentinel matches exactly as it would in Go; an error the
// program threw itself has no Go error behind it and matches nothing.
func (e *Error) Is(target error) bool {
	return e.cause != nil && errors.Is(e.cause, target)
}

// As unwraps the caught error's Go error into target, the lowering of err.as(...)
// (section 7.7). It defers to errors.As, so target is a pointer to the concrete Go
// error type the chain is searched for, and it reports whether a match was
// assigned; an error the program threw itself has no Go error to unwrap and never
// matches.
func (e *Error) As(target any) bool {
	return e.cause != nil && errors.As(e.cause, target)
}

// ThrownString is a thrown primitive string, the `throw "reason"` JavaScript
// allows beside thrown errors. It carries the Thrown surface with the string
// as the name and no message, so the uncaught reporter prints the string the
// way node reports a thrown primitive. A catch that recovers one binds an
// *Error that stashes the string, so a dynamic read of the binding boxes back
// to the string primitive: `e === "reason"` and typeof e === "string" hold the
// way a JavaScript catch binds the primitive itself.
type ThrownString BStr

// ErrorName reports the thrown string itself, the text the reporter prints.
func (t ThrownString) ErrorName() string { return BStr(t).ToGoString() }

// ErrorMessage reports an empty message; a thrown primitive has none.
func (t ThrownString) ErrorMessage() string { return "" }

// ThrownValue is a thrown JavaScript value that is neither a built-in error nor a
// primitive string: a number, a boolean, null, undefined, or an object the program
// raised with `throw <expr>`. JavaScript allows any value to be thrown, so the
// runtime models the general case with one carrier that boxes the raised value and
// carries the Thrown surface with the value's String coercion as the name and no
// message, so the uncaught reporter prints it the way the engine spells a thrown
// value: `throw 7` reports "Uncaught 7" and `throw {}` reports "Uncaught
// [object Object]". A catch that recovers one binds the value itself, so a dynamic
// read of the binding sees the original value: throwing 42 and catching it holds
// `e === 42` and `typeof e === "number"` the way a JavaScript catch binds the value.
type ThrownValue struct{ v Value }

// NewThrownValue wraps a raised value in the Thrown carrier, the payload a
// `throw <expr>` over a non-error, non-string value lowers to.
func NewThrownValue(v Value) ThrownValue { return ThrownValue{v: v} }

// ErrorName reports the value's String coercion, the text the reporter prints
// after "Uncaught ": a number spells its digits, an object its "[object Object]"
// tag, null and undefined their literal words.
func (t ThrownValue) ErrorName() string { return ToString(t.v).ToGoString() }

// ErrorMessage reports an empty message; a thrown value carries none.
func (t ThrownValue) ErrorMessage() string { return "" }

// Value reports the wrapped value, the primitive or object a catch binds so the
// binding reads back as the value the program threw.
func (t ThrownValue) Value() Value { return t.v }

// Throw raises a thrown value so an enclosing catch recovers it or the top-level
// reporter surfaces it. The payload is anything carrying the Thrown surface: the
// runtime's own *Error, or a program class whose thrown instances gained
// ErrorName and ErrorMessage in emission. It is a named entry point so every
// throw lowers to one call shape rather than an inline panic, which keeps the
// generated code readable and gives the runtime one place to evolve the throw
// path.
func Throw(e Thrown) {
	panic(e)
}

// ReportUncaught is deferred at the program root to surface a throw that escaped
// every catch. It recovers the panic, and if the payload is a thrown JavaScript
// value it prints an uncaught-error line to standard error and exits non-zero, the
// way a runtime reports an unhandled exception. A payload that is not a thrown
// value is a Go runtime panic, a bug in the runtime rather than a program throw,
// so it is re-panicked to keep its original stack. A run that did not panic
// recovers nothing and returns, leaving a clean exit untouched.
func ReportUncaught() {
	r := recover()
	if r == nil {
		return
	}
	t, ok := r.(Thrown)
	if !ok {
		panic(r)
	}
	line := "Uncaught " + t.ErrorName()
	if msg := t.ErrorMessage(); msg != "" {
		line += ": " + msg
	}
	_, _ = os.Stderr.WriteString(line + "\n")
	os.Exit(1)
}
