package lower

import (
	"strings"
	"testing"
)

// The lib signature for parseFloat takes a string, so a non-string argument is a
// code 2345 the front door tolerates. parseFloat runs ToString on its argument
// first, so the renderer boxes the argument and stringifies it through
// value.ToString before the lenient prefix parse, and records the 2345 span as
// handled so the end-of-render reconciliation does not hand the unit back for it.
// Before this slice a non-string argument handed back.

// TestParseFloatNonStringLowersThroughToString pins the lowering: a boolean
// argument, a 2345, lowers through value.ToString then value.ParseFloat rather
// than handing back.
func TestParseFloatNonStringLowersThroughToString(t *testing.T) {
	src := "console.log(parseFloat(true));\n"
	out := renderProgramTolerant(t, src)
	for _, want := range []string{"value.ToString(", "value.ParseFloat("} {
		if !strings.Contains(out, want) {
			t.Fatalf("parseFloat over a non-string did not lower through %q:\n%s", want, out)
		}
	}
}

// TestParseFloatNonStringRuns builds and runs parseFloat over non-string
// arguments against the Node oracle: a number stringifies and parses back to
// itself, a boolean stringifies to "true" which has no numeric prefix and parses
// to NaN, and null stringifies to "null" which likewise parses to NaN.
func TestParseFloatNonStringRuns(t *testing.T) {
	skipIfShort(t)
	src := `
console.log(parseFloat(1.5));
console.log(parseFloat(true));
console.log(parseFloat(null));
`
	got := runProgramGoTolerant(t, src)
	want := "1.5\nNaN\nNaN\n"
	if got != want {
		t.Fatalf("parseFloat coercion run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestParseFloatStringStillDirect pins the string path is unchanged: a statically
// string argument parses directly through value.ParseFloat with no ToString box.
func TestParseFloatStringStillDirect(t *testing.T) {
	src := "const s = \"3.14x\"; console.log(parseFloat(s));\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.ParseFloat(") {
		t.Fatalf("parseFloat over a string did not lower to value.ParseFloat:\n%s", out)
	}
	if strings.Contains(out, "value.ToString(") {
		t.Fatalf("parseFloat over a string should not box through ToString:\n%s", out)
	}
}
