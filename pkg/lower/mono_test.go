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

// A Go method cannot carry a type parameter, so a generic method has no single Go
// form either: bento emits one mangled Go method per instantiation its call sites
// fix, keyed by the receiver type and the type argument, and rewrites each method
// call to the specialization it resolves to. The body of each specialization lowers
// with the type parameter resolved to the concrete type the call fixed.

// TestGenericMethodMonomorphizesPerTypeArgument proves a generic method called with
// two distinct type arguments emits two specialized Go methods with the concrete Go
// types, and no unspecialized generic method is left behind.
func TestGenericMethodMonomorphizesPerTypeArgument(t *testing.T) {
	const src = "class Box {\n" +
		"  wrap<T>(x: T): T { return x; }\n" +
		"}\n" +
		"const b = new Box();\n" +
		"console.log(b.wrap(5));\n" +
		"console.log(b.wrap(\"hi\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (b *Box) Wrap_num(x float64) float64") {
		t.Errorf("the number instantiation was not emitted with a float64 signature:\n%s", source)
	}
	if !strings.Contains(source, "func (b *Box) Wrap_str(x value.BStr) value.BStr") {
		t.Errorf("the string instantiation was not emitted with a value.BStr signature:\n%s", source)
	}
	if strings.Contains(source, "func (b *Box) Wrap(") {
		t.Errorf("an unspecialized generic method was emitted:\n%s", source)
	}
}

// TestGenericMethodCallResolvesToSpecialization proves each method call is rewritten
// to the specialized Go name the type arguments fix, not the bare method name.
func TestGenericMethodCallResolvesToSpecialization(t *testing.T) {
	const src = "class Box {\n" +
		"  wrap<T>(x: T): T { return x; }\n" +
		"}\n" +
		"const b = new Box();\n" +
		"console.log(b.wrap(5));\n" +
		"console.log(b.wrap(\"hi\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.Wrap_num(5)") {
		t.Errorf("the number call was not rewritten to the specialization:\n%s", source)
	}
	if !strings.Contains(source, "b.Wrap_str(value.FromGoString(\"hi\"))") {
		t.Errorf("the string call was not rewritten to the specialization:\n%s", source)
	}
}

