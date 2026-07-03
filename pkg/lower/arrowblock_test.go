package lower

import (
	"strings"
	"testing"
)

// TestArrowBlockBodyLowers pins that an arrow with a statement block for a body
// lowers to a func literal whose body is the lowered block, not a single return of
// a concise expression. The block declares a local and returns it, the shape a
// callback that needs more than one expression takes, and the result type comes
// from the arrow's own call signature so the return coerces the way a named
// function's does.
func TestArrowBlockBodyLowers(t *testing.T) {
	const src = `const nums = [1, 2, 3];
const doubled = nums.map((n: number): number => {
  const twice = n * 2;
  return twice;
});
console.log(doubled.length);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func(n float64) float64") {
		t.Errorf("block-body arrow did not lower to a typed func literal:\n%s", source)
	}
	if !strings.Contains(source, "twice") {
		t.Errorf("block-body arrow dropped its local declaration:\n%s", source)
	}
}

// TestArrowBlockBodyRuns proves the block-body arrow runs to the same output the
// concise form would, so the block is only a longer way to write the same callback.
func TestArrowBlockBodyRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const nums = [1, 2, 3, 4];
const doubled = nums.map((n: number): number => {
  const twice = n * 2;
  return twice;
});
console.log(doubled.length);
console.log(doubled[3]);
`
	got := runProgramGo(t, src)
	if want := "4\n8\n"; got != want {
		t.Errorf("block-body arrow ran to %q, want %q", got, want)
	}
}

// TestArrowBlockBodyVoidThrowsInline proves a void callback can now throw directly
// from inside its own block rather than routing the conditional through a named
// function. The callback crosses into TryEach as a Go func returning error, the
// inline throw becomes that func's non-nil error, and the (int, error) result
// hoists back to a throw the program catches. This is the ergonomics the block-body
// slice unlocks for go: callbacks.
func TestArrowBlockBodyVoidThrowsInline(t *testing.T) {
	skipIfShort(t)
	const src = `import { TryEach } from "go:github.com/tamnd/bento/pkg/goimport/funcfixture";
try {
  console.log(TryEach(4, (i: number): void => {
    if (i === 2) {
      throw new Error("stop at " + i);
    }
  }));
} catch (e) {
  if (e instanceof Error) {
    console.log("caught: " + e.message);
  }
}
console.log(TryEach(2, (i: number): void => {
  if (i === 2) {
    throw new Error("stop at " + i);
  }
}));
`
	got := runProgramGo(t, src)
	if want := "caught: Error: stop at 2\n2\n"; got != want {
		t.Errorf("inline-throwing block-body callback ran to %q, want %q", got, want)
	}
}
