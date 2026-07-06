package lower

import (
	"strings"
	"testing"
)

// TestStringIndexEmits pins that s[i] on a string receiver lowers to the
// value.BStr.CharAt code-unit read, the bracket spelling of charAt, rather than
// handing back on the non-array-receiver path.
func TestStringIndexEmits(t *testing.T) {
	const src = `const s = "abc";
const i = 1;
console.log(s[i + 1]);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "CharAt") {
		t.Errorf("s[i] did not lower to CharAt:\n%s", source)
	}
}

// TestStringIndexLoopEmits pins that a proven-integer loop counter indexes the
// string through CharAtI, the same native-int fast path the array AtI read
// takes, so the counter stays an int and the float truncation is dropped. The
// bound is a literal because the int32 counter proof wants a literal or const
// bound; an s.length bound keeps the counter a float and the CharAt read.
func TestStringIndexLoopEmits(t *testing.T) {
	const src = `const s = "abc";
for (let i = 0; i < 3; i++) {
  console.log(s[i]);
}
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "CharAtI") {
		t.Errorf("loop-indexed s[i] did not lower to CharAtI:\n%s", source)
	}
}

// TestStringIndexRuns builds and runs the emitted Go against Node's output for
// in-range reads: a direct index, a computed index, and a counter loop over a
// string with a two-byte character, so the code-unit read is exercised on a
// non-ASCII backing too.
func TestStringIndexRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const s = "héllo";
console.log(s[0]);
console.log(s[1]);
const base = 1;
console.log(s[base + 2]);
let out = "";
for (let i = 0; i < s.length; i++) {
  out += s[i];
}
console.log(out);
`
	got := runProgramGo(t, src)
	const want = "h\né\nl\nhéllo\n"
	if got != want {
		t.Errorf("string index program printed %q, want %q", got, want)
	}
}
