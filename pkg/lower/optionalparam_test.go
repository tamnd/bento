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
