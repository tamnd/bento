package lower

import (
	"strings"
	"testing"
)

// TestNonEmptyArrayLiteralReturnsBoxed pins that a concretely-typed array literal
// returned as any[] re-emits at value.Value elements. The checker types [1] as
// float64[], which the *value.Array[value.Value] return slot rejects, so the literal
// boxes each element and rebuilds as value.NewArray[value.Value].
func TestNonEmptyArrayLiteralReturnsBoxed(t *testing.T) {
	const src = `function foo(): any[] { return [1]; }
console.log(foo().length);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(1))") {
		t.Errorf("array literal returned as any[] did not box its element:\n%s", source)
	}
}

// TestNonEmptyArrayLiteralArgBoxesRuns builds and runs a mixed literal flowing into an
// any[] parameter through a call: each element boxes to value.Value, so the argument
// fits the boxed header and the program reads the elements back.
func TestNonEmptyArrayLiteralArgBoxesRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function func1(stuff: any[]) { return stuff; }
function func2(a: string, b: number, c: number) {
  return func1([a, b, c]);
}
const r = func2("3", 1, 2);
console.log(r.length, r[0], r[1]);
`
	if got, want := runProgramGo(t, src), "3 3 1\n"; got != want {
		t.Fatalf("any[] arg boxing run mismatch:\n got %q\nwant %q", got, want)
	}
}
