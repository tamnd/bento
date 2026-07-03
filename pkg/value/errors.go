package value

import (
	"errors"
	"os"
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
