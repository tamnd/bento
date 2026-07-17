package value

import "testing"

// A using declaration's disposal runs at scope exit through Dispose, which threads the
// explicit-resource-management error semantics through Go's panic unwinding: a throw
// from the release wraps the error already propagating in a SuppressedError, a clean
// release re-raises that error, and a release throw with nothing pending propagates on
// its own. These tests drive Dispose the way the generated code does, deferred inside a
// scope that panics, and read the resulting throw a catch would bind.

// recoverThrow runs fn and returns the JavaScript throw it raised, or nil if it did
// not throw, the recover a catch performs.
func recoverThrow(fn func()) (thrown Thrown) {
	defer func() {
		if r := recover(); r != nil {
			thrown = r.(Thrown)
		}
	}()
	fn()
	return nil
}

// TestDisposeWrapsPendingThrowWhenReleaseThrows proves the suppression chain: a
// release that throws while the body's throw is unwinding raises a SuppressedError
// whose error is the release's throw and whose suppressed is the body's throw.
func TestDisposeWrapsPendingThrowWhenReleaseThrows(t *testing.T) {
	body := NewError(FromGoString("body"))
	release := NewError(FromGoString("release"))
	thrown := recoverThrow(func() {
		defer Dispose(func() { Throw(release) })
		Throw(body)
	})
	se, ok := thrown.(*Error)
	if !ok || se.ErrorName() != "SuppressedError" {
		t.Fatalf("Dispose raised %#v, want a SuppressedError", thrown)
	}
	boxed := se.ToValue()
	if got := boxed.Get(FromGoString("error")); got.ref != release.ToValue().ref {
		t.Error("SuppressedError.error is not the release throw")
	}
	if got := boxed.Get(FromGoString("suppressed")); got.ref != body.ToValue().ref {
		t.Error("SuppressedError.suppressed is not the body throw")
	}
}

// TestDisposeReRaisesPendingThrowWhenReleaseClean proves a clean release re-raises the
// error the scope was already unwinding, unchanged and unwrapped.
func TestDisposeReRaisesPendingThrowWhenReleaseClean(t *testing.T) {
	body := NewError(FromGoString("body"))
	thrown := recoverThrow(func() {
		defer Dispose(func() {})
		Throw(body)
	})
	if thrown != Thrown(body) {
		t.Fatalf("Dispose raised %#v, want the body throw unchanged", thrown)
	}
}

// TestDisposeRaisesReleaseThrowWhenNothingPending proves a release throw with no error
// already unwinding propagates on its own, not wrapped in a SuppressedError.
func TestDisposeRaisesReleaseThrowWhenNothingPending(t *testing.T) {
	release := NewError(FromGoString("release"))
	thrown := recoverThrow(func() {
		defer Dispose(func() { Throw(release) })
	})
	if thrown != Thrown(release) {
		t.Fatalf("Dispose raised %#v, want the release throw unwrapped", thrown)
	}
}

// TestDisposeCleanReleaseCleanBodyRaisesNothing proves the ordinary path: a clean
// release with no pending throw raises nothing, so a scope that neither the body nor
// the release threw from exits clean.
func TestDisposeCleanReleaseCleanBodyRaisesNothing(t *testing.T) {
	ran := false
	thrown := recoverThrow(func() {
		defer Dispose(func() { ran = true })
	})
	if thrown != nil {
		t.Fatalf("Dispose raised %#v, want nothing", thrown)
	}
	if !ran {
		t.Error("Dispose did not run the release")
	}
}

// TestDisposeChainsNestedSuppression proves two releases that each throw while an
// error unwinds nest the SuppressedErrors: the outer error is the last release's
// throw, and its suppressed is the SuppressedError the inner release built.
func TestDisposeChainsNestedSuppression(t *testing.T) {
	body := NewError(FromGoString("body"))
	inner := NewError(FromGoString("inner"))
	outer := NewError(FromGoString("outer"))
	thrown := recoverThrow(func() {
		// outer registered first runs last, the reverse order two using
		// declarations dispose in.
		defer Dispose(func() { Throw(outer) })
		defer Dispose(func() { Throw(inner) })
		Throw(body)
	})
	se, ok := thrown.(*Error)
	if !ok || se.ErrorName() != "SuppressedError" {
		t.Fatalf("Dispose raised %#v, want a SuppressedError", thrown)
	}
	boxed := se.ToValue()
	if got := boxed.Get(FromGoString("error")); got.ref != outer.ToValue().ref {
		t.Error("outer SuppressedError.error is not the last release throw")
	}
	nested := boxed.Get(FromGoString("suppressed"))
	if got := nested.Get(FromGoString("error")); got.ref != inner.ToValue().ref {
		t.Error("nested SuppressedError.error is not the first release throw")
	}
	if got := nested.Get(FromGoString("suppressed")); got.ref != body.ToValue().ref {
		t.Error("nested SuppressedError.suppressed is not the body throw")
	}
}
