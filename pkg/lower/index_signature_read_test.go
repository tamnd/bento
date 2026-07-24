package lower

import (
	"strings"
	"testing"
)

// TestIndexSignatureReadLowers pins that a read of a key on a pure string-index
// dictionary lowers to the runtime Get on the boxed receiver, unboxed to the
// signature's element type. The dictionary is a value.Value (isStringIndexDict),
// so a const-string-key read routes through the dynamic path rather than folding
// to a fixed-shape miss, which would land a value.Value where the signature's
// number is wanted. An undeclared key reads undefined through the same Get, which
// ToNumber turns into NaN, the JavaScript answer.
func TestIndexSignatureReadLowers(t *testing.T) {
	const src = `type Dict = { [k: string]: number };
const o: Dict = { a: 1 };
console.log(String(o["b"]));
`
	out := renderProgram(t, src)
	if strings.Contains(out, "MissingProperty") {
		t.Fatalf("index-signature read folded to a fixed-shape miss:\n%s", out)
	}
	if !strings.Contains(out, ".Get(value.FromGoString(\"b\"))") {
		t.Fatalf("index-signature read did not route through the runtime Get:\n%s", out)
	}
}

// TestIndexSignatureReadRuns builds and runs a dictionary write then read back end
// to end: a stored key reads its value, proving the Get and Set agree on the boxed
// key. Node prints 5.
func TestIndexSignatureReadRuns(t *testing.T) {
	skipIfShort(t)
	const src = `type Dict = { [k: string]: number };
const o: Dict = {};
o["a"] = 5;
console.log(String(o["a"]));
`
	got := runProgramGo(t, src)
	want := "5\n"
	if got != want {
		t.Fatalf("index-signature read run mismatch:\n got %q\nwant %q", got, want)
	}
}