// TestGenericMethodRuns builds and runs two instantiations of a generic method so
// the monomorphized Go is proven against the JavaScript result.
func TestGenericMethodRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
class Box {
  wrap<T>(x: T): T {
    return x;
  }
}
const b = new Box();
console.log(b.wrap(5));
console.log(b.wrap("hi"));
`
	if got, want := runProgramGo(t, src), "5\nhi\n"; got != want {
		t.Fatalf("monomorphized generic method printed %q, want %q", got, want)
	}
}

// TestUncalledGenericMethodHandsBack proves a generic method no call site
// instantiates has no specialization to emit and hands back, the same zero-fail
// guard an uncalled generic function keeps.
func TestUncalledGenericMethodHandsBack(t *testing.T) {
	const src = "class Box {\n" +
		"  wrap<T>(x: T): T { return x; }\n" +
		"}\n" +
		"const b = new Box();\n" +
		"console.log(1);\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "generic") {
		t.Fatalf("uncalled generic method hand-back reason = %q, want a generic reason", reason)
	}
}

// TestGenericBodyTypeParameterResolves proves a bare type parameter resolves to
// the concrete type everywhere the specialization's body spells it, not only in the
// parameter and return: a local annotated T becomes the concrete Go type and a T[]
// built in the body becomes value.Array of that type, all under the one substitution
// the specialization lowers under.
func TestGenericBodyTypeParameterResolves(t *testing.T) {
	const src = "function box<T>(x: T): T[] {\n" +
		"  const first: T = x;\n" +
		"  const pair: T[] = [first, x];\n" +
		"  return pair;\n" +
		"}\n" +
		"console.log(box(5).length);\n" +
		"console.log(box(\"hi\").length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Box_num(x float64) *value.Array[float64]") {
		t.Errorf("the number body did not resolve T to float64 throughout:\n%s", source)
	}
	if !strings.Contains(source, "func Box_str(x value.BStr) *value.Array[value.BStr]") {
		t.Errorf("the string body did not resolve T to value.BStr throughout:\n%s", source)
	}
	if !strings.Contains(source, "value.NewArray[float64](first, x)") {
		t.Errorf("a T[] in the number body did not resolve to a float64 array:\n%s", source)
	}
}

// A generic function type used as a value or a parameter type lowers at the call
// sites that fix its type arguments: monomorphizing the enclosing generic resolves
// the callback's own type parameter to a concrete type, so a callback parameter
// typed (x: T) => T becomes func(float64) float64 in the number specialization and
// func(value.BStr) value.BStr in the string one, and a returned () => T reads the
// concrete result the call fixed.

// TestGenericFunctionTypeParameterInstantiates proves a callback parameter typed by
// the enclosing generic's type parameter lowers to a concrete Go func type per
// instantiation, and a named function passes as that value.
func TestGenericFunctionTypeParameterInstantiates(t *testing.T) {
	const src = "function apply<T>(f: (x: T) => T, v: T): T { return f(v); }\n" +
		"function inc(n: number): number { return n + 1; }\n" +
		"console.log(apply(inc, 5));\n" +
		"console.log(apply(s => s, \"hi\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Apply_num(f func(float64) float64, v float64) float64") {
		t.Errorf("the number instantiation did not lower the callback to a float64 func type:\n%s", source)
	}
	if !strings.Contains(source, "func Apply_str(f func(value.BStr) value.BStr, v value.BStr) value.BStr") {
		t.Errorf("the string instantiation did not lower the callback to a value.BStr func type:\n%s", source)
	}
	if !strings.Contains(source, "Apply_num(Inc, 5)") {
		t.Errorf("the named function was not passed as the specialized callback value:\n%s", source)
	}
}

// TestGenericFunctionTypeValueReturned proves a generic that returns a function
// value lowers the returned () => T to a concrete Go func type the call resolves.
func TestGenericFunctionTypeValueReturned(t *testing.T) {
	const src = "function twice<T>(x: T): () => T { return () => x; }\n" +
		"console.log(twice(9)());\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Twice_num(x float64) func() float64") {
		t.Errorf("the returned function value did not resolve to func() float64:\n%s", source)
	}
}

// A call fixes a generic's type argument by inference from the arguments it passes,
// not only from an explicit <T> the source spells, so the monomorphizer reads the
// concrete type off the argument the same way the checker does. An argument typed
// T[] fixes T to its element type, so first([10, 20, 30]) instantiates T=number and
// first(words) over a string[] instantiates a second specialization at T=string.

// TestGenericInfersTypeArgumentFromArrayArgument proves a type parameter reached
// only through an array argument's element type is inferred, monomorphizing one
// specialization per concrete element type with no explicit type argument written.
func TestGenericInfersTypeArgumentFromArrayArgument(t *testing.T) {
	const src = "function first<T>(xs: T[]): T { return xs[0]; }\n" +
		"console.log(first([10, 20, 30]));\n" +
		"const words: string[] = [\"a\", \"b\"];\n" +
		"console.log(first(words));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func First_num(xs *value.Array[float64]) float64") {
		t.Errorf("T was not inferred number from the array element type:\n%s", source)
	}
	if !strings.Contains(source, "func First_str(xs *value.Array[value.BStr]) value.BStr") {
		t.Errorf("T was not inferred string from the array element type:\n%s", source)
	}
	if strings.Contains(source, "func First(") {
		t.Errorf("an unspecialized generic was emitted:\n%s", source)
	}
}

// TestGenericTypeArgumentInferenceRuns builds and runs both inferred instantiations
// so the argument-inferred monomorphization is proven against the JavaScript result.
func TestGenericTypeArgumentInferenceRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function first<T>(xs: T[]): T { return xs[0]; }\n" +
		"console.log(first([10, 20, 30]));\n" +
		"const words: string[] = [\"a\", \"b\"];\n" +
		"console.log(first(words));\n"
	if got, want := runProgramGo(t, src), "10\na\n"; got != want {
		t.Fatalf("argument-inferred generics printed %q, want %q", got, want)
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

// The atom mangler names a specialization only for a type it can spell as a stable
// suffix: a number, string, boolean, bigint, or an array of one of those. A type
// argument outside that set, a self-referential object type or a bare object shape,
// has no suffix, so the monomorphizer fixes no specialization and the generic hands
// back rather than emit an unsound unspecialized func or a name it never declared.

// TestGenericOverRecursiveTypeHandsBack proves a generic instantiated over a
// self-referential object type hands back cleanly, with no crash and no emitted Go,
// since a recursive type has no stable monomorphization suffix.
func TestGenericOverRecursiveTypeHandsBack(t *testing.T) {
	const src = "interface Cell { next: Cell | null; val: number; }\n" +
		"function id<T>(x: T): T { return x; }\n" +
		"const c: Cell = { next: null, val: 1 };\n" +
		"console.log(id(c).val);\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "generic") {
		t.Fatalf("recursive-type generic hand-back reason = %q, want a generic reason", reason)
	}
}

// TestGenericOverObjectTypeHandsBack proves a generic instantiated over a bare
// object shape hands back cleanly, the same guard the recursive type keeps, since an
// object type is outside the set of types the atom mangler can name.
func TestGenericOverObjectTypeHandsBack(t *testing.T) {
	const src = "function id<T>(x: T): T { return x; }\n" +
		"console.log(id({ a: 1 }).a);\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "generic") {
		t.Fatalf("object-type generic hand-back reason = %q, want a generic reason", reason)
	}
}
