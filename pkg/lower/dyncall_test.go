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

// TestOptionalStaticParamStillHandsBack pins the boundary: an omitted optional
// of a static type has no Go value to stand in for the omission, so the short
// call hands back the way it always has.
func TestOptionalStaticParamStillHandsBack(t *testing.T) {
	const src = `class E {
  n: number;
  constructor(n?: number) {
    this.n = 1;
  }
}
const e = new E();
console.log(e.n);
`
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatal("omitting a static optional lowered, want a hand-back until value.Opt synthesis lands")
	}
}
