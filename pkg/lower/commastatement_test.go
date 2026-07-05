package lower

import (
	"strings"
	"testing"
)

// A comma expression in statement position evaluates its operands left to right
// and throws the result away, so it lowers to the Go statements those operands
// spell, one per operand, rather than handing back. This covers a line that
// sequences two assignments or two calls.

// TestCommaStatementFlattens proves a two-assignment comma lowers to two Go
// assignment statements in source order.
func TestCommaStatementFlattens(t *testing.T) {
	const src = "let a = 0; let b = 0; a = 1, b = 2;\n"
	source := renderProgram(t, src)
	ia := strings.Index(source, "a = 1")
	ib := strings.Index(source, "b = 2")
	if ia < 0 || ib < 0 {
		t.Fatalf("comma statement did not lower to both assignments:\n%s", source)
	}
	if ia > ib {
		t.Errorf("comma statement lowered its operands out of order:\n%s", source)
	}
}

// TestCommaStatementRuns builds and runs the assembled Go so the left-to-right
// evaluation and the discarded result are proven for assignments and for calls.
func TestCommaStatementRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let a = 0;
let b = 0;
let c = 0;
a = 1, b = 2, c = 3;
console.log(a + b + c);
`
	if got, want := runProgramGo(t, src), "6\n"; got != want {
		t.Fatalf("comma statement printed %q, want %q", got, want)
	}
}

// TestCommaStatementInFunctionRuns proves the same flattening inside a function
// body, which shares the statement lowering with the module top level.
func TestCommaStatementInFunctionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(): number {
  let a = 0;
  let b = 0;
  a = 5, b = 7;
  return a + b;
}
console.log(f());
`
	if got, want := runProgramGo(t, src), "12\n"; got != want {
		t.Fatalf("comma statement in a function printed %q, want %q", got, want)
	}
}
