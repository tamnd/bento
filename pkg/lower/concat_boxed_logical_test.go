package lower

import "testing"

// A string concatenation whose operand is a dynamic-operand && / || lowers that
// operand to value.And / value.Or, a boxed value.Value, even though the checker
// types the logical as the union of its arms (string | null for arr && arr[0]).
// The concat string-coercion must read that box through value.ToString; the
// union-method path would emit a .ToString() the box does not carry and fail to
// compile. This is the shape test262 regexp assertion messages take:
// throw new Error('#1: Actual. ' + (arr && arr[0])).
func TestConcatBoxedLogicalReadsThroughValueToString(t *testing.T) {
	skipIfShort(t)
	src := "var arr = /\\t/.exec(\"\\t\");\n" +
		"console.log('Actual. ' + (arr && arr[0]));\n"
	if got := runProgramGoTolerant(t, src); got != "Actual. \t\n" {
		t.Fatalf("concat of boxed logical: got %q, want %q", got, "Actual. \t\n")
	}
}

// The null arm of the same shape concatenates as "null": a non-matching exec
// returns null, and 'x' + (null && ...) reads the box as "null", matching Node.
func TestConcatBoxedLogicalNullArm(t *testing.T) {
	skipIfShort(t)
	src := "var arr = /z/.exec(\"a\");\n" +
		"console.log('Actual. ' + (arr && arr[0]));\n"
	if got := runProgramGoTolerant(t, src); got != "Actual. null\n" {
		t.Fatalf("concat of boxed logical null arm: got %q, want %q", got, "Actual. null\n")
	}
}
