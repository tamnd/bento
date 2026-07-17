package lower

import (
	"strings"
	"testing"
)

// TestTOrNullInternsTaggedSum pins that a T | null union interns as a two-arm tagged
// sum, a value arm and the null sentinel, rather than handing back on the object-union
// path. A number | null grows a NumOrNull struct with a tag and a num field, a wrapping
// constructor per arm, and a null compare narrows to a tag check while the else branch
// reads the value arm.
func TestTOrNullInternsTaggedSum(t *testing.T) {
	const src = `function f(x: number | null): string {
  if (x === null) return "n";
  return String(x + 1);
}
f(1);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"type NumOrNull struct {",
		"NumOrNullOfNum(v float64) NumOrNull",
		"NumOrNullOfNull() NumOrNull",
		"if x.tag == NumOrNullNull {",
		"x.num + 1",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("emitted Go missing %q\n%s", want, source)
		}
	}
}

// TestTOrNullTruthy pins that a T | null in boolean position reads its truth through
// the union's ToBoolean, so a present zero is falsy and any other number is truthy while
// the null arm rides the trailing false.
func TestTOrNullTruthy(t *testing.T) {
	const src = `function f(x: number | null): string {
  return x ? "t" : "f";
}
f(1);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "if x.ToBoolean() {") {
		t.Fatalf("number | null truthiness did not lower to ToBoolean\n%s", source)
	}
}

// TestTOrUndefinedStaysOpt pins that the sibling T | undefined keeps its leaner
// value.Opt path and does not intern as a tagged sum, so relaxing the single-value-arm
// guard for null did not disturb the optional.
func TestTOrUndefinedStaysOpt(t *testing.T) {
	const src = `function f(x: number | undefined): string {
  if (x === undefined) return "u";
  return String(x + 1);
}
f(1);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "OfNull") || strings.Contains(source, "NumOrUndef struct") {
		t.Fatalf("number | undefined should stay value.Opt, not a tagged sum\n%s", source)
	}
	if !strings.Contains(source, "value.Opt[float64]") {
		t.Fatalf("number | undefined did not keep the value.Opt slot\n%s", source)
	}
}

// TestTOrNullRun builds and runs a T | null across a number, string, and boolean inner
// through a null compare and a truthiness test and matches the oracle: the null arm
// takes the guard, a present value reads its arm, and a present zero is falsy.
func TestTOrNullRun(t *testing.T) {
	skipIfShort(t)
	const src = `function num(x: number | null): string {
  if (x === null) return "null";
  return String(x + 1);
}
function truthy(x: number | null): string {
  return x ? "t" : "f";
}
console.log(num(5));
console.log(num(null));
console.log(truthy(0));
console.log(truthy(4));
console.log(truthy(null));
`
	got := runProgramGo(t, src)
	want := "6\nnull\nf\nt\nf\n"
	if got != want {
		t.Fatalf("T | null run mismatch:\n got %q\nwant %q", got, want)
	}
}
