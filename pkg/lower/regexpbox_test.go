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

// TestRegexpArrayElemNotReboxed pins that a RegExp[] element read flowing into a
// dynamic slot does NOT re-wrap in value.RegExpValue: a RegExp[] backs its elements in
// value.Value, so the read is already a box and value.RegExpValue (which takes an
// unboxed *value.RegExp) would not compile over it. The scalar case above still wraps.
func TestRegexpArrayElemNotReboxed(t *testing.T) {
	src := `
function notSame(a: unknown, b: unknown): void { if (a === b) throw new Error("same"); }
var values = [];
for (var i = 0; i < 2; ++i) { values[i] = /(?:)/; }
notSame(values[0], values[1]);
`
	out := renderProgram(t, src)
	if strings.Contains(out, "value.RegExpValue(values.At") {
		t.Fatalf("a RegExp[] element read was re-wrapped in value.RegExpValue:\n%s", out)
	}
}

// TestRegexpArrayElemRuns builds and runs the emitted Go: two distinct regexp literals
// stored in a RegExp[] are not identical, matching the spec, so the guard does not throw.
func TestRegexpArrayElemRuns(t *testing.T) {
	src := `
function notSame(a: unknown, b: unknown): void { if (a === b) throw new Error("same"); }
var values = [];
for (var i = 0; i < 2; ++i) { values[i] = /(?:)/; }
notSame(values[0], values[1]);
console.log("ok");
`
	got := runProgramGo(t, src)
	if strings.TrimSpace(got) != "ok" {
		t.Fatalf("RegExp[] element identity ran wrong: got %q, want %q", got, "ok")
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

// TestRegexpBoxMethodsRun builds and runs a dynamic re.test and re.exec off a boxed
// regexp: the match boolean, the capture-bearing exec result, and a global regexp's
// lastIndex advancing across calls all come off the one live regexp.
func TestRegexpBoxMethodsRun(t *testing.T) {
	src := `
const r: any = /a(b+)/;
console.log(r.test("zabbbc"));
const res = r.exec("zabbbc");
console.log(res[0]);
console.log(res[1]);
const g: any = /a/g;
console.log(g.test("aaa"));
console.log(g.lastIndex);
console.log(g.test("aaa"));
console.log(g.lastIndex);
`
	got := runProgramGo(t, src)
	want := "true\nabbb\nbbb\ntrue\n1\ntrue\n2\n"
	if got != want {
		t.Fatalf("boxed regexp methods ran wrong\n got: %q\nwant: %q", got, want)
	}
}
