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

// A method, an async function, and a generator all lower their parameters through
// the shared funcParamFields, but their bodies never build the optParams narrowing
// set: only a body lowered through funcDeclNamed does. So a bare optional parameter
// there must hand back rather than emit a value.Opt[T] field the body reads as a bare
// T, which would not compile. These pin that zero-fail guard, one per shared caller.

// TestMethodOptionalParamHandsBack pins the method path: a class method with a bare
// optional parameter hands back, since no optParams set is built for a method body.
func TestMethodOptionalParamHandsBack(t *testing.T) {
	const src = `class C {
  add(x: number, y?: number): number {
    if (y !== undefined) { return x + y; }
    return x;
  }
}
console.log(new C().add(1, 2));
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
