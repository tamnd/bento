package lower

import (
	"strings"
	"testing"
)

// A bare optional parameter (x?: T, no default) lowers to a value.Opt[T] field. A
// supplied argument wraps in value.Some, an omitted one fills value.None, and a read
// the checker narrowed past a presence guard unwraps with .Get(). These tests prove
// the machinery end to end and pin the emitted shape.

// TestOptionalParamNarrowsPositive runs the x !== undefined guard: the narrowed read
// unwraps the option, and the omitted call binds the empty option the else branch
// returns.
func TestOptionalParamNarrowsPositive(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(x?: number): number {
  if (x !== undefined) { return x + 1; }
  return 0;
}
console.log(f(5));
console.log(f());
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("optional parameter printed %q, want %q", got, want)
	}
}

// TestOptionalParamNarrowsEarlyReturn runs the mirror guard x === undefined, whose
// early return leaves the parameter narrowed to T for the read that follows.
func TestOptionalParamNarrowsEarlyReturn(t *testing.T) {
	skipIfShort(t)
	const src = `
function g(s?: string): string {
  if (s === undefined) { return "none"; }
  return s + "!";
}
console.log(g("hi"));
console.log(g());
`
	if got, want := runProgramGo(t, src), "hi!\nnone\n"; got != want {
		t.Fatalf("optional string parameter printed %q, want %q", got, want)
	}
}

// TestOptionalParamNullishDefault runs the option through the ?? operator, which
// reads the present value or the fallback with no explicit guard.
func TestOptionalParamNullishDefault(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(x?: number): number {
  return (x ?? 10) + 1;
}
console.log(f(5));
console.log(f());
`
	if got, want := runProgramGo(t, src), "6\n11\n"; got != want {
		t.Fatalf("optional parameter through ?? printed %q, want %q", got, want)
	}
}

// TestOptionalParamPassThrough proves a bare option threads between two optional
// parameters without a double wrap: the option passes straight into the next
// parameter, boxToOptional leaving an already-optional source alone.
func TestOptionalParamPassThrough(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(x?: number): number | undefined { return x; }
function g(x?: number): number {
  const y = f(x);
  if (y !== undefined) { return y; }
  return -1;
}
console.log(g(5));
console.log(g());
`
	if got, want := runProgramGo(t, src), "5\n-1\n"; got != want {
		t.Fatalf("optional pass-through printed %q, want %q", got, want)
	}
}

// TestOptionalParamEmitsOptField pins the emitted shape: the parameter is a
// value.Opt[float64] field, the narrowed read unwraps with .Get(), a supplied
// argument wraps in Some, and an omitted one fills None.
func TestOptionalParamEmitsOptField(t *testing.T) {
	const src = `
function f(x?: number): number {
  if (x !== undefined) { return x + 1; }
  return 0;
}
console.log(f(5));
console.log(f());
`
	out := renderProgram(t, src)
	for _, want := range []string{
		"x value.Opt[float64]",
		"x.Get()",
		"value.Some[float64](",
		"value.None[float64]()",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("emitted Go missing %q:\n%s", want, out)
		}
	}
}

// A required parameter annotated x: T | undefined binds a value.Opt[T] field too,
// since typeExpr renders the two-member optional union to Opt[T]. The caller always
// supplies it, as Some for a present value or None for an explicit undefined, and a
// read the checker narrowed to T unwraps with .Get(). These pin that a narrowed read
// of a required optional-union parameter lowers, not just the bare x?: T form.

// TestRequiredUnionParamNarrows runs the x !== undefined guard on a required
// x: T | undefined parameter: the narrowed read unwraps, and the explicit-undefined
// call binds the empty option the else branch returns.
func TestRequiredUnionParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(x: number | undefined): number {
  if (x !== undefined) { return x + 1; }
  return 0;
}
console.log(f(5));
console.log(f(undefined));
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("required union parameter printed %q, want %q", got, want)
	}
}

