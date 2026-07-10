package lower

import (
	"strings"
	"testing"
)

// TestStaticFieldComputedRuns runs the emitted Go and pins that a non-constant
// static field initializer evaluates in member order: a later static reads an
// earlier one and a module function's result flows into the field.
func TestStaticFieldComputedRuns(t *testing.T) {
	const src = `function twice(n: number): number { return n * 2; }
class Config {
  static base: number = 10;
  static doubled: number = twice(Config.base);
  static total: number = Config.base + Config.doubled;
}
console.log(String(Config.total));
`
	got := runProgramGo(t, src)
	if got != "30\n" {
		t.Errorf("computed static fields ran wrong\n got: %q\nwant: %q", got, "30\n")
	}
}

// TestStaticFieldComputedEmitsZeroVar pins the split: a constant static keeps its
// package-var initializer while a computed one declares the var zero-valued and
// runs its initializer as an assignment in the class's static init function.
func TestStaticFieldComputedEmitsZeroVar(t *testing.T) {
	const src = `function twice(n: number): number { return n * 2; }
class Config {
  static base: number = 10;
  static doubled: number = twice(Config.base);
}
console.log(String(Config.doubled));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func staticInitConfig()") {
		t.Errorf("computed static field did not lower to func staticInitConfig:\n%s", source)
	}
	// The constant keeps its value on the var declaration, the computed field
	// declares a bare zero-valued var and takes its value in the init function.
	if !strings.Contains(source, "var configBase float64 = 10") {
		t.Errorf("constant static base does not carry its value on the var decl:\n%s", source)
	}
	if !strings.Contains(source, "var configDoubled float64\n") {
		t.Errorf("computed static doubled did not emit a zero-valued var decl:\n%s", source)
	}
	if !strings.Contains(source, "configDoubled = Twice(configBase)") {
		t.Errorf("computed static doubled did not run its initializer in the init func:\n%s", source)
	}
}

// TestStaticFieldAndBlockInterleave pins that a computed field and a static block
// run in member order: the block observes the field the earlier declaration set,
// and a later field reads what the block wrote.
func TestStaticFieldAndBlockInterleave(t *testing.T) {
	const src = `class C {
  static a: number = 2;
  static b: number = C.a + 1;
  static { C.b = C.b * 10; }
  static c: number = C.b + 4;
}
console.log(String(C.c));
`
	got := runProgramGo(t, src)
	if got != "34\n" {
		t.Errorf("field/block interleave ran wrong\n got: %q\nwant: %q", got, "34\n")
	}
}

// TestStaticFieldThisHandsBack pins the boundary: a static field initializer that
// reads this touches the class constructor object, a dynamic-world value this
// slice does not model, so it hands back with its own named reason.
func TestStaticFieldThisHandsBack(t *testing.T) {
	const src = `class Counter {
  static start: number = 1;
  static self: any = (this as any);
}
new Counter();
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "a static field initializer that reads this is a later slice") {
		t.Errorf("hand-back reason %q does not name the this case", reason)
	}
}
