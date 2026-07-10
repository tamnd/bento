package lower

import (
	"strings"
	"testing"
)

// TestStaticGetterEmitsPackageFunc pins that a static get accessor lowers to a
// package function named CX, the class name prefixed spelling a static method
// takes, not a receiver method.
func TestStaticGetterEmitsPackageFunc(t *testing.T) {
	const src = `class C {
  static _x: number = 1;
  static get x(): number { return C._x; }
}
console.log(String(C.x));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func CX() float64") {
		t.Errorf("static getter did not lower to func CX():\n%s", source)
	}
	if !strings.Contains(source, "value.NumberToString(CX())") {
		t.Errorf("static getter read did not route to the CX() call:\n%s", source)
	}
}

// TestStaticSetterEmitsPackageFunc pins that a static set accessor lowers to a
// package function named CSetX, the Set-prefixed sibling, and a write through
// the class name routes to the call.
func TestStaticSetterEmitsPackageFunc(t *testing.T) {
	const src = `class C {
  static _x: number = 1;
  static set x(v: number) { C._x = v; }
}
C.x = 9;
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func CSetX(v float64)") {
		t.Errorf("static setter did not lower to func CSetX(v float64):\n%s", source)
	}
	if !strings.Contains(source, "CSetX(9)") {
		t.Errorf("static setter write did not route to the CSetX call:\n%s", source)
	}
}

// TestStaticAccessorPairRuns runs the emitted Go and pins that a static
// getter/setter pair, backing a static field through the class name, reads and
// writes the field the source spells.
func TestStaticAccessorPairRuns(t *testing.T) {
	const src = `class Box {
  static _v: number = 10;
  static get v(): number { return Box._v; }
  static set v(n: number) { Box._v = n; }
}
Box.v = Box.v + 5;
console.log(String(Box.v));
`
	got := runProgramGo(t, src)
	if got != "15\n" {
		t.Errorf("static accessor pair ran wrong\n got: %q\nwant: %q", got, "15\n")
	}
}

// TestStaticSetterCoercesInt pins that a write through a static setter coerces
// an integer literal to the float64 the parameter declares, the same coercion a
// static-field store takes.
func TestStaticSetterCoercesInt(t *testing.T) {
	const src = `class Counter {
  static _n: number = 0;
  static get n(): number { return Counter._n; }
  static set n(x: number) { Counter._n = x; }
}
Counter.n = 3;
console.log(String(Counter.n * 2));
`
	got := runProgramGo(t, src)
	if got != "6\n" {
		t.Errorf("static setter coercion ran wrong\n got: %q\nwant: %q", got, "6\n")
	}
}

// TestStaticAccessorHandsBack pins the boundary: a static getter colliding with
// a static method the module already speaks, a read of a write-only static
// accessor, and a compound store through one each hand back with their own
// honest reason.
func TestStaticAccessorHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"getterClashesStaticMethod",
			"class C { static X(): number { return 1; } static _x: number = 2; static get x(): number { return C._x; } }\nnew C();\n",
			"the module already speaks CX, the name static get .x needs",
		},
		{
			"readWriteOnly",
			"class C { static _x: number = 0; static set x(v: number) { C._x = v; } }\nconsole.log(String(C.x));\n",
			"reading the write-only static accessor .x of class C is a later slice",
		},
		{
			"compoundThroughSetter",
			"class C { static _x: number = 0; static get x(): number { return C._x; } static set x(v: number) { C._x = v; } }\nC.x += 1;\n",
			"a compound store or increment through the static .x accessor of class C is a later slice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := renderProgramHandBack(t, tc.src)
			if !strings.Contains(reason, tc.want) {
				t.Errorf("hand-back reason %q does not name %q", reason, tc.want)
			}
		})
	}
}
