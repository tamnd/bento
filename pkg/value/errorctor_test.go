package value

import "testing"

// TestErrorConstructorName pins that a built-in constructor value answers its name
// through a dynamic property read, the .name assert.throws reads for its message.
func TestErrorConstructorName(t *testing.T) {
	c := ErrorConstructor("TypeError")
	got := c.Get(FromGoString("name"))
	if got.Kind() != KindString || got.AsString().ToGoString() != "TypeError" {
		t.Fatalf("ErrorConstructor(TypeError).name = %v, want string TypeError", got)
	}
}

// TestErrorConstructorTypeof pins typeof "function" for a constructor value, the
// tag JavaScript reports for a callable.
func TestErrorConstructorTypeof(t *testing.T) {
	c := ErrorConstructor("RangeError")
	if got := c.TypeOf().ToGoString(); got != "function" {
		t.Fatalf("typeof ErrorConstructor = %q, want function", got)
	}
}

// TestErrorConstructorIdentity pins that two references to the same built-in
// constructor compare equal under ===, and two different ones do not, the identity
// assert.throws leans on when it checks a caught error's constructor against the
// expected one.
func TestErrorConstructorIdentity(t *testing.T) {
	if !StrictEquals(ErrorConstructor("TypeError"), ErrorConstructor("TypeError")) {
		t.Fatal("TypeError === TypeError was false")
	}
	if StrictEquals(ErrorConstructor("TypeError"), ErrorConstructor("RangeError")) {
		t.Fatal("TypeError === RangeError was true")
	}
}

// TestCaughtErrorConstructor pins that a caught error reports the interned
// constructor for its name, so thrown.constructor === TypeError holds for a thrown
// TypeError.
func TestCaughtErrorConstructor(t *testing.T) {
	e := NewTypeError(FromGoString("boom"))
	if !StrictEquals(e.Constructor(), ErrorConstructor("TypeError")) {
		t.Fatal("caught TypeError's constructor was not the TypeError value")
	}
	if got := e.Constructor().Get(FromGoString("name")).AsString().ToGoString(); got != "TypeError" {
		t.Fatalf("caught error constructor name = %q, want TypeError", got)
	}
}

// TestErrorConstructorUnknownName pins that a name outside the built-in set still
// answers its name, the graceful path for a custom error until the class slice
// interns user constructors.
func TestErrorConstructorUnknownName(t *testing.T) {
	c := ErrorConstructor("MyError")
	if got := c.Get(FromGoString("name")).AsString().ToGoString(); got != "MyError" {
		t.Fatalf("ErrorConstructor(MyError).name = %q, want MyError", got)
	}
}
