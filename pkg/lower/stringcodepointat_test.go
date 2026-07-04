package lower

import (
	"strings"
	"testing"
)

// TestStringCodePointAtEmits pins that s.codePointAt(i) lowers to the
// value.BStr.CodePointAtOpt method rather than handing the call back.
func TestStringCodePointAtEmits(t *testing.T) {
	const src = `const s = "abc";
const c = s.codePointAt(0);
console.log(c !== undefined ? c : -1);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "CodePointAtOpt") {
		t.Errorf("s.codePointAt(i) did not lower to CodePointAtOpt:\n%s", source)
	}
}

// TestStringCodePointAtRuns builds and runs the emitted Go against the Node
// oracle. codePointAt returns number | undefined, so the result is an Opt[float64]
// consumed through the same x !== undefined narrowing: a BMP character reads as its
// code point, the start of a surrogate pair reads as the combined astral code
// point (not the lone high surrogate charCodeAt would give), and an out-of-range
// index takes the undefined branch.
func TestStringCodePointAtRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const s = "a😀b";
const a = s.codePointAt(0);
console.log(a !== undefined ? a : -1);
const b = s.codePointAt(1);
console.log(b !== undefined ? b : -1);
const c = s.codePointAt(3);
console.log(c !== undefined ? c : -1);
const d = s.codePointAt(10);
console.log(d !== undefined ? d : -1);
`
	got := runProgramGo(t, src)
	// 'a' is 97, the emoji U+1F600 is 128512, 'b' at code-unit index 3 (past the
	// surrogate pair) is 98, and index 10 is out of range.
	const want = "97\n128512\n98\n-1\n"
	if got != want {
		t.Errorf("string codePointAt program printed %q, want %q", got, want)
	}
}
