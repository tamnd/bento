package lower

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestStringFromCodePointEmits pins that String.fromCodePoint lowers to the
// variadic value.FromCodePoint with each number argument passed through, the
// same shape fromCharCode takes to value.FromCharCode. An astral argument is not
// special at the call site: the surrogate split happens inside FromCodePoint, so
// the emitted call is the same.
func TestStringFromCodePointEmits(t *testing.T) {
	const src = "function make(a: number, b: number): string { return String.fromCodePoint(a, b); }\nconsole.log(make(104, 105));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.FromCodePoint(a, b)") {
		t.Errorf("fromCodePoint did not lower to value.FromCodePoint(a, b):\n%s", source)
	}
}

// TestStringFromCodePointHandsBack pins that a non-number argument hands back
// with a named reason, since fromCodePoint takes code points as numbers and a
// dynamic argument would need the ToNumber coercion the string statics do not run
// here. An any-typed value assigns to the number parameter so the call still
// type-checks, but it is not statically a number, so the guard trips.
func TestStringFromCodePointHandsBack(t *testing.T) {
	const src = "function make(x: any): string { return String.fromCodePoint(x); }\nconsole.log(make(97));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "non-number argument") {
		t.Errorf("hand-back reason = %q, want it to contain %q", nyl.Reason, "non-number argument")
	}
}

// TestStringFromCodePointRuns builds and runs fromCodePoint end to end against
// the Node oracle: a BMP point maps to one code unit, an astral point above
// U+FFFF becomes a surrogate pair and prints its single character, and a mix of
// several arguments concatenates in order. The astral case is the one that
// separates fromCodePoint from fromCharCode, which would need the two surrogate
// halves spelled out by hand.
func TestStringFromCodePointRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the test builds and runs generated Go")
	}
	const src = `function bmp(): string {
  return String.fromCodePoint(104, 105);
}
function astral(): string {
  return String.fromCodePoint(0x1f600);
}
function mixed(): string {
  return String.fromCodePoint(65, 0x1f4a9, 66);
}
console.log(bmp());
console.log(astral());
console.log(mixed());
console.log(String.fromCodePoint().length);
`
	got := runProgramGo(t, src)
	want := "hi\n" +
		"\U0001f600\n" +
		"A\U0001f4a9B\n" +
		"0\n"
	if got != want {
		t.Fatalf("fromCodePoint program printed %q, want %q", got, want)
	}
}
