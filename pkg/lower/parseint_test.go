package lower

import (
	"strings"
	"testing"
)

// The lib signature for parseInt takes a string and a number radix, so a
// non-string first argument or a non-number radix is a code 2345 the front door
// tolerates. parseInt runs ToString on the value and ToNumber on the radix, so the
// renderer boxes each mismatched argument and coerces it, recording the 2345 span
// as handled. Before this slice either mismatch handed the unit back.

// TestParseIntNonStringLowersThroughToString pins that a non-string first argument
// lowers through value.ToString then value.ParseInt rather than handing back.
func TestParseIntNonStringLowersThroughToString(t *testing.T) {
	src := "console.log(parseInt(true));\n"
	out := renderProgramTolerant(t, src)
	for _, want := range []string{"value.ToString(", "value.ParseInt("} {
		if !strings.Contains(out, want) {
			t.Fatalf("parseInt over a non-string did not lower through %q:\n%s", want, out)
		}
	}
}

// TestParseIntNonNumberRadixLowersThroughToNumber pins that a non-number radix
// lowers through value.ToNumber rather than handing back.
func TestParseIntNonNumberRadixLowersThroughToNumber(t *testing.T) {
	src := "console.log(parseInt(\"11\", \"8\"));\n"
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.ToNumber(") {
		t.Fatalf("parseInt with a string radix did not coerce through value.ToNumber:\n%s", out)
	}
}

// TestParseIntCoercionRuns builds and runs parseInt over mismatched arguments
// against the Node oracle: a number first argument stringifies (15.99 -> "15.99")
// and parses its integer prefix to 15; a string radix coerces to a number (parseInt
// of "11" in base 8 is 9); and a boolean stringifies to "true" which has no digit
// prefix and parses to NaN.
func TestParseIntCoercionRuns(t *testing.T) {
	skipIfShort(t)
	src := `
console.log(parseInt(15.99, 10));
console.log(parseInt("11", "8"));
console.log(parseInt(true));
`
	got := runProgramGoTolerant(t, src)
	want := "15\n9\nNaN\n"
	if got != want {
		t.Fatalf("parseInt coercion run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestParseIntStringStillDirect pins the well-typed path is unchanged: a string
// value and a number radix parse directly through value.ParseInt with no box.
func TestParseIntStringStillDirect(t *testing.T) {
	src := "console.log(parseInt(\"101\", 2));\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.ParseInt(") {
		t.Fatalf("parseInt over a string did not lower to value.ParseInt:\n%s", out)
	}
	if strings.Contains(out, "value.ToString(") || strings.Contains(out, "value.ToNumber(") {
		t.Fatalf("parseInt over well-typed arguments should not box:\n%s", out)
	}
}
