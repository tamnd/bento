package lower

import (
	"strings"
	"testing"
)

// TestStaticFunctionFieldCallLowers pins that calling a static field that holds a
// function value lowers to the package var applied to the arguments, the value twin
// of a static method's package-function call, not a hand-back.
func TestStaticFunctionFieldCallLowers(t *testing.T) {
	const src = `class C {
  x: number;
  constructor(x: number) { this.x = x; }
  val(): number { return this.x; }
  static make = function (x: number): C { return new C(x); };
}
const c = C.make(42);
console.log(String(c.val()));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "var cMake func(float64) *C") {
		t.Errorf("static function field did not lower to a package var closure:\n%s", source)
	}
	if !strings.Contains(source, "cMake(42)") {
		t.Errorf("call of the static function field did not lower to the var applied:\n%s", source)
	}
}

// TestStaticFunctionFieldCallRuns runs the emitted Go end to end: the body-declared
// static function field builds and calls an instance through the AOT path.
func TestStaticFunctionFieldCallRuns(t *testing.T) {
	const src = `class C {
  x: number;
  constructor(x: number) { this.x = x; }
  val(): number { return this.x; }
  static make = function (x: number): C { return new C(x); };
}
const c = C.make(42);
console.log(String(c.val()));
`
	got := runProgramGo(t, src)
	if got != "42\n" {
		t.Errorf("static function field call ran wrong\n got: %q\nwant: %q", got, "42\n")
	}
}

// TestStaticFunctionFieldOptionalParamHandsBack pins the boundary: a static function
// field with an optional parameter has no call-site defaulting (the field value carries
// no parameter nodes), so a plain call of it that could omit a parameter hands back
// rather than emit a call the var's Go type would reject.
func TestStaticFunctionFieldOptionalParamHandsBack(t *testing.T) {
	const src = `class C {
  x: number;
  constructor(x: number) { this.x = x; }
  static make = function (x?: number): C { return new C(x ?? 0); };
}
const c = C.make(1);
console.log(String(c.x));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "calling static .make of class C as a function is a later slice") {
		t.Errorf("hand-back reason %q does not name the static-field-call boundary", reason)
	}
}
