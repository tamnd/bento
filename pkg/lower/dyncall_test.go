package lower

import (
	"strings"
	"testing"
)

// This file covers the dynamic call surface: an optional parameter of dynamic
// type lowers as a plain value.Value, an omitting call site fills the slot with
// value.Undefined, and an argument crossing the dynamic boundary boxes or
// coerces the way an assignment does. This is the shape test262's sta.js
// prelude leans on: Test262Error's constructor and its static thrower both
// take message?: any and are called with a string or with nothing.

// TestOptionalDynamicCtorParamLowers pins the constructor shape: message?: any
// becomes a value.Value parameter, a short new fills it with value.Undefined,
// and a string argument boxes on the way in.
func TestOptionalDynamicCtorParamLowers(t *testing.T) {
	const src = `class E {
  message: string;
  constructor(message?: any) {
    this.message = message || "";
  }
}
const a = new E("boom");
const b = new E();
console.log(a.message + b.message);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"func NewE(message value.Value) *E {",
		`NewE(value.StringValue(value.FromGoString("boom")))`,
		"NewE(value.Undefined)",
		`value.ToString(value.Or(message, value.StringValue(value.FromGoString(""))))`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("optional dynamic constructor parameter lowering missing %q:\n%s", want, source)
		}
	}
}

// TestOptionalDynamicStaticMethodPads pins the static method shape: a short
// call to a static whose trailing parameter is a dynamic optional passes
// value.Undefined in the omitted slot.
func TestOptionalDynamicStaticMethodPads(t *testing.T) {
	const src = `class E {
  message: string;
  constructor(message?: any) {
    this.message = message || "";
  }
  static raise(message?: any): never {
    throw new E(message);
  }
}
try {
  E.raise();
} catch (err: any) {
  console.log(err.message);
}
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "ERaise(value.Undefined)") {
		t.Errorf("short static call did not pad the dynamic optional with value.Undefined:\n%s", source)
	}
	if !strings.Contains(source, "func ERaise(message value.Value) {") {
		t.Errorf("static with a dynamic optional did not lower to a value.Value parameter:\n%s", source)
	}
}

// TestOptionalDynamicFunctionParamPads pins the top-level function shape: a
// bare x?: any needs no written default because the omitted slot fills with
// value.Undefined, the absent value the language binds.
func TestOptionalDynamicFunctionParamPads(t *testing.T) {
	const src = `function tag(x?: any): string {
  return "got " + x;
}
console.log(tag("a"));
console.log(tag());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "Tag(value.Undefined)") {
		t.Errorf("short call did not pad the dynamic optional with value.Undefined:\n%s", source)
	}
}

// TestEvolvedAnyCalleeRoutesThroughRuntimeCall pins the evolving-any shape: a var
// declared with no initializer, later assigned a function, stores a value.Value box.
// Control-flow analysis narrows the callee to a concrete function type at the call
// site, but the slot stays a box, so the call must dispatch through the runtime Call
// rather than a static Go call the box does not support. The call result is a box
// too, so the enclosing String coercion routes through value.ToString, not the
// number path the evolved return type would pick.
func TestEvolvedAnyCalleeRoutesThroughRuntimeCall(t *testing.T) {
	const src = `var f;
f = function () { return 1; };
console.log(String(f()));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "f.Call()") {
		t.Errorf("evolved-any callee did not dispatch through the runtime Call:\n%s", source)
	}
	if !strings.Contains(source, "value.ToString(f.Call())") {
		t.Errorf("call result was not treated as a box by the enclosing coercion:\n%s", source)
	}
	if strings.Contains(source, "value.NumberToString(f.Call())") {
		t.Errorf("call result took the static number coercion over the box:\n%s", source)
	}
}

// TestEvolvedAnyCalleeRuns builds and runs the evolving-any call end to end,
// including the block-scope var-sharing shape test262 exercises in scope-var-none:
// two closures declared around a bare block both capture the one function-scoped x,
// and each is stored in an implicit-any var and called through the runtime Call.
func TestEvolvedAnyCalleeRuns(t *testing.T) {
	skipIfShort(t)
	const src = `var x = "outside";
var probeBefore = function () { return x; };
var probeInside;
{
  var x = "inside";
  probeInside = function () { return x; };
}
console.log(String(probeBefore()));
console.log(String(probeInside()));
console.log(x);
`
	got := runProgramGo(t, src)
	want := "inside\ninside\ninside\n"
	if got != want {
		t.Fatalf("evolved-any call program printed %q, want %q", got, want)
	}
}

// TestOmittedStaticCtorParamFillsNone runs an omitted bare optional constructor
// parameter: ctorParamFields renders its value.Opt[float64] field and the new-E call
// site fills value.None, so the short call lowers and the body runs even though it
// never reads the parameter.
func TestOmittedStaticCtorParamFillsNone(t *testing.T) {
	skipIfShort(t)
	const src = `class E {
  n: number;
  constructor(n?: number) {
    this.n = 1;
  }
}
const e = new E();
console.log(e.n);
`
	if got, want := runProgramGo(t, src), "1\n"; got != want {
		t.Fatalf("omitted static constructor parameter printed %q, want %q", got, want)
	}
}
