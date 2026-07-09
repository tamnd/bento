package value

import (
	"errors"
	"fmt"
	"testing"
)

// goErrorStub stands in for the bridge value a failed go: call raises: a Thrown
// that wraps the original Go error, so Caught can pull the error out through
// Unwrap. The value package cannot import the bridge (the bridge imports it), so
// the test models the one thing Caught depends on, the Unwrap method, directly.
type goErrorStub struct{ err error }

func (g goErrorStub) ErrorName() string    { return "Error" }
func (g goErrorStub) ErrorMessage() string { return g.err.Error() }
func (g goErrorStub) Unwrap() error        { return g.err }

// concreteGoError is a projected concrete Go error type, the kind err.as unwraps
// the chain to. It carries a field so the test can read the recovered value.
type concreteGoError struct{ code int }

func (c *concreteGoError) Error() string { return fmt.Sprintf("go error %d", c.code) }

// TestCaughtPreservesGoErrorIdentity proves a caught boundary failure keeps a live
// handle to the Go error behind it, so err.is matches the sentinel the go: call
// wrapped exactly as errors.Is would in Go, and a different sentinel does not
// match. This is the identity section 7.7 keeps usable across the boundary.
func TestCaughtPreservesGoErrorIdentity(t *testing.T) {
	sentinel := errors.New("EOF")
	other := errors.New("closed")
	raised := goErrorStub{err: fmt.Errorf("read failed: %w", sentinel)}

	caught := Caught(raised)
	if caught.Cause() == nil {
		t.Fatal("caught error dropped the Go cause, want the wrapped error preserved")
	}
	if !caught.Is(sentinel) {
		t.Error("err.is(sentinel) = false, want true for the wrapped sentinel")
	}
	if caught.Is(other) {
		t.Error("err.is(other) = true, want false for an unrelated sentinel")
	}
	if caught.ErrorMessage() != "read failed: EOF" {
		t.Errorf("message = %q, want the Go error string", caught.ErrorMessage())
	}
}

// TestCaughtAsUnwrapsConcreteError proves err.as pulls a projected concrete error
// out of the chain, the errors.As half of section 7.7, and reports false with no
// assignment when the chain holds no such type.
func TestCaughtAsUnwrapsConcreteError(t *testing.T) {
	raised := goErrorStub{err: fmt.Errorf("wrapped: %w", &concreteGoError{code: 42})}
	caught := Caught(raised)

	var target *concreteGoError
	if !caught.As(&target) {
		t.Fatal("err.as(concrete) = false, want true when the chain holds it")
	}
	if target == nil || target.code != 42 {
		t.Errorf("as assigned %+v, want the concrete error with code 42", target)
	}

	var missing *concreteGoError
	plain := Caught(goErrorStub{err: errors.New("no concrete type here")})
	if plain.As(&missing) {
		t.Error("err.as matched a chain without the concrete type, want false")
	}
}

// TestProgramThrownErrorHasNoCause proves an error the program threw itself carries
// no Go error, so is and as never match: there is nothing behind a new Error(...)
// to compare against a Go sentinel, and the runtime leaves the cause nil rather
// than invent one.
func TestProgramThrownErrorHasNoCause(t *testing.T) {
	thrown := NewError(FromGoString("boom"))
	if thrown.Cause() != nil {
		t.Error("a program-thrown error has a Go cause, want nil")
	}
	if thrown.Is(errors.New("x")) {
		t.Error("err.is matched on a program-thrown error, want false")
	}
	var target *concreteGoError
	if thrown.As(&target) {
		t.Error("err.as matched on a program-thrown error, want false")
	}
}

// TestCaughtIdentityRoundTripsUnchanged proves a thrown *Error binds unchanged, so
// a rethrow keeps its cause: an error caught with a Go cause and rethrown is the
// same pointer with the same handle when caught again.
func TestCaughtIdentityRoundTripsUnchanged(t *testing.T) {
	sentinel := errors.New("EOF")
	first := Caught(goErrorStub{err: fmt.Errorf("read: %w", sentinel)})
	again := Caught(first)
	if again != first {
		t.Fatal("re-catching a thrown error changed its identity, want the same pointer")
	}
	if !again.Is(sentinel) {
		t.Error("rethrown error lost its Go cause, want is(sentinel) true")
	}
}

