package lower

import (
	"strings"
	"testing"
)

// TestHeterogeneousArrayLiteralWrapsElements pins that an array literal whose
// element type is a tagged-sum union wraps each element in the arm constructor its
// own type selects. Without the wrap a mixed literal emits value.NewArray[NumOrStr]
// over bare number and string values, which does not build because a NumOrStr is
// neither, so a person reading the output would see a Go type error rather than the
// array the source wrote.
func TestHeterogeneousArrayLiteralWrapsElements(t *testing.T) {
	src := "const a = ['x', 0, -0]; console.log(String(a.length));"
	out := renderProgram(t, src)
	for _, want := range []string{
		"NumOrStrOfStr(value.FromGoString(\"x\"))",
		"NumOrStrOfNum(0)",
		"NumOrStrOfNum(math.Copysign(0, -1))",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("mixed array element not wrapped, missing %q:\n%s", want, out)
		}
	}
}

// TestHomogeneousArrayLiteralIsUnchanged pins that the wrap stays off an array
// whose element type is a single primitive: a number array lowers to bare element
// values with no union constructor, the same output it had before the wrap existed.
func TestHomogeneousArrayLiteralIsUnchanged(t *testing.T) {
	src := "const a = [1, 2, 3]; console.log(String(a.length));"
	out := renderProgram(t, src)
	if strings.Contains(out, "Of") && strings.Contains(out, "Tag") {
		t.Fatalf("homogeneous number array grew a union wrap:\n%s", out)
	}
}

// TestHeterogeneousArrayLiteralRuns builds and runs a few mixed-primitive array
// literals so the wrapped construction compiles and each array reports its length,
// the observable the JavaScript source prints.
func TestHeterogeneousArrayLiteralRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const a = ['x', 0, -0];
console.log(String(a.length));
const b: (string | number)[] = ['a', 1, 'b', 2];
console.log(String(b.length));
const c = [true, 0, 'z'];
console.log(String(c.length));
`
	got := runProgramGo(t, src)
	want := "3\n4\n3\n"
	if got != want {
		t.Fatalf("mixed array run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestHeterogeneousArrayJSONStringifyRuns builds and runs JSON.stringify over mixed
// arrays so the generated union emits its JSONArm hook and the serializer renders
// each element by its arm: a string, a number with negative zero folded to 0, and a
// boolean, the exact text V8 produces.
func TestHeterogeneousArrayJSONStringifyRuns(t *testing.T) {
	skipIfShort(t)
	src := `
console.log(JSON.stringify(['-0', 0, -0]));
console.log(JSON.stringify([1, 'a', true]));
`
	got := runProgramGo(t, src)
	want := "[\"-0\",0,0]\n[1,\"a\",true]\n"
	if got != want {
		t.Fatalf("mixed array JSON run mismatch:\n got %q\nwant %q", got, want)
	}
}
