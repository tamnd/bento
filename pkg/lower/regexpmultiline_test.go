package lower

import (
	"strings"
	"testing"
)

// An anchored multiline pattern used to hand back because RE2 breaks a line only at
// \n while the language breaks it at four characters. The runtime searches a
// normalized subject now, so the pattern lowers; the refusal that is left is the one
// the normalization cannot be faithful for.

// TestAnAnchoredMultilinePatternLowers is the shape the Node compat suite reads a
// file with, and the top gate before this slice.
func TestAnAnchoredMultilinePatternLowers(t *testing.T) {
	got := renderProgram(t, `const text: any = "a\nb";
console.log(/^Hardware\s*:\s*(.*)$/im.exec(text)?.[1]);`)
	if !strings.Contains(got, `value.NewRegExpLiteral("^Hardware\\s*:\\s*(.*)$", "im")`) {
		t.Errorf("an anchored multiline pattern did not lower:\n%s", got)
	}
}

// TestAMultilinePatternTellingTerminatorsApartHandsBack keeps the limit honest. The
// normalization rewrites a subject's \r to \n, which is invisible only to a pattern
// that cannot tell the two apart; one that names a terminator would read the rewrite,
// so it still refuses.
func TestAMultilinePatternTellingTerminatorsApartHandsBack(t *testing.T) {
	cases := []struct{ name, pattern string }{
		{"a named terminator", `^a\r$`},
		{"a class holding one of them", `^a[^\n]$`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := renderProgramHandBack(t, `const s = "a";
console.log(/`+tc.pattern+`/m.test(s));
`)
			if !strings.Contains(reason, "tells the line terminators apart") {
				t.Errorf("hand back said %q, want it to name the terminators", reason)
			}
		})
	}
}

// TestAnUnanchoredMultilinePatternStillLowers pins that the guard reaches only the
// patterns the normalization runs for. Without a ^ or $ the m flag changes nothing a
// match can see, so a pattern that reads a \r directly is untouched by any of this.
func TestAnUnanchoredMultilinePatternStillLowers(t *testing.T) {
	got := renderProgram(t, `const s = "a\rb";
console.log(/a\r/m.test(s));`)
	if !strings.Contains(got, "value.NewRegExpLiteral(") {
		t.Errorf("an unanchored multiline pattern did not lower:\n%s", got)
	}
}