// TestRequiredUnionParamEarlyReturn runs the mirror guard on a required optional-union
// parameter, whose early return leaves the parameter narrowed to T for the read after.
func TestRequiredUnionParamEarlyReturn(t *testing.T) {
	skipIfShort(t)
	const src = `
function g(s: string | undefined): string {
  if (s === undefined) { return "none"; }
  return s + "!";
}
console.log(g("hi"));
console.log(g(undefined));
`
	if got, want := runProgramGo(t, src), "hi!\nnone\n"; got != want {
		t.Fatalf("required union string parameter printed %q, want %q", got, want)
	}
}

// TestRequiredUnionParamEmitsGet pins the shape: the read narrowed past the guard
// unwraps the field with .Get(), the fix for the Opt[T] field the union already
// rendered before this slice taught the narrowing pass to track it.
func TestRequiredUnionParamEmitsGet(t *testing.T) {
	const src = `
function f(x: number | undefined): number {
  if (x !== undefined) { return x + 1; }
  return 0;
}
console.log(f(5));
`
	out := renderProgram(t, src)
	for _, want := range []string{
		"x value.Opt[float64]",
		"x.Get() + 1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("emitted Go missing %q:\n%s", want, out)
		}
	}
}

// An async function declaration and a generator declaration lower their parameters
// through the shared funcParamFields but fill no value.None at their call sites, so a
// bare optional parameter there must hand back rather than emit a value.Opt[T] field
// the body reads as a bare T, which would not compile. These pin that zero-fail guard,
// one per still-open caller. A method is no longer among them: it pushes the full
// optParamsOf before its fields build and every method call site fills value.None, so a
// method's bare optional lowers, covered by TestMethodBareOptionalParamNarrows below.

// TestMethodBareOptionalParamNarrows runs an instance method with a bare x?: T
// parameter read past a presence guard, proving the method declaration binds the
// value.Opt[T] field, the narrowed read unwraps with .Get(), and an omitting call site
// fills value.None.
func TestMethodBareOptionalParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `class C {
  add(x: number, y?: number): number {
    if (y !== undefined) { return x + y; }
    return x;
  }
}
const c = new C();
console.log(c.add(1, 2));
console.log(c.add(1));
`
	got := runProgramGo(t, src)
	if want := "3\n1\n"; got != want {
		t.Fatalf("method bare optional printed %q, want %q", got, want)
	}
}

// TestStaticMethodBareOptionalParamNarrows runs the same guard on a static method,
// whose declaration and call both take the package-function path, to prove the static
// call site fills value.None too.
func TestStaticMethodBareOptionalParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `class C {
  static tag(s?: string): string {
    if (s !== undefined) { return "got:" + s; }
    return "none";
  }
}
console.log(C.tag("hi"));
console.log(C.tag());
`
	got := runProgramGo(t, src)
	if want := "got:hi\nnone\n"; got != want {
		t.Fatalf("static method bare optional printed %q, want %q", got, want)
	}
}

// TestMethodBooleanOptionalParamHandsBack pins that a bare boolean optional method
// parameter still hands back: TypeScript models boolean as true | false, so
// boolean | undefined is a three-member union optionalInner does not fold to a
// value.Opt[T], leaving the declaration on the call-site-defaulting handback.
func TestMethodBooleanOptionalParamHandsBack(t *testing.T) {
	const src = `class C {
  g(b?: boolean): number {
    if (b) { return 1; }
    return 0;
  }
}
console.log(new C().g(true));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional parameter needs call-site defaulting") {
		t.Errorf("hand-back reason %q does not name the optional-parameter case", reason)
	}
}

// TestAsyncOptionalParamHandsBack pins the async path: an async function with a bare
// optional parameter hands back, since async.go reaches funcParamFields without the
// optParams set.
func TestAsyncOptionalParamHandsBack(t *testing.T) {
	const src = `async function f(x: number, y?: number): Promise<number> {
  if (y !== undefined) { return x + y; }
  return x;
}
f(5).then(v => console.log(v));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional parameter needs call-site defaulting") {
		t.Errorf("hand-back reason %q does not name the optional-parameter case", reason)
	}
}

