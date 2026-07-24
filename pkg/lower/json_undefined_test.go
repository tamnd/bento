package lower

import "testing"

// A top-level value whose JSON form is undefined, a function, a symbol, or undefined
// itself, makes JSON.stringify return the value undefined rather than a string. The
// call lowers to value.JSONStringifyUndefined, which returns the undefined Value, and
// isDynamic keeps the box on the dynamic path, so a dynamic sink prints "undefined"
// exactly as Node does. These run each shape end to end.
func TestJSONStringifyTopLevelFunctionRuns(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, "console.log(JSON.stringify(function() {}));\n"); got != "undefined\n" {
		t.Fatalf("stringify of a top-level function = %q, want undefined", got)
	}
}

func TestJSONStringifyTopLevelSymbolRuns(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, "const s: symbol = Symbol('d');\nconsole.log(JSON.stringify(s));\n"); got != "undefined\n" {
		t.Fatalf("stringify of a top-level symbol = %q, want undefined", got)
	}
}

func TestJSONStringifyTopLevelUndefinedRuns(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, "console.log(JSON.stringify(undefined));\n"); got != "undefined\n" {
		t.Fatalf("stringify of a top-level undefined = %q, want undefined", got)
	}
}

// A top-level value whose JSON form is undefined flowing into a string slot cannot be
// represented (there is no BStr that equals undefined), so that consumer still hands
// back rather than ship a wrong string.
func TestJSONStringifyUndefinedIntoStringSlotHandsBack(t *testing.T) {
	reason := renderProgramTolerantHandBack(t, "const s: string = JSON.stringify(function() {});\nconsole.log(s);\n")
	if reason == "" {
		t.Fatal("stringify of a top-level function into a string slot lowered, want a handback")
	}
}

// The undefined-form path is scoped to a top-level function, symbol, or undefined; an
// ordinary serializable argument, an array of numbers, must not trip it, so the program
// still lowers and runs through the plain serializer.
func TestJSONStringifyOrdinaryArgStillRuns(t *testing.T) {
	skipIfShort(t)
	src := "console.log(JSON.stringify([1, 2, 3]));\n"
	if got := runProgramGoTolerant(t, src); got != "[1,2,3]\n" {
		t.Fatalf("stringify of a number array = %q, want [1,2,3]", got)
	}
}
