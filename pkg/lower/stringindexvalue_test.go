package lower

import (
	"strings"
	"testing"
)

// TestStringIndexBoxedEmits pins that a bracket read s[i] boxed into a dynamic sink
// lowers to the value.BStr.StringIndexValue method, the read that yields undefined
// for an out-of-range or non-canonical index rather than the empty string CharAt
// gives the typed string slot. The any-typed sink forces the box: the checker types
// s[i] as string, so only a genuinely dynamic slot (an any variable, a dynamic call
// argument like assert.sameValue's) can observe the undefined an exotic read carries.
func TestStringIndexBoxedEmits(t *testing.T) {
	const src = `const s = "hello world";
const x: any = s[100];
console.log(x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "StringIndexValue") {
		t.Errorf("boxed s[i] did not lower to StringIndexValue:\n%s", source)
	}
}

// TestStringIndexBoxedRuns builds and runs the emitted Go: an in-range index prints
// the one-code-unit string, and a negative, out-of-range, fractional, NaN, or
// infinite index prints undefined, matching Node's String exotic own-property read.
func TestStringIndexBoxedRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const s = "hi";
const a: any = s[0];
const b: any = s[-1];
const c: any = s[5];
const d: any = s[NaN];
const e: any = s[Infinity];
console.log(a);
console.log(b);
console.log(c);
console.log(d);
console.log(e);
`
	got := runProgramGo(t, src)
	const want = "h\nundefined\nundefined\nundefined\nundefined\n"
	if got != want {
		t.Errorf("boxed string index program printed %q, want %q", got, want)
	}
}

// TestStringIndexTypedSlotKeepsCharAt pins that a string-typed consumer of s[i] still
// reads through CharAt, so the typed path is unchanged: the checker types s[i] as
// string, and a static string slot never observes the undefined an out-of-range read
// carries.
func TestStringIndexTypedSlotKeepsCharAt(t *testing.T) {
	const src = `const s = "hi";
const c: string = s[0];
console.log(c);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "CharAt") {
		t.Errorf("typed string-slot s[i] did not keep CharAt:\n%s", source)
	}
}
