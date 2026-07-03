package value

import "os"

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
// message, the two fields every built-in error carries. It is a pointer type so a
// caught error compares by identity the way a JavaScript object does, and it
// implements the Go error interface so it round-trips through panic and recover
// and reads cleanly if it ever reaches Go-level logging.
type Error struct {
	Name    string
	Message string
}

// ErrorName reports the error's constructor name (Error, TypeError, RangeError),
// the value JavaScript's err.name exposes.
func (e *Error) ErrorName() string { return e.Name }

// ErrorMessage reports the error's message, the value JavaScript's err.message
// exposes.
func (e *Error) ErrorMessage() string { return e.Message }

// Error formats the error the way JavaScript's Error.prototype.toString does: the
// name, then ": " and the message when the message is non-empty, or just the name
// when it is empty.
func (e *Error) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

// NewError constructs a plain Error, the lowering of new Error(message). The
// message is a bento string transcoded to Go, the same crossing every string
// takes into a Go field.
func NewError(message BStr) *Error {
	return &Error{Name: "Error", Message: message.ToGoString()}
}

// NewTypeError constructs a TypeError, the lowering of new TypeError(message) and
// the error a failed type guard raises.
func NewTypeError(message BStr) *Error {
	return &Error{Name: "TypeError", Message: message.ToGoString()}
}

// NewRangeError constructs a RangeError, the lowering of new RangeError(message)
// and the error a numeric range check raises.
func NewRangeError(message BStr) *Error {
	return &Error{Name: "RangeError", Message: message.ToGoString()}
}

// Throw raises a thrown error so an enclosing catch recovers it or the top-level
// reporter surfaces it. It is a named entry point so every throw lowers to one
// call shape rather than an inline panic, which keeps the generated code readable
// and gives the runtime one place to evolve the throw path.
func Throw(e *Error) {
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