// TestGeneratorOptionalParamHandsBack pins the generator path: a generator with a
// bare optional parameter hands back, since generator.go reaches funcParamFields
// without the optParams set.
func TestGeneratorOptionalParamHandsBack(t *testing.T) {
	const src = `function* g(x: number, y?: number): Generator<number> {
  if (y !== undefined) { yield x + y; }
  yield x;
}
for (const v of g(5)) { console.log(v); }
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional parameter needs call-site defaulting") {
		t.Errorf("hand-back reason %q does not name the optional-parameter case", reason)
	}
}

// A required parameter annotated x: T | undefined binds a value.Opt[T] field in a
// method, async, or generator body the same way a top-level function does, since
// typeExpr renders the union that way before the funcParamFields switch is reached.
// Those forms reach funcParamFields without funcDeclNamed's narrowing set, so each
// pushes a required-only optParams set around its body, and a read the checker
// narrowed to T unwraps with .Get(). These run the narrowed guard end to end, one
// per body form, proving the previously broken Go now compiles and runs.

// TestMethodRequiredUnionParamNarrows runs an instance method whose required
// optional-union parameter is read past a presence guard.
func TestMethodRequiredUnionParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
class C {
  f(x: number | undefined): number {
    if (x !== undefined) { return x + 1; }
    return 0;
  }
}
const c = new C();
console.log(c.f(5));
console.log(c.f(undefined));
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("method required-union parameter printed %q, want %q", got, want)
	}
}

// TestStaticMethodRequiredUnionParamNarrows runs the same guard in a static method,
// the second funcParamFields caller in classes.go.
func TestStaticMethodRequiredUnionParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
class C {
  static f(x: number | undefined): number {
    if (x !== undefined) { return x + 1; }
    return 0;
  }
}
console.log(C.f(5));
console.log(C.f(undefined));
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("static method required-union parameter printed %q, want %q", got, want)
	}
}

// TestAsyncRequiredUnionParamNarrows runs the guard in an async function, whose
// captured parameter reads through the value.Async closure.
func TestAsyncRequiredUnionParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
async function f(x: number | undefined): Promise<number> {
  if (x !== undefined) { return x + 1; }
  return 0;
}
f(5).then(v => console.log(v));
f(undefined).then(v => console.log(v));
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("async required-union parameter printed %q, want %q", got, want)
	}
}

// TestGeneratorRequiredUnionParamNarrows runs the guard in a generator, whose
// captured parameter reads through the coroutine closure.
func TestGeneratorRequiredUnionParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
function* g(x: number | undefined): Generator<number> {
  if (x !== undefined) { yield x + 1; }
  yield 0;
}
for (const v of g(5)) { console.log(v); }
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("generator required-union parameter printed %q, want %q", got, want)
	}
}

// A closure, a function expression or an arrow, lowers its parameters through
// closureParamFields, a path apart from funcParamFields, and its body reaches none of
// funcDeclNamed's narrowing set. But a closure's call sites already fill value.None for
// an omitted argument, so a closure tracks the full optParamsOf: both a bare x?: T and a
// required x: T | undefined parameter binds a value.Opt[T] field, and a read the checker
// narrowed to T unwraps with .Get(). These run each closure form end to end, proving the
// previously broken Go now compiles and runs. The bare form is the stronger case: the
// top-level path hands it back, a closure runs it, because only a closure fills None.

// TestFuncExprRequiredUnionParamNarrows runs a function expression whose required
// optional-union parameter is read past a presence guard.
func TestFuncExprRequiredUnionParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
const f = function (x: number | undefined): number {
  if (x !== undefined) { return x + 1; }
  return 0;
};
console.log(f(5));
console.log(f(undefined));
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("function expression required-union parameter printed %q, want %q", got, want)
	}
}

// TestFuncExprBareOptionalParamNarrows runs a function expression with a bare x?: T
// parameter: its call sites fill None, so the omitting call binds the empty option and
// the narrowed read unwraps, the case the top-level path hands back.
func TestFuncExprBareOptionalParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
const f = function (x: number, y?: number): number {
  if (y !== undefined) { return x + y; }
  return x;
};
console.log(f(1, 2));
console.log(f(1));
`
	if got, want := runProgramGo(t, src), "3\n1\n"; got != want {
		t.Fatalf("function expression bare optional parameter printed %q, want %q", got, want)
	}
}

