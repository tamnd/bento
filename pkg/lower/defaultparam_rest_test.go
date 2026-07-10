package lower

import (
	"strings"
	"testing"
)

// A rest parameter that follows a default parameter gathers the trailing arguments
// after the defaulted slot is filled. The call site fills the omitted default in its
// slot and packs whatever is left into the rest array, so the body reads its default
// and its gathered rest with no special casing.

// TestDefaultThenRestFillsAndGathers proves an omitted default before a rest lowers to
// the default in its slot while the rest still gathers the tail into an array.
func TestDefaultThenRestFillsAndGathers(t *testing.T) {
	const src = "function f(a: number, b: number = 1, ...rest: number[]): number { return a + b; }\n" +
		"f(1);\n" +
		"f(1, 2);\n" +
		"f(1, 2, 3, 4);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "rest *value.Array[float64]") {
		t.Errorf("the rest parameter after a default did not lower to an array field:\n%s", source)
	}
	if !strings.Contains(source, "F(1, 1, value.NewArray[float64]())") {
		t.Errorf("an omitted default before a rest did not fill with the default:\n%s", source)
	}
	if !strings.Contains(source, "F(1, 2, value.NewArray[float64](3, 4))") {
		t.Errorf("the rest did not gather the trailing arguments:\n%s", source)
	}
}

// TestDefaultThenRestRuns builds and runs a default-then-rest function with the
// default omitted, the default supplied, and a gathered tail, so the filled default
// and the gathered rest are proven against the JavaScript result.
func TestDefaultThenRestRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(a: number, b: number = 1, ...rest: number[]): number {
  let s = a + b;
  for (const r of rest) {
    s = s + r;
  }
  return s;
}
console.log(f(1));
console.log(f(1, 2));
console.log(f(1, 2, 3, 4));
`
	if got, want := runProgramGo(t, src), "2\n3\n10\n"; got != want {
		t.Fatalf("default then rest printed %q, want %q", got, want)
	}
}
