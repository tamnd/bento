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
