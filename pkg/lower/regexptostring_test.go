package lower

import (
	"strings"
	"testing"
)

// TestRegexpStringifyLowers pins that the four ways a regexp becomes a string all
// lower to the runtime ToStringBStr method, "/" + source + "/" + flags, read off the
// concrete *value.RegExp with no boxing: String(re), a template substitution, string
// concatenation with +, and an explicit re.toString().
func TestRegexpStringifyLowers(t *testing.T) {
	src := `
const re = /ab+c/gi;
console.log(String(re));
console.log("x=" + re);
console.log(` + "`t:${re}`" + `);
console.log(re.toString());
`
	out := renderProgram(t, src)
	if n := strings.Count(out, ".ToStringBStr()"); n != 4 {
		t.Fatalf("expected 4 ToStringBStr calls (String, +, template, toString), got %d:\n%s", n, out)
	}
	if strings.Contains(out, "value.ToString(") {
		t.Fatalf("a regexp coercion boxed to the dynamic ToString instead of the concrete method:\n%s", out)
	}
}

// TestRegexpStringifyRuns builds and runs the emitted Go end to end: each form prints
// the literal the program wrote, and the empty pattern round-trips through its "(?:)"
// source.
func TestRegexpStringifyRuns(t *testing.T) {
	src := `
const re = /ab+c/gi;
console.log(String(re));
console.log("x=" + re);
console.log(re.toString());
const empty = /(?:)/;
console.log(String(empty));
`
	got := runProgramGo(t, src)
	want := "/ab+c/gi\nx=/ab+c/gi\n/ab+c/gi\n/(?:)/\n"
	if got != want {
		t.Fatalf("regexp stringify ran wrong\n got: %q\nwant: %q", got, want)
	}
}
