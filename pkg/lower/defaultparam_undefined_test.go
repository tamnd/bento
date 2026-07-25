package lower

import (
	"strings"
	"testing"
)

// An explicit undefined argument in a defaulted slot counts as a missing argument:
// the language fills the parameter's default for undefined exactly as it does for an
// omission. bento substitutes the default at the call site rather than lowering
// undefined into a slot whose static type cannot hold it. This holds for a top-level
// function and for a method, and only when the default is call-site-reconstructible.

// TestDefaultParamExplicitUndefinedFillsDefault proves an explicit undefined in a
// defaulted slot lowers to the parameter's default, while a supplied argument passes
// through, on a top-level function.
func TestDefaultParamExplicitUndefinedFillsDefault(t *testing.T) {
	const src = "function inc(x: number, by: number = 1): number { return x + by; }\n" +
		"inc(5, undefined);\n" +
		"inc(5, 3);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Inc(5, 1)") {
		t.Errorf("an explicit undefined did not fill with the default:\n%s", source)
	}
	if !strings.Contains(source, "Inc(5, 3)") {
		t.Errorf("a supplied argument did not pass through:\n%s", source)
	}
}

// TestDefaultParamVoidZeroFillsDefault proves that void 0, which always evaluates to
// undefined, fills a defaulted slot the same way the undefined literal does: the fold
// hands the parameter its default, so an undefined never lands in a slot whose static
// type (float64 here) cannot hold it. This is the shape the test262 dflt-params tests
// take when they pass void 0 for a defaulted argument.
func TestDefaultParamVoidZeroFillsDefault(t *testing.T) {
	const src = "function ref(fromLiteral = 23, fromExpr = 45, fromHole = 99): number {\n" +
		"  return fromLiteral + fromExpr + fromHole;\n" +
		"}\n" +
		"ref(undefined, void 0);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Ref(23, 45, 99)") {
		t.Errorf("void 0 did not fill with the default:\n%s", source)
	}
	if strings.Contains(source, "value.Undefined") {
		t.Errorf("void 0 lowered to value.Undefined in a float64 slot instead of the default:\n%s", source)
	}
}

// TestDefaultParamVoidSideEffectHandsBack proves the fold is withheld when the void
// operand could run: void applied to a call keeps the operand's effect, so it is not
// folded away to the default. Such a void hands back at lowering rather than dropping
// the call, so this must not silently substitute the default.
func TestDefaultParamVoidSideEffectHandsBack(t *testing.T) {
	const src = "let n = 0;\n" +
		"function bump(): number { n = n + 1; return n; }\n" +
		"function ref(by: number = 1): number { return by; }\n" +
		"ref(void bump());\n"
	if reason := renderProgramHandBack(t, src); reason == "" {
		t.Fatalf("a void over a call in a defaulted slot should hand back, not fold to the default")
	}
}

// TestDefaultParamExplicitUndefinedOnMethodFillsDefault proves the same undefined
// substitution on an instance method, whose call site can always reconstruct the
// default.
func TestDefaultParamExplicitUndefinedOnMethodFillsDefault(t *testing.T) {
	const src = "class Box {\n" +
		"  scale(x: number, by: number = 2): number { return x * by; }\n" +
		"}\n" +
		"const b = new Box();\n" +
		"b.scale(5, undefined);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.Scale(5, 2)") {
		t.Errorf("an explicit undefined on a method did not fill with the default:\n%s", source)
	}
}

// TestDefaultParamExplicitUndefinedRuns builds and runs the function and method forms
// so the substituted default is proven against the JavaScript result.
func TestDefaultParamExplicitUndefinedRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function inc(x: number, by: number = 1): number {
  return x + by;
}
class Box {
  scale(x: number, by: number = 2): number {
    return x * by;
  }
}
const b = new Box();
console.log(inc(5, undefined));
console.log(inc(5, 3));
console.log(b.scale(5, undefined));
console.log(b.scale(5, 4));
`
	if got, want := runProgramGo(t, src), "6\n8\n10\n20\n"; got != want {
		t.Fatalf("explicit undefined defaulting printed %q, want %q", got, want)
	}
}
