package lower

import (
	"strings"
	"testing"
)

// A default parameter on a method fills the omitted argument at the call site, the
// same call-site defaulting a top-level function takes. A method is always called as
// `recv.Method(...)` or `Class.Method(...)`, never a bare flexible-arity func value,
// so the call site can always reconstruct the default. The parameter lowers to a
// plain Go field of its type.

// TestDefaultParamOnMethodFillsOmittedArg proves an omitted trailing argument on an
// instance method and on a static method lowers to the parameter's default at the
// call site, while a provided argument passes through, and both defaults lower to
// plain Go fields.
func TestDefaultParamOnMethodFillsOmittedArg(t *testing.T) {
	const src = "class Box {\n" +
		"  scale(x: number, by: number = 2): number { return x * by; }\n" +
		"  static make(w: number = 3): number { return w * w; }\n" +
		"}\n" +
		"const b = new Box();\n" +
		"b.scale(5);\n" +
		"b.scale(5, 3);\n" +
		"Box.make();\n" +
		"Box.make(4);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.Scale(5, 2)") {
		t.Errorf("an omitted default on an instance method did not fill with the default:\n%s", source)
	}
	if !strings.Contains(source, "b.Scale(5, 3)") {
		t.Errorf("a provided argument on an instance method did not pass through:\n%s", source)
	}
	if !strings.Contains(source, "BoxMake(3)") {
		t.Errorf("an omitted default on a static method did not fill with the default:\n%s", source)
	}
	if !strings.Contains(source, "BoxMake(4)") {
		t.Errorf("a provided argument on a static method did not pass through:\n%s", source)
	}
	if !strings.Contains(source, "func (b *Box) Scale(x float64, by float64)") {
		t.Errorf("the instance method default did not lower to a plain Go field:\n%s", source)
	}
	if !strings.Contains(source, "func BoxMake(w float64)") {
		t.Errorf("the static method default did not lower to a plain Go field:\n%s", source)
	}
}

// TestDefaultParamOnMethodRuns builds and runs the instance and static method forms,
// both omitted and supplied, so the filled default is proven against the JavaScript
// result rather than just the emitted shape.
func TestDefaultParamOnMethodRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
class Box {
  scale(x: number, by: number = 2): number {
    return x * by;
  }
  static make(w: number = 3): number {
    return w * w;
  }
}
const b = new Box();
console.log(b.scale(5));
console.log(b.scale(5, 3));
console.log(Box.make());
console.log(Box.make(4));
`
	if got, want := runProgramGo(t, src), "10\n15\n9\n16\n"; got != want {
		t.Fatalf("method default parameters printed %q, want %q", got, want)
	}
}

// TestDefaultParamOnMethodReadingEarlierParamHandsBack proves a method default that
// reads an earlier parameter still hands back: the callee-scope variadic tail that
// top-level functions use for this form is not wired for methods, so bento declines
// rather than emit a call that cannot fill the read.
func TestDefaultParamOnMethodReadingEarlierParamHandsBack(t *testing.T) {
	const src = "class Box {\n" +
		"  add(a: number, b: number = a + 1): number { return a + b; }\n" +
		"}\n" +
		"const b = new Box();\n" +
		"b.add(5);\n"
	renderProgramHandBack(t, src)
}
