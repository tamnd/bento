package lower

import (
	"strings"
	"testing"
)

// A generic function has no single Go form, so bento monomorphizes it: one Go
// function per distinct concrete type argument its call sites fix it to, named
// with a suffix mangled from those types (Identity_num, Identity_str), and each
// call rewritten to the specialization it resolves to. The body of a
// specialization lowers with the type parameter resolved to the concrete type, so
// a bare T reads as the float64 or value.BStr the call fixed.

// TestGenericFunctionMonomorphizesPerTypeArgument proves a generic called with two
// distinct type arguments emits two specialized Go functions with the concrete Go
// types, and no unspecialized generic func is left behind.
func TestGenericFunctionMonomorphizesPerTypeArgument(t *testing.T) {
	const src = "function identity<T>(x: T): T { return x; }\n" +
		"console.log(identity(5));\n" +
		"console.log(identity(\"hi\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Identity_num(x float64) float64") {
		t.Errorf("the number instantiation was not emitted with a float64 signature:\n%s", source)
	}
	if !strings.Contains(source, "func Identity_str(x value.BStr) value.BStr") {
		t.Errorf("the string instantiation was not emitted with a value.BStr signature:\n%s", source)
	}
	if strings.Contains(source, "func Identity(") {
		t.Errorf("an unspecialized generic function was emitted:\n%s", source)
	}
}

// TestGenericCallResolvesToSpecialization proves each call site is rewritten to the
// specialized Go name the type arguments fix, not the bare exported name.
func TestGenericCallResolvesToSpecialization(t *testing.T) {
	const src = "function identity<T>(x: T): T { return x; }\n" +
		"console.log(identity(5));\n" +
		"console.log(identity(\"hi\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Identity_num(5)") {
		t.Errorf("the number call was not rewritten to the specialization:\n%s", source)
	}
	if !strings.Contains(source, "Identity_str(value.FromGoString(\"hi\"))") {
		t.Errorf("the string call was not rewritten to the specialization:\n%s", source)
	}
}

// TestGenericLiteralArgumentsShareOneSpecialization proves two calls whose literal
// arguments widen to one Go type collapse to a single specialization, the
// dedup-at-the-lowered-type rule, so a type parameter named twice is fixed cleanly.
func TestGenericLiteralArgumentsShareOneSpecialization(t *testing.T) {
	const src = "function firstOf<T>(a: T, b: T): T { return a; }\n" +
		"console.log(firstOf(10, 20));\n" +
		"console.log(firstOf(1, 2));\n"
	source := renderProgram(t, src)
	if got := strings.Count(source, "func FirstOf_num("); got != 1 {
		t.Errorf("expected one number specialization, got %d:\n%s", got, source)
	}
}

// TestGenericFunctionRuns builds and runs two instantiations of a generic function
// so the monomorphized Go is proven against the JavaScript result.
func TestGenericFunctionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function identity<T>(x: T): T {
  return x;
}
function firstOf<T>(a: T, b: T): T {
  return a;
}
console.log(identity(5));
console.log(identity("hi"));
console.log(firstOf(true, false));
console.log(firstOf(10, 20));
`
	if got, want := runProgramGo(t, src), "5\nhi\ntrue\n10\n"; got != want {
		t.Fatalf("monomorphized generics printed %q, want %q", got, want)
	}
}

// TestUncalledGenericHandsBack proves a generic no call site instantiates has no
// specialization to emit and hands back, since an unspecialized generic has no
// single Go form. The whole program routes to the engine rather than emit an
// unsound unspecialized func.
func TestUncalledGenericHandsBack(t *testing.T) {
	const src = "function unused<T>(x: T): T { return x; }\n" +
		"console.log(1);\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "generic") {
		t.Fatalf("uncalled generic hand-back reason = %q, want a generic reason", reason)
	}
}
