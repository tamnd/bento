package lower

import (
	"strings"
	"testing"
)

// TestStringAtEmits pins that s.at(i) lowers to the value.BStr.AtOpt method, the
// string sibling of the array at read, rather than handing the call back.
func TestStringAtEmits(t *testing.T) {
	const src = `const s = "abc";
const c = s.at(1);
console.log(c !== undefined ? c : "none");
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "AtOpt") {
		t.Errorf("s.at(i) did not lower to AtOpt:\n%s", source)
	}
}

// TestStringAtRuns builds and runs the emitted Go against the Node oracle. at
// returns string | undefined, so the result is an Opt[BStr] consumed through the
// same x !== undefined narrowing the array at read uses: an in-range index yields
// the one-character string, a negative index counts from the end, and an
// out-of-range index takes the undefined branch.
func TestStringAtRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const s = "hello";
const a = s.at(1);
console.log(a !== undefined ? a : "none");
const b = s.at(-1);
console.log(b !== undefined ? b : "none");
const c = s.at(10);
console.log(c !== undefined ? c : "none");
const d = s.at(-10);
console.log(d !== undefined ? d : "none");
`
	got := runProgramGo(t, src)
	const want = "e\no\nnone\nnone\n"
	if got != want {
		t.Errorf("string at program printed %q, want %q", got, want)
	}
}
