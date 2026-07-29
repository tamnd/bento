package value

import "testing"

// require('util') used to be a throw-on-use stub, so a program that called
// util.format got an error naming the module rather than a formatted string. These
// pin what it answers now, and pin that the members it still does not carry keep
// saying so rather than reading undefined.

// TestUtilModuleFormats is the reason the module exists: the three members bento
// carries answer through it.
func TestUtilModuleFormats(t *testing.T) {
	util := RequireBuiltin("util")
	call := func(member string, args ...Value) string {
		fn := util.Get(FromGoString(member))
		if fn.Kind() != KindFunc {
			t.Fatalf("util.%s is %v, want a function", member, fn.Kind())
		}
		return ToString(fn.Call(args...)).ToGoString()
	}
	str := func(s string) Value { return StringValue(FromGoString(s)) }
	obj := NewObject()
	obj.Set(FromGoString("a"), Number(1))

	if got := call("format", str("%s is %d"), str("x"), Number(2)); got != "x is 2" {
		t.Errorf(`util.format("%%s is %%d", "x", 2) = %q, want x is 2`, got)
	}
	if got := call("inspect", obj); got != "{ a: 1 }" {
		t.Errorf("util.inspect({a:1}) = %q, want { a: 1 }", got)
	}
	sep := NewObject()
	sep.Set(FromGoString("numericSeparator"), True)
	if got := call("formatWithOptions", sep, str("%d"), Number(1234567)); got != "1_234_567" {
		t.Errorf("util.formatWithOptions(numericSeparator) = %q, want 1_234_567", got)
	}
}

// TestUtilIsOneModuleUnderBothSpecifiers pins the identity require gives a built-in:
// the bare and the node: form name the same module, so a program that stores one and
// compares it to the other sees one value.
func TestUtilIsOneModuleUnderBothSpecifiers(t *testing.T) {
	if !StrictEquals(RequireBuiltin("util"), RequireBuiltin("node:util")) {
		t.Error(`require("util") !== require("node:util")`)
	}
}

// TestAMissingUtilMemberSaysSo is the honest-stub rule applied to a module that is
// only partly there. promisify is not implemented, and a program reaching for it
// should learn that from the read rather than from calling undefined later.
func TestAMissingUtilMemberSaysSo(t *testing.T) {
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("reading util.promisify did not throw")
		}
		msg := Caught(rec.(Thrown)).ErrorMessage()
		want := "The built-in module 'util' is registered but not implemented in bento yet (reading 'promisify')"
		if msg != want {
			t.Errorf("thrown message is %q, want %q", msg, want)
		}
	}()
	RequireBuiltin("util").Get(FromGoString("promisify"))
}

// TestASymbolReadOfAPartialModuleIsAnswered pins the exception to that rule. A
// symbol-keyed read is the language looking for a hook rather than a program reaching
// for a member, and throwing on one would make an unimplemented module impossible to
// spread or coerce instead of merely unusable.
func TestASymbolReadOfAPartialModuleIsAnswered(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("a symbol read of the module threw %v", rec)
		}
	}()
	if got := RequireBuiltin("util").GetElem(SymbolToPrimitive()); got.Kind() != KindUndefined {
		t.Errorf("util[Symbol.toPrimitive] is %v, want undefined", got.Kind())
	}
}
