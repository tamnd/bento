package lower

import (
	"strings"
	"testing"
)

// TestMathRandomLowers pins that Math.random() lowers to the value.MathRandom
// runtime call.
func TestMathRandomLowers(t *testing.T) {
	src := `
console.log(Math.random() < 1);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.MathRandom()") {
		t.Fatalf("expected a MathRandom call, got:\n%s", out)
	}
}

// TestMathRandomShapeRuns builds and runs the emitted Go against the Node oracle.
// The two runtimes draw unrelated numbers, so the program prints only derived
// facts both satisfy: every draw is in [0, 1) and the draws are not all equal.
func TestMathRandomShapeRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let allInRange = true;
let allSame = true;
const first = Math.random();
for (let i = 0; i < 1000; i++) {
  const r = Math.random();
  if (r < 0 || r >= 1) allInRange = false;
  if (r !== first) allSame = false;
}
console.log(allInRange);
console.log(allSame);
`
	got := runProgramGo(t, src)
	want := "true\nfalse\n"
	if got != want {
		t.Fatalf("Math.random shape mismatch:\n got %q\nwant %q", got, want)
	}
}