// TestErrorFormatsLikeJavaScript checks the string an error renders to matches
// Error.prototype.toString: name and message joined by ": ", or the bare name when
// the message is empty.
func TestErrorFormatsLikeJavaScript(t *testing.T) {
	if got := NewTypeError(FromGoString("not a function")).Error(); got != "TypeError: not a function" {
		t.Errorf("error text = %q, want %q", got, "TypeError: not a function")
	}
	if got := NewError(FromGoString("")).Error(); got != "Error" {
		t.Errorf("empty-message error text = %q, want %q", got, "Error")
	}
}

// TestConstructorsCarryTheirName checks each constructor stamps the JavaScript
// error name its new expression names, since a catch and the reporter tell errors
// apart by name.
func TestConstructorsCarryTheirName(t *testing.T) {
	cases := []struct {
		err  *Error
		name string
	}{
		{NewError(FromGoString("x")), "Error"},
		{NewTypeError(FromGoString("x")), "TypeError"},
		{NewRangeError(FromGoString("x")), "RangeError"},
	}
	for _, tc := range cases {
		if tc.err.ErrorName() != tc.name {
			t.Errorf("ErrorName = %q, want %q", tc.err.ErrorName(), tc.name)
		}
		if tc.err.ErrorMessage() != "x" {
			t.Errorf("ErrorMessage = %q, want %q", tc.err.ErrorMessage(), "x")
		}
	}
}

// TestErrorIsThrown proves a constructed error satisfies the Thrown marker, so the
// top-level reporter and a catch recognize it as a deliberate throw rather than a
// Go runtime panic.
func TestErrorIsThrown(t *testing.T) {
	var _ Thrown = NewError(FromGoString("x"))
	var _ error = NewError(FromGoString("x"))
}

// TestCaughtThrownStringBoxesBackToTheString proves a caught thrown primitive
// string reads as the string primitive in the dynamic world, the way a JavaScript
// catch binds `throw "reason"` as the string itself: e === "reason" holds and
// typeof e is string. The runtime models every thrown value as a name and a
// message for the uncaught reporter, so before this the caught binding boxed as a
// {name, message} object and e === "reason" was false. ToValue now hands back the
// stashed primitive so the strict compare folds to true.
func TestCaughtThrownStringBoxesBackToTheString(t *testing.T) {
	caught := Caught(ThrownString(FromGoString("NoInExpression")))
	boxed := caught.ToValue()
	if boxed.Kind() != KindString {
		t.Fatalf("caught thrown string boxed as kind %v, want a string primitive", boxed.Kind())
	}
	if !StrictEquals(boxed, StringValue(FromGoString("NoInExpression"))) {
		t.Error("e === \"NoInExpression\" is false, want true for a caught thrown string")
	}
	if StrictEquals(boxed, StringValue(FromGoString("other"))) {
		t.Error("e === \"other\" is true, want false for an unequal string")
	}
}

// TestCaughtThrownErrorStillBoxesAsObject proves the primitive fidelity does not
// leak into a real thrown error: a boundary Thrown that is not a primitive string
// still boxes as the {name, message} object, so a dynamic .name read resolves and
// two boxings keep one identity.
func TestCaughtThrownErrorStillBoxesAsObject(t *testing.T) {
	caught := Caught(goErrorStub{err: errors.New("boom")})
	boxed := caught.ToValue()
	if boxed.Kind() != KindObject {
		t.Fatalf("caught boundary error boxed as kind %v, want an object", boxed.Kind())
	}
	if again := caught.ToValue(); again.ref != boxed.ref {
		t.Error("two boxings of a caught error returned different objects, want one identity")
	}
}

// TestThrowPanicsWithTheError proves Throw raises the error as the panic payload,
// unchanged, so a recover downstream gets the same *Error to bind into a catch.
func TestThrowPanicsWithTheError(t *testing.T) {
	want := NewRangeError(FromGoString("out of range"))
	defer func() {
		got := recover()
		if got != want {
			t.Fatalf("recovered %v, want the thrown error %v", got, want)
		}
	}()
	Throw(want)
	t.Fatal("Throw returned instead of panicking")
}

// TestReportUncaughtRepanicsNonThrown proves the reporter leaves a Go runtime
// panic alone: a payload that is not a thrown value is re-panicked so its original
// crash and stack survive rather than being dressed up as a caught error. The
// deliberate-throw path exits the process, so it is covered end to end by the
// lowering test that runs a throwing program rather than here.
func TestReportUncaughtRepanicsNonThrown(t *testing.T) {
	defer func() {
		if got := recover(); got != "a runtime bug" {
			t.Fatalf("re-panicked %v, want the original payload", got)
		}
	}()
	func() {
		defer ReportUncaught()
		panic("a runtime bug")
	}()
	t.Fatal("ReportUncaught swallowed a non-thrown panic")
}
