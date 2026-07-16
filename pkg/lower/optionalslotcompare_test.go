package lower

import (
	"strings"
	"testing"
)

// TestOptionalSlotCompareAfterWriteLowers pins that a presence test on an optional
// local the checker narrowed away from the optional union, the shape a write leaves
// (`n = 42; n === undefined`), lowers to the raw slot's IsUndefined rather than
// handing back on mixed operands: the Go slot stays value.Opt[T] and holds Some, so
// the test is a valid n.IsUndefined() that answers false.
func TestOptionalSlotCompareAfterWriteLowers(t *testing.T) {
	src := `function f(): void {
  let n: number | undefined;
  n = 42;
  if (n === undefined) { console.log("u"); } else { console.log("d"); }
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "n.IsUndefined()") {
		t.Fatalf("write-then-compare did not lower to the slot presence test:\n%s", out)
	}
	if !strings.Contains(out, "n = value.Some[float64](42)") {
		t.Fatalf("write into the optional slot did not wrap in Some:\n%s", out)
	}
}

// TestOptionalSlotCompareParamAfterWriteLowers proves the same rewrite fires for a
// narrowed optional parameter, whose field is also a value.Opt[T] slot, so a write
// followed by a presence test reads the raw field's IsUndefined.
func TestOptionalSlotCompareParamAfterWriteLowers(t *testing.T) {
	src := `function f(p: string | undefined): void {
  p = "x";
  if (p !== undefined) { console.log("def"); }
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "!p.IsUndefined()") {
		t.Fatalf("narrowed optional parameter write-then-compare did not lower to the slot presence test:\n%s", out)
	}
}

// TestOptionalSlotCompareRuns builds and runs the write-then-compare shape so the
// slot presence test is proven to answer the way JavaScript does: a written optional
// is never undefined, and a fresh one still is.
func TestOptionalSlotCompareRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function run(): void {
  let n: number | undefined;
  n = 42;
  if (n === undefined) { console.log("still-undef"); } else { console.log("has-value"); }
  let s: string | undefined;
  if (s === undefined) { console.log("s-fresh-undef"); }
}
function param(p: string | undefined): void {
  p = "x";
  if (p !== undefined) { console.log("p-def"); }
}
run();
param(undefined);
`
	got := runProgramGo(t, src)
	want := "has-value\ns-fresh-undef\np-def\n"
	if got != want {
		t.Fatalf("write-then-compare run mismatch:\n got %q\nwant %q", got, want)
	}
}
