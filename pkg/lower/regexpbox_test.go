package lower

import (
	"strings"
	"testing"
)

// TestRegexpBoxLowers pins that a regexp flowing into an any binding boxes through
// value.RegExpValue, so the concrete *value.RegExp enters the dynamic slot as an
// object box the value model can carry.
func TestRegexpBoxLowers(t *testing.T) {
	src := `
const r: any = /ab+c/gi;
console.log(String(r));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.RegExpValue(value.NewRegExpLiteral(\"ab+c\", \"gi\"))") {
		t.Fatalf("a regexp into an any binding did not box through value.RegExpValue:\n%s", out)
	}
}

// TestRegexpBoxRuns builds and runs the emitted Go: a boxed regexp reports typeof
// "object", stringifies to its literal form, reads its own accessors off the live
// regexp, and is truthy, all through the dynamic value model.
func TestRegexpBoxRuns(t *testing.T) {
	src := `
const r: any = /ab+c/gi;
console.log(String(r));
console.log(typeof r);
console.log(r.source);
console.log(r.flags);
console.log(r.global);
console.log(r ? "truthy" : "falsy");
`
	got := runProgramGo(t, src)
	want := "/ab+c/gi\nobject\nab+c\ngi\ntrue\ntruthy\n"
	if got != want {
		t.Fatalf("boxed regexp ran wrong\n got: %q\nwant: %q", got, want)
	}
}
