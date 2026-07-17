package lower

import (
	"strings"
	"testing"
)

// TestStringApplyDynamicArrayEmits pins that String.fromCodePoint.apply over an any[]
// array lowers to the coercing value.FromCodePointValues over the array's Elems, the
// runtime that runs the ToNumber apply applies to each boxed element, rather than handing
// back on the non-number array guard.
func TestStringApplyDynamicArrayEmits(t *testing.T) {
	const src = "function make(points: any[]): string { return String.fromCodePoint.apply(null, points); }\nconsole.log(make([104, 105]));\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.FromCodePointValues(") {
		t.Fatalf("fromCodePoint.apply over an any[] did not lower to FromCodePointValues:\n%s", out)
	}
	if !strings.Contains(out, ".Elems()...)") {
		t.Fatalf("fromCodePoint.apply did not spread the array Elems:\n%s", out)
	}
}

// TestStringApplyDynamicArrayFromCharCode pins the fromCharCode sibling lowers to the
// coercing value.FromCharCodeValues the same way.
func TestStringApplyDynamicArrayFromCharCode(t *testing.T) {
	const src = "function make(codes: any[]): string { return String.fromCharCode.apply(null, codes); }\nconsole.log(make([88, 89]));\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.FromCharCodeValues(") {
		t.Fatalf("fromCharCode.apply over an any[] did not lower to FromCharCodeValues:\n%s", out)
	}
}

// TestStringApplyNumberArrayStillPlain pins that a statically number[] array keeps the
// plain value.FromCodePoint over Elems, unchanged by the dynamic-array branch.
func TestStringApplyNumberArrayStillPlain(t *testing.T) {
	const src = "function make(points: number[]): string { return String.fromCodePoint.apply(null, points); }\nconsole.log(make([104, 105]));\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.FromCodePoint(") || strings.Contains(out, "value.FromCodePointValues(") {
		t.Fatalf("fromCodePoint.apply over a number[] should stay the plain FromCodePoint:\n%s", out)
	}
}

// TestStringApplyBareAnyHandsBack pins that apply over a bare any value, which is not a
// statically known array, still hands back rather than emit an Elems read on a value the
// Go form does not expose one on.
func TestStringApplyBareAnyHandsBack(t *testing.T) {
	const src = "function make(points: any): string { return String.fromCodePoint.apply(null, points); }\nconsole.log(make([104, 105]));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "non-array value") {
		t.Fatalf("apply over a bare any handed back for the wrong reason: %q", reason)
	}
}

// TestStringApplyDynamicArrayRuns builds and runs fromCodePoint/fromCharCode apply over an
// any[] end to end: plain numbers pass through, a numeric string coerces the way ToNumber
// does, an astral point becomes a surrogate pair, and the empty array yields the empty
// string.
func TestStringApplyDynamicArrayRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function fromPoints(a: any, b: any): string {
  const points: any[] = [a, b];
  return String.fromCodePoint.apply(null, points);
}
function fromCodes(a: any, b: any, c: any): string {
  const codes: any[] = [a, b, c];
  return String.fromCharCode.apply(null, codes);
}
function emptyChars(): string {
  const codes: any[] = [];
  return String.fromCharCode.apply(null, codes);
}
console.log(fromPoints(104, 105));
console.log(fromPoints(65, "66"));
console.log(fromPoints(0x1f600, 0x1f4a9));
console.log(fromCodes(88, 89, 90));
console.log(emptyChars());
`
	got := runProgramGo(t, src)
	want := "hi\n" +
		"AB\n" +
		"\U0001f600\U0001f4a9\n" +
		"XYZ\n" +
		"\n"
	if got != want {
		t.Fatalf("dynamic-array apply run mismatch:\n got %q\nwant %q", got, want)
	}
}
