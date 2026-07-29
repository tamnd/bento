package value

import "testing"

// Every error a Node built-in raises carries a code, and the code rather than the
// message is what a program branches on: the message is prose that changes between
// releases, while ERR_INVALID_ARG_TYPE and ENOENT are the contract. These pin the
// code end of that, and the "Received ..." wording an argument check reports, which
// is the part a developer reads when the throw reaches them.

// TestANodeErrorCarriesItsCode pins the code on the typed error and on the boxed
// object a catch reads it through, since a program reaches it by the second route.
func TestANodeErrorCarriesItsCode(t *testing.T) {
	e := NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE", FromGoString("bad"))
	if e.ErrorName() != "TypeError" || e.ErrorMessage() != "bad" {
		t.Errorf("error is %s: %s, want TypeError: bad", e.ErrorName(), e.ErrorMessage())
	}
	code, has := e.Code()
	if !has || code.ToGoString() != "ERR_INVALID_ARG_TYPE" {
		t.Errorf("Code() = %q, %v, want ERR_INVALID_ARG_TYPE, true", code.ToGoString(), has)
	}
	boxed := e.ToValue()
	got := boxed.Get(FromGoString("code"))
	if got.Kind() != KindString || got.AsString().ToGoString() != "ERR_INVALID_ARG_TYPE" {
		t.Errorf("boxed err.code = %v %q, want the code string", got.Kind(), ToString(got).ToGoString())
	}
	if n := boxed.Get(FromGoString("name")); ToString(n).ToGoString() != "TypeError" {
		t.Errorf("boxed err.name = %q, want TypeError", ToString(n).ToGoString())
	}
}

// TestAnOrdinaryErrorHasNoCode pins the absence. An error the program threw itself
// has no code, and that must read back as undefined rather than as the empty string:
// a program tests the code against a name, and an empty string is still a string, so
// it would compare unequal to everything while reading as present.
func TestAnOrdinaryErrorHasNoCode(t *testing.T) {
	e := NewTypeError(FromGoString("boom"))
	if _, has := e.Code(); has {
		t.Error("a plain TypeError reports a code, want none")
	}
	if got := e.CodeValue(); got.Kind() != KindUndefined {
		t.Errorf("CodeValue() = %v, want undefined", got.Kind())
	}
	if got := e.ToValue().Get(FromGoString("code")); got.Kind() != KindUndefined {
		t.Errorf("boxed err.code = %v, want undefined", got.Kind())
	}
}

// TestTheReceivedWordingMatchesNode pins how each kind of argument is described in
// an ERR_INVALID_ARG_TYPE. Node spells it three different ways and the difference
// carries information: a primitive is shown with its value, since the value is short
// and is usually the mistake, an object is described by its constructor, since
// printing it would bury the message, and null and undefined are named outright
// because "type object (null)" would read as a class of values rather than one.
func TestTheReceivedWordingMatchesNode(t *testing.T) {
	for _, c := range []struct {
		in   Value
		want string
	}{
		{Number(5), "Received type number (5)"},
		{Number(1.5), "Received type number (1.5)"},
		{True, "Received type boolean (true)"},
		{StringValue(FromGoString("x")), "Received type string ('x')"},
		{Null, "Received null"},
		{Undefined, "Received undefined"},
		{NewObject(), "Received an instance of Object"},
		{NewArrayValue(nil), "Received an instance of Array"},
		{WithName(NewFunc(func([]Value) Value { return Undefined }), "foo"), "Received function foo"},
	} {
		if got := receivedDescription(c.in); got != c.want {
			t.Errorf("receivedDescription(%v) = %q, want %q", c.in.Kind(), got, c.want)
		}
	}
}

// TestALongReceivedStringIsCut pins the trim. The parenthesized value is there to
// identify the argument rather than to reproduce it, and a message with a paragraph
// pasted into the middle of it stops being readable, which is the whole point of the
// wording.
func TestALongReceivedStringIsCut(t *testing.T) {
	long := "0123456789012345678901234567890123456789"
	got := receivedDescription(StringValue(FromGoString(long)))
	if want := "Received type string ('0123456789012345678901234...')"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestDescribingASymbolDoesNotThrow pins the one kind that would. The abstract
// ToString throws on a symbol, and throwing while building the message for another
// throw would lose the original error, so the description takes the String coercion
// that renders one instead.
func TestDescribingASymbolDoesNotThrow(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("describing a symbol argument threw %v", rec)
		}
	}()
	if got := receivedDescription(NewSymbol(FromGoString("s"))); got != "Received type symbol (Symbol(s))" {
		t.Errorf("got %q, want Received type symbol (Symbol(s))", got)
	}
}
