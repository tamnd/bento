package lower

import (
	"strings"
	"testing"
)

// A generic function can fix its type parameter through the rest parameter alone,
// as in firstOf<T>(...xs: T[]): T, where T appears only in the trailing array and
// the return. bento monomorphizes it the same way it does a fixed parameter: one Go
// function per concrete element type a call site fixes, with the rest lowered to a
// *value.Array[T] field, and the specialization body reads a bare T as the concrete
// type, so a print or a String() of an element lowers rather than handing back.

// TestGenericRestMonomorphizesPerTypeArgument proves a generic whose type parameter
// is bound only through its rest parameter emits one specialization per concrete
// element type, each with the concrete *value.Array field and return.
func TestGenericRestMonomorphizesPerTypeArgument(t *testing.T) {
	const src = "function firstOf<T>(...xs: T[]): T { return xs[0]; }\n" +
		"const a: string = \"a\";\nconst b: string = \"b\";\n" +
		"console.log(firstOf(10, 20, 30));\n" +
		"console.log(firstOf(a, b));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func FirstOf_num(xs *value.Array[float64]) float64") {
		t.Errorf("the number instantiation was not emitted with a *value.Array[float64] rest:\n%s", source)
	}
	if !strings.Contains(source, "func FirstOf_str(xs *value.Array[value.BStr]) value.BStr") {
		t.Errorf("the string instantiation was not emitted with a *value.Array[value.BStr] rest:\n%s", source)
	}
	if strings.Contains(source, "func FirstOf(") {
		t.Errorf("an unspecialized generic function was emitted:\n%s", source)
	}
}

// TestGenericRestCallGathers proves each call to a generic rest is rewritten to its
// specialization name and gathers its trailing arguments into the concrete array.
func TestGenericRestCallGathers(t *testing.T) {
	const src = "function firstOf<T>(...xs: T[]): T { return xs[0]; }\n" +
		"console.log(firstOf(10, 20, 30));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "FirstOf_num(value.NewArray[float64](10, 20, 30))") {
		t.Errorf("the call did not gather its arguments into the specialization's rest array:\n%s", source)
	}
}

// TestGenericRestStringLiteralUnionSpecializes proves a call whose string-literal
// arguments the checker infers as a union ("a" | "b") mangles to the same str atom a
// plain string does, since the union lowers to value.BStr; the numeric-literal union
// already folds to num, so this is the symmetric string case.
func TestGenericRestStringLiteralUnionSpecializes(t *testing.T) {
	const src = "function firstOf<T>(...xs: T[]): T { return xs[0]; }\n" +
		"console.log(firstOf(\"a\", \"b\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func FirstOf_str(xs *value.Array[value.BStr]) value.BStr") {
		t.Errorf("a string-literal-union rest did not specialize to value.BStr:\n%s", source)
	}
	if !strings.Contains(source, "FirstOf_str(value.NewArray[value.BStr](value.FromGoString(\"a\"), value.FromGoString(\"b\")))") {
		t.Errorf("the string-literal call did not gather its arguments:\n%s", source)
	}
}

// TestGenericRestBodyStringifiesTypeParameter proves the specialization body reads a
// bare type parameter as the concrete type: a print of an element in the number
// instantiation coerces through value.NumberToString, while the string instantiation
// prints the value.BStr directly, the type-parameter resolution the coercion
// predicates gained for a monomorphized body.
func TestGenericRestBodyStringifiesTypeParameter(t *testing.T) {
	const src = "function logAll<T>(...xs: T[]): void {\n" +
		"  for (const x of xs) { console.log(x); }\n" +
		"}\n" +
		"const s: string = \"x\";\n" +
		"logAll(1, 2);\n" +
		"logAll(s);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ConsoleLog(value.NumberToString(x))") {
		t.Errorf("the number instantiation did not stringify its element through NumberToString:\n%s", source)
	}
	if !strings.Contains(source, "value.ConsoleLog(x)") {
		t.Errorf("the string instantiation did not print its element directly:\n%s", source)
	}
}

// TestGenericRestRuns builds and runs a generic rest so the monomorphized Go is
// proven against the JavaScript result, across the number and string instantiations
// and the empty-rest case.
func TestGenericRestRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function firstOf<T>(fallback: T, ...xs: T[]): T {
  if (xs.length === 0) return fallback;
  return xs[0];
}
function logAll<T>(...xs: T[]): void {
  for (const x of xs) console.log(x);
}
console.log(firstOf(0, 10, 20, 30));
console.log(firstOf(99));
const a: string = "a";
const b: string = "b";
console.log(firstOf(a, b));
logAll(1, 2, 3);
`
	if got, want := runProgramGo(t, src), "10\n99\nb\n1\n2\n3\n"; got != want {
		t.Fatalf("generic rest printed %q, want %q", got, want)
	}
}
