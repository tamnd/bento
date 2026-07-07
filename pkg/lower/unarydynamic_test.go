package lower

import (
	"strings"
	"testing"
)

// TestDynamicUnaryMinusCoerces pins unary minus on a dynamic operand: the value
// coerces through value.ToNumber and the Go minus applies to the resulting
// float64, so -x on an any-typed value reads -value.ToNumber(x).
func TestDynamicUnaryMinusCoerces(t *testing.T) {
	src := "function f(x: any): number { return -x; }\nconsole.log(f(5));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "-value.ToNumber(x)") {
		t.Errorf("dynamic unary minus did not coerce through ToNumber:\n%s", source)
	}
}

// TestDynamicUnaryPlusCoerces pins unary plus on a dynamic operand: plus is
// ToNumber and nothing else, so +x lowers to the bare value.ToNumber(x) call.
func TestDynamicUnaryPlusCoerces(t *testing.T) {
	src := "function f(x: any): number { return +x; }\nconsole.log(f(\"5\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "return value.ToNumber(x)") {
		t.Errorf("dynamic unary plus did not lower to a bare ToNumber:\n%s", source)
	}
}

// TestDynamicBitwiseNotCoerces pins ~x on a dynamic operand: it coerces through
// ToNumber, narrows to a 32-bit integer with ToInt32, complements, and widens
// back to a number, so ~x reads float64(^value.ToInt32(value.ToNumber(x))).
func TestDynamicBitwiseNotCoerces(t *testing.T) {
	src := "function f(x: any): number { return ~x; }\nconsole.log(f(5));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "float64(^value.ToInt32(value.ToNumber(x)))") {
		t.Errorf("dynamic bitwise not did not coerce through ToNumber then ToInt32:\n%s", source)
	}
}

// TestDynamicUnaryRuns builds and runs the three dynamic unary operators and
// matches the JavaScript answers: minus and plus coerce a numeric string, and
// bitwise not of 5 is -6.
func TestDynamicUnaryRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function neg(x: any): number { return -x; }
function pos(x: any): number { return +x; }
function bnot(x: any): number { return ~x; }
console.log(neg("5"));
console.log(pos("5"));
console.log(bnot(5));
console.log(neg(true));
`
	got := runProgramGo(t, src)
	want := "-5\n5\n-6\n-1\n"
	if got != want {
		t.Fatalf("dynamic unary program printed %q, want %q", got, want)
	}
}
