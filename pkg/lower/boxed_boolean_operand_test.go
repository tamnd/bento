package lower

import (
	"strings"
	"testing"
)

// A boolean-typed operand whose runtime lowering is a value.Value box (a method
// call on a generic receiver, obj.hasOwnProperty(k), typed boolean by the checker
// but dispatched through the runtime Call) must be read back through ToBoolean
// before a Go && can carry it, otherwise the operator compares a value.Value where
// Go wants a bool and the program fails to build. This is the shape the test262
// Object.defineProperty enumerable-check tests take.
func TestBoxedBooleanOperandInAndCoerces(t *testing.T) {
	src := "var obj = {};\n" +
		"var item = \"property\";\n" +
		"var r = obj.hasOwnProperty(item) && item === \"property\";\n" +
		"console.log(r);\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.ToBoolean(") {
		t.Fatalf("expected boxed boolean operand to be wrapped in value.ToBoolean, got:\n%s", out)
	}
}

// End to end, obj.hasOwnProperty(item) && item === "property" over a property
// defined with Object.defineProperty runs and prints true, matching Node: the
// property is own and enumerable, so the for-in loop finds it and the guard holds.
func TestBoxedBooleanOperandRuns(t *testing.T) {
	src := "var obj = {};\n" +
		"Object.defineProperty(obj, \"property\", { enumerable: true });\n" +
		"var isEnumerable = false;\n" +
		"for (var item in obj) {\n" +
		"  if (obj.hasOwnProperty(item) && item === \"property\") {\n" +
		"    isEnumerable = true;\n" +
		"  }\n" +
		"}\n" +
		"console.log(isEnumerable);\n"
	if got := runProgramGo(t, src); strings.TrimSpace(got) != "true" {
		t.Fatalf("boxed boolean && ran wrong: got %q, want %q", got, "true")
	}
}