// TestNamedFuncExprBareOptionalParamNarrows runs the same bare-optional guard in a
// named function expression, whose body lowers through the self-reference two-step.
func TestNamedFuncExprBareOptionalParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
const f = function add(x: number, y?: number): number {
  if (y !== undefined) { return x + y; }
  return x;
};
console.log(f(1, 2));
console.log(f(1));
`
	if got, want := runProgramGo(t, src), "3\n1\n"; got != want {
		t.Fatalf("named function expression bare optional parameter printed %q, want %q", got, want)
	}
}

// TestBlockArrowRequiredUnionParamNarrows runs a block-body arrow whose required
// optional-union parameter is read past a presence guard.
func TestBlockArrowRequiredUnionParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
const f = (x: number | undefined): number => {
  if (x !== undefined) { return x + 1; }
  return 0;
};
console.log(f(5));
console.log(f(undefined));
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("block arrow required-union parameter printed %q, want %q", got, want)
	}
}

// TestConciseArrowBareOptionalParamNarrows runs a concise-body arrow whose bare
// optional parameter is read in the ternary's narrowed branch.
func TestConciseArrowBareOptionalParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
const f = (x: number, y?: number): number => (y !== undefined ? x + y : x);
console.log(f(1, 2));
console.log(f(1));
`
	if got, want := runProgramGo(t, src), "3\n1\n"; got != want {
		t.Fatalf("concise arrow bare optional parameter printed %q, want %q", got, want)
	}
}

// TestAsyncArrowRequiredUnionParamNarrows runs the guard in an async arrow, whose
// captured parameter reads through the value.Async closure.
func TestAsyncArrowRequiredUnionParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
const f = async (x: number | undefined): Promise<number> => {
  if (x !== undefined) { return x + 1; }
  return 0;
};
f(5).then(v => console.log(v));
f(undefined).then(v => console.log(v));
`
	if got, want := runProgramGo(t, src), "6\n0\n"; got != want {
		t.Fatalf("async arrow required-union parameter printed %q, want %q", got, want)
	}
}

// TestGeneratorExprBareOptionalParamNarrows runs the guard in a generator function
// expression, whose captured parameter reads through the coroutine closure.
func TestGeneratorExprBareOptionalParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
const g = function* (x: number, y?: number): Generator<number> {
  if (y !== undefined) { yield x + y; }
  yield x;
};
for (const v of g(1, 2)) { console.log(v); }
`
	if got, want := runProgramGo(t, src), "3\n1\n"; got != want {
		t.Fatalf("generator expression bare optional parameter printed %q, want %q", got, want)
	}
}

// TestClosureNestedOptionalParamNarrows nests an arrow with its own required
// optional-union parameter inside a top-level function that has one too, proving each
// body unwraps its own parameter and the outer set is restored after the inner arrow
// lowers.
func TestClosureNestedOptionalParamNarrows(t *testing.T) {
	skipIfShort(t)
	const src = `
function outer(x: number | undefined): number {
  const inner = (y: number | undefined): number => {
    if (y !== undefined) { return y + 1; }
    return 0;
  };
  if (x !== undefined) { return inner(x) + 100; }
  return inner(undefined);
}
console.log(outer(5));
console.log(outer(undefined));
`
	if got, want := runProgramGo(t, src), "106\n0\n"; got != want {
		t.Fatalf("nested closure optional parameters printed %q, want %q", got, want)
	}
}

// TestClosureOptionalParamPassThrough proves a closure threads a bare option to another
// closure without a double wrap: the option passes straight in, and only the guarded
// read at the end unwraps. Pins that tracking a closure's optional parameter does not
// unwrap a pass-through use whose type stays the optional union.
func TestClosureOptionalParamPassThrough(t *testing.T) {
	skipIfShort(t)
	const src = `
const id = function (x: number | undefined): number | undefined { return x; };
const g = (x: number | undefined): number => {
  const y = id(x);
  if (y !== undefined) { return y; }
  return -1;
};
console.log(g(5));
console.log(g(undefined));
`
	if got, want := runProgramGo(t, src), "5\n-1\n"; got != want {
		t.Fatalf("closure optional pass-through printed %q, want %q", got, want)
	}
}
