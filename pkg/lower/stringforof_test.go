package lower

import (
	"strings"
	"testing"
)

// A for...of over a string iterates its Unicode code points, one substring per
// code point, so it lowers to a range over the string's CodePoints() the same way
// an array for...of ranges over Elems(). A loop that never reads the binding ranges
// with no loop variable so the generated Go compiles, matching the counting idiom.

// TestStringForOfLowers proves a for...of over a string ranges over CodePoints with
// the loop variable bound, when the body reads it.
func TestStringForOfLowers(t *testing.T) {
	const src = "export function f(): string { let out = \"\"; for (const c of \"abc\") out = out + c; return out; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".CodePoints()") {
		t.Errorf("string for...of did not lower to a range over CodePoints:\n%s", source)
	}
	if !strings.Contains(source, "for _, c := range") {
		t.Errorf("string for...of did not bind the read loop variable:\n%s", source)
	}
}

// TestStringForOfUnusedBindingRangesBare proves a for...of whose binding the body
// never reads (a counting loop) ranges with no loop variable, since Go rejects an
// unused range value.
func TestStringForOfUnusedBindingRangesBare(t *testing.T) {
	const src = "export function f(): number { let n = 0; for (const c of \"abc\") n = n + 1; return n; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for range") {
		t.Errorf("counting string for...of did not drop the unused binding:\n%s", source)
	}
	if strings.Contains(source, ":= range") {
		t.Errorf("counting string for...of should not declare a loop variable:\n%s", source)
	}
}

// TestStringForOfRuns builds and runs the generated Go so the code-point iteration
// is proven, including an astral character that is one code point of two code units.
func TestStringForOfRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let out = "";
for (const c of "abc") out = out + c + "-";
console.log(out);

let count = 0;
for (const c of "a\u{1F600}b") count = count + 1;
console.log(count);

let n = 0;
for (const c of "hello") n = n + 1;
console.log(n);
`
	if got, want := runProgramGo(t, src), "a-b-c-\n3\n5\n"; got != want {
		t.Fatalf("string for...of printed %q, want %q", got, want)
	}
}

// TestArrayForOfUnusedBindingRangesBare proves the unused-binding rule also covers
// an array for...of, which shares the same lowering and would otherwise emit an
// unused Go loop variable.
func TestArrayForOfUnusedBindingRangesBare(t *testing.T) {
	const src = "export function f(): number { let n = 0; for (const x of [1, 2, 3]) n = n + 1; return n; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for range") {
		t.Errorf("counting array for...of did not drop the unused binding:\n%s", source)
	}
}
