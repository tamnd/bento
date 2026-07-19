package lower

import (
	"strings"
	"testing"
)

// TestStaticBlockEmitsInitFunc pins that a static initialization block lowers to
// a package function named staticInitC, called from the main body rather than
// run at package-init time, since package-level Go has no ordered execution.
func TestStaticBlockEmitsInitFunc(t *testing.T) {
	const src = `class C {
  static x: number = 1;
  static { C.x = 2; }
}
console.log(String(C.x));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func staticInitC()") {
		t.Errorf("static block did not lower to func staticInitC:\n%s", source)
	}
	if !strings.Contains(source, "staticInitC()") {
		t.Errorf("main body does not call staticInitC:\n%s", source)
	}
}

// TestStaticBlockRuns runs the emitted Go and pins that two static blocks run in
// member order and their writes are observed after the class declaration.
func TestStaticBlockRuns(t *testing.T) {
	const src = `class Registry {
  static count: number = 0;
  static { Registry.count = 3; }
  static { Registry.count = Registry.count + 1; }
}
console.log(String(Registry.count));
`
	got := runProgramGo(t, src)
	if got != "4\n" {
		t.Errorf("static blocks ran wrong\n got: %q\nwant: %q", got, "4\n")
	}
}

// TestStaticBlockRunsAtDeclarationPosition pins the interleaving: a log before
// the class runs before the block, a log after runs after, because the init
// call sits at the class declaration's position in the main body.
func TestStaticBlockRunsAtDeclarationPosition(t *testing.T) {
	const src = `console.log("A");
class C {
  static x: number = 1;
  static { console.log("block:" + String(C.x)); C.x = 2; }
}
console.log("B:" + String(C.x));
`
	got := runProgramGo(t, src)
	want := "A\nblock:1\nB:2\n"
	if got != want {
		t.Errorf("static block position wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestStaticBlockHandsBack pins the boundary: a block that reads this or super
// touches the class constructor object, a dynamic-world value this slice does
// not model, so it hands back with its own reason.
func TestStaticBlockHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"readsThis",
			"class C { static x: number = 1; static { (this as any).x = 2; } }\nnew C();\n",
			"a static block that reads this is a later slice",
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

// TestBoxedPrivateStaticReadStringifies pins that stringifying a private static
// field read routes through the value model, not the number coercer. A private
// static is boxed to value.Value, so console.log(C.#x) must reach value.ToString
// rather than value.NumberToString, which would type-error on the box.
func TestBoxedPrivateStaticReadStringifies(t *testing.T) {
	skipIfShort(t)
	const src = `class C {
  static #x = 123;
  static { console.log(C.#x); }
  foo() { return C.#x; }
}
`
	source := renderProgram(t, src)
	if strings.Contains(source, "value.NumberToString") {
		t.Errorf("boxed private static read took the number coercer:\n%s", source)
	}
	got := runProgramGo(t, src)
	if got != "123\n" {
		t.Errorf("boxed private static stringify wrong\n got: %q\nwant: %q", got, "123\n")
	}
}
