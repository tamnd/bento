package lower

import (
	"strings"
	"testing"
)

// A top-level value whose JSON form is undefined, a function, a symbol, or undefined
// itself, makes JSON.stringify return the value undefined rather than a string. The
// typed call result is a string, so there is no BStr that equals undefined; the unit
// hands back rather than emit "" where Node produces undefined. These pin each shape.
func TestJSONStringifyTopLevelFunctionHandsBack(t *testing.T) {
	reason := renderProgramTolerantHandBack(t, "console.log(JSON.stringify(function() {}));\n")
	if !strings.Contains(reason, "JSON form is undefined") {
		t.Fatalf("stringify of a top-level function reason = %q, want the undefined-form handback", reason)
	}
}

func TestJSONStringifyTopLevelSymbolHandsBack(t *testing.T) {
	reason := renderProgramTolerantHandBack(t, "const s: symbol = Symbol('d');\nconsole.log(JSON.stringify(s));\n")
	if !strings.Contains(reason, "JSON form is undefined") {
		t.Fatalf("stringify of a top-level symbol reason = %q, want the undefined-form handback", reason)
	}
}

func TestJSONStringifyTopLevelUndefinedHandsBack(t *testing.T) {
	reason := renderProgramTolerantHandBack(t, "console.log(JSON.stringify(undefined));\n")
	if !strings.Contains(reason, "JSON form is undefined") {
		t.Fatalf("stringify of a top-level undefined reason = %q, want the undefined-form handback", reason)
	}
}

// The guard is scoped to a top-level function, symbol, or undefined; an ordinary
// serializable argument, an array of numbers, must not trip it, so the program still
// lowers and runs through the plain serializer.
func TestJSONStringifyOrdinaryArgStillRuns(t *testing.T) {
	skipIfShort(t)
	src := "console.log(JSON.stringify([1, 2, 3]));\n"
	if got := runProgramGoTolerant(t, src); got != "[1,2,3]\n" {
		t.Fatalf("stringify of a number array = %q, want [1,2,3]", got)
	}
}
