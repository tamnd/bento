package lower

import (
	"strings"
	"testing"
)

// TestNullishAssignDynamicEmits pins that ??= on a dynamic target guards the store
// with the runtime IsNullish, the null-or-undefined presence test, rather than the
// Opt-only IsUndefined the optional target uses.
func TestNullishAssignDynamicEmits(t *testing.T) {
	const src = "function f(v: any): any { v ??= 42; return v; }\nconsole.log(f(null));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "v.IsNullish()") {
		t.Errorf("dynamic ??= did not guard on IsNullish:\n%s", source)
	}
}

// TestNullishAssignLocalDefiniteEmits pins that ??= with a definite right-hand side
// into an optional local keeps the slot Opt[T] (the store wraps the value in Some)
// and a later narrowed read unwraps with .Get(), so the two agree.
func TestNullishAssignLocalDefiniteEmits(t *testing.T) {
	const src = "let v: number | undefined = undefined;\nv ??= 42;\nconsole.log(v + 1);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Some[float64](42)") {
		t.Errorf("definite ??= did not wrap the store in Some:\n%s", source)
	}
	if !strings.Contains(source, "v.Get() + 1") {
		t.Errorf("narrowed read after ??= did not unwrap with Get:\n%s", source)
	}
}

// TestNullishAssignDynamicAndDefiniteRuns builds and runs ??= over a dynamic target
// and an optional local and matches Node: a null or undefined dynamic takes the
// fallback while a present value stays, and a definite fallback into an optional local
// leaves it a plain number for the arithmetic that follows.
func TestNullishAssignDynamicAndDefiniteRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function dyn(v: any): any {
  v ??= 42;
  return v;
}
console.log(dyn(undefined));
console.log(dyn(null));
console.log(dyn(7));
console.log(dyn("keep"));
let a: number | undefined = undefined;
a ??= 5;
console.log(a + 1);
let b: number | undefined = 10;
b ??= 99;
console.log(b + 1);
`
	got := runProgramGo(t, src)
	want := "42\n42\n7\nkeep\n6\n11\n"
	if got != want {
		t.Fatalf("nullish assignment program printed %q, want %q", got, want)
	}
}

// TestNullishAssignNullableUnionEmits pins that ??= on a nullable tagged-sum target
// guards the store with a tag compare against the sentinel arm: a T | null union
// triggers on the null tag alone, a T | null | undefined union on the null tag ored
// with the undefined tag, and the store wraps the value back into the union.
func TestNullishAssignNullableUnionEmits(t *testing.T) {
	const src = "let a: number | null = null;\na ??= 5;\nconsole.log(a);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if a.tag == NumOrNullNull {") {
		t.Errorf("T | null ??= did not guard on the null tag:\n%s", source)
	}
	if !strings.Contains(source, "a = NumOrNullOfNum(5)") {
		t.Errorf("T | null ??= store did not wrap into the union:\n%s", source)
	}

	const src2 = "let c: string | null | undefined = null;\nc ??= \"x\";\nconsole.log(c);\n"
	source2 := renderProgram(t, src2)
	if !strings.Contains(source2, "if c.tag == StrOrUndefOrNullNull || c.tag == StrOrUndefOrNullUndef {") {
		t.Errorf("T | null | undefined ??= did not guard on both sentinel tags:\n%s", source2)
	}
}

// TestNullishAssignNullableUnionRuns builds and runs ??= over a T | null and a
// T | null | undefined local and matches Node: a null or undefined target takes the
// fallback, a present value stays, and the fallback is evaluated only on the trigger.
func TestNullishAssignNullableUnionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `let a: number | null = null;
a ??= 5;
console.log(a);
let b: number | null = 7;
b ??= 99;
console.log(b);
let c: string | null | undefined = null;
c ??= "x";
console.log(c);
let d: string | null | undefined = undefined;
d ??= "y";
console.log(d);
let e: string | null | undefined = "keep";
e ??= "z";
console.log(e);
let hits = 0;
function bump(): number { hits++; return 1; }
let f: number | null = 3;
f ??= bump();
console.log(f, hits);
`
	got := runProgramGo(t, src)
	want := "5\n7\nx\ny\nkeep\n3 0\n"
	if got != want {
		t.Fatalf("nullable-union nullish assignment program printed %q, want %q", got, want)
	}
}
