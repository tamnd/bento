package lower

import (
	"strings"
	"testing"
)

// TestNamedCallableFuncDeclLowers pins that a named function declaration that
// later carries own data properties (foo.x = 1) lowers to the callable-object
// shape: a struct-typed package var, not a bare `func Foo`. Its checker type is a
// callable object, so the model interns a `type Foo struct { Call func(); X ... }`
// and constructs it at the top of main; emitting a `func Foo` too would put two
// Foo declarations in one block, which does not compile (Object/keys 15.2.3.14-3-2
// hit exactly this). The var and its construction replace the old handback.
func TestNamedCallableFuncDeclLowers(t *testing.T) {
	const src = "function foo() {}\nfoo.x = 1;\nconsole.log(String(foo.x));\n"
	out := renderProgram(t, src)
	if strings.Contains(out, "func Foo(") {
		t.Fatalf("named callable object emitted a colliding bare func:\n%s", out)
	}
	if !strings.Contains(out, "var foo *Foo") {
		t.Fatalf("named callable object was not declared at package scope:\n%s", out)
	}
	if !strings.Contains(out, "foo = &Foo{}") {
		t.Fatalf("named callable object was not constructed into its package var:\n%s", out)
	}
	if strings.Contains(out, "foo := &Foo{}") {
		t.Fatalf("named callable object kept a short declaration that shadows the package var:\n%s", out)
	}
}

// TestNamedCallableFuncDeclRuns builds and runs the named callable object end to
// end: it writes an own property and reads it back, proving the package var holds
// the object and the property field round-trips. Node prints 1.
func TestNamedCallableFuncDeclRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function foo() {}\nfoo.x = 1;\nconsole.log(String(foo.x));\n"
	got := runProgramGo(t, src)
	want := "1\n"
	if got != want {
		t.Fatalf("named callable object run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestModuleHoistedCallableInitsInPlace pins that a callable-object binding a
// top-level function reads is hoisted to a package-level var and its own site
// assigns into that var, rather than a short declaration that shadows it. The
// test262 prelude is exactly this shape: `const assert = function () {}` that a
// later function body reaches for, so a `assert := &Assert{}` in main would leave
// the package-level assert nil and every assert.sameValue call would nil-deref.
func TestModuleHoistedCallableInitsInPlace(t *testing.T) {
	const src = `interface Assert {
  (x: any): void;
  same(a: any, b: any): void;
}
const assert = function (x: any): void {
  if (!x) { throw new Error("f"); }
} as Assert;
assert.same = function (a: any, b: any): void {
  if (a !== b) { throw new Error("ne"); }
};
function check(): void {
  assert.same(2, 2);
}
check();
console.log("ran");
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var assert *Assert") {
		t.Fatalf("module-hoisted callable was not declared at package scope:\n%s", out)
	}
	if !strings.Contains(out, "assert = &Assert{}") {
		t.Fatalf("module-hoisted callable did not assign into its package var:\n%s", out)
	}
	if strings.Contains(out, "assert := &Assert{}") {
		t.Fatalf("module-hoisted callable kept a short declaration that shadows the package var:\n%s", out)
	}
}

// TestModuleHoistedCallableRuns builds and runs the module-hoisted callable end
// to end: a top-level function calls a method on the callable the prelude bound,
// so the run proves the package-level var holds the object, not nil.
func TestModuleHoistedCallableRuns(t *testing.T) {
	skipIfShort(t)
	const src = `interface Assert {
  (x: any): void;
  same(a: any, b: any): void;
}
const assert = function (x: any): void {
  if (!x) { throw new Error("f"); }
} as Assert;
assert.same = function (a: any, b: any): void {
  if (a !== b) { throw new Error("ne"); }
};
function check(): void {
  assert.same(2, 2);
}
check();
console.log("ran");
`
	got := runProgramGo(t, src)
	want := "ran\n"
	if got != want {
		t.Fatalf("module-hoisted callable run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestPlainFunctionStillLowers pins that a function with no own properties keeps
// lowering to a bare Go func, so the callable-object handback does not swallow an
// ordinary declaration. A plain function type carries no data properties, so it
// is not a callable object and stays on the func path.
func TestPlainFunctionStillLowers(t *testing.T) {
	const src = "function twice(n: number): number { return n * 2; }\nconsole.log(String(twice(3)));\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func Twice(") {
		t.Fatalf("plain function did not lower to a bare func:\n%s", out)
	}
}
