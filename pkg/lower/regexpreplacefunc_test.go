package lower

import (
	"strings"
	"testing"
)

// TestRegexpReplaceFuncLowers pins that a str.replace with a regexp and a
// single-parameter string-to-string function replacement lowers to the runtime
// ReplaceFuncStr method, the slot that calls the closure with each matched
// substring.
func TestRegexpReplaceFuncLowers(t *testing.T) {
	src := `
const out = "xAxAx".replace(/A/, function(m: string): string { return "[" + m + "]"; });
console.log(out);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ReplaceFuncStr(") {
		t.Fatalf("expected a ReplaceFuncStr call, got:\n%s", out)
	}
	if !strings.Contains(out, "func(m value.BStr) value.BStr") {
		t.Fatalf("expected a func(BStr) BStr closure, got:\n%s", out)
	}
}

// TestRegexpReplaceAllFuncLowers pins that the replaceAll spelling selects the
// ReplaceAllFuncStr method, the global-required variant of the same function
// replacement.
func TestRegexpReplaceAllFuncLowers(t *testing.T) {
	src := `
const out = "xAxAx".replaceAll(/A/g, (m: string): string => "[" + m + "]");
console.log(out);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ReplaceAllFuncStr(") {
		t.Fatalf("expected a ReplaceAllFuncStr call for replaceAll, got:\n%s", out)
	}
}

// TestRegexpReplaceFuncMultiParamHandsBack pins that a replacer that reads more
// than the matched substring (a second offset or capture parameter) hands back,
// since the runtime slot passes only the match text.
func TestRegexpReplaceFuncMultiParamHandsBack(t *testing.T) {
	src := `
const out = "aXbXc".replace(/X/g, function(m: string, offset: number): string { return String(offset); });
console.log(out);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "more than the matched substring") {
		t.Fatalf("expected the matched-substring handback reason, got: %q", reason)
	}
}

// TestRegexpReplaceFuncStringArgStillLowers pins that a string replacement still
// takes the string path: the function detector reports not-a-function, so the
// existing ReplaceStr template lowering runs unchanged.
func TestRegexpReplaceFuncStringArgStillLowers(t *testing.T) {
	src := `
const out = "aXbXc".replace(/[XY]/g, "-");
console.log(out);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ReplaceStr(") {
		t.Fatalf("expected the string ReplaceStr path, got:\n%s", out)
	}
}

// TestRegexpReplaceFuncRuns builds and runs the emitted Go against the Node
// oracle: a non-global regexp replaces the first match through the closure, a
// global one every match, and the closure sees each match's text.
func TestRegexpReplaceFuncRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function bracketFirst(s: string): string { return s.replace(/A/, function(m: string): string { return "[" + m + "]"; }); }
function bracketAll(s: string): string { return s.replace(/A/g, (m: string): string => "[" + m + "]"); }
function upperWords(s: string): string { return s.replace(/\w+/g, (w: string): string => w.toUpperCase()); }
console.log(bracketFirst("xAxAx"));
console.log(bracketAll("xAxAx"));
console.log(upperWords("hello world"));
`
	got := runProgramGo(t, src)
	want := "x[A]xAx\nx[A]x[A]x\nHELLO WORLD\n"
	if got != want {
		t.Fatalf("regexp replace-with-function mismatch:\n got %q\nwant %q", got, want)
	}
}
