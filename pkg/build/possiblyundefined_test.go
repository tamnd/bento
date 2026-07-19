package build

import (
	"strings"
	"testing"
)

// TestPossiblyUndefinedMemberReachesRenderer pins that a member read off a value the
// checker cannot prove present no longer gates the build at the front door. Calling a
// method on a Map.get result draws 18048 ("'X' is possibly 'undefined'"), a strictness
// artifact over JavaScript that returns the stored value at run time when the key is
// present, so the front door tolerates the report and the program reaches the renderer.
// The renderer lowers a method call only over a receiver whose static type carries it,
// and the un-narrowed `V | undefined` receiver carries none, so it hands back to the
// engine rather than emitting a wrong call: the tell is that the error is now a lowering
// hand-back, not the checker's possibly-undefined message.
func TestPossiblyUndefinedMemberReachesRenderer(t *testing.T) {
	src := "const m = new Map<number, number>([[1, 2]]);\nconst v = m.get(1);\nconsole.log(v.toString());\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("a method call on a possibly-undefined receiver should hand back, not lower")
	}
	if strings.Contains(err.Error(), "possibly 'undefined'") {
		t.Fatalf("the possibly-undefined read should no longer gate at the front door, got the checker report: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("expected a lowering hand-back to the engine, got: %v", err)
	}
}

// TestPossiblyUndefinedArithReachesRenderer pins the same for an arithmetic use of an
// optional property. Adding to `o.a` where a is optional draws 18048 on the operand, and
// the renderer refuses a numeric operation over an un-narrowed `number | undefined`, so
// the front-door tolerance lets it reach the engine and hand back rather than gating on
// the checker report.
func TestPossiblyUndefinedArithReachesRenderer(t *testing.T) {
	src := "interface O { a?: number }\nconst o: O = { a: 5 };\nconsole.log(o.a + 1);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("arithmetic on a possibly-undefined operand should hand back, not lower")
	}
	if strings.Contains(err.Error(), "possibly 'undefined'") {
		t.Fatalf("the possibly-undefined operand should no longer gate at the front door, got the checker report: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("expected a lowering hand-back to the engine, got: %v", err)
	}
}

// TestNarrowedOptionalStillLowers pins that admitting 18048/2532 did not disturb the
// narrowing path: an optional read the checker narrows past a guard draws no
// possibly-undefined report and still lowers through the Opt unwrap rather than handing
// back.
func TestNarrowedOptionalStillLowers(t *testing.T) {
	src := "function f(x: number | undefined): number {\n  if (x === undefined) return 0;\n  return x + 1;\n}\nconsole.log(f(5));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("a narrowed optional read should still lower, got: %v", err)
	}
	if !strings.Contains(out, "func F(") {
		t.Fatalf("expected the guarded function to lower, got:\n%s", out)
	}
}

// TestGenuineErrorStillGatesAfterPossiblyUndefinedTolerance pins that admitting the
// possibly-undefined family did not open the gate to unrelated errors: an undeclared
// name is still a hard front-door failure.
func TestGenuineErrorStillGatesAfterPossiblyUndefinedTolerance(t *testing.T) {
	src := "console.log(nope);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("an undeclared name should still gate the build")
	}
	if !strings.Contains(err.Error(), "Cannot find name") {
		t.Fatalf("expected the undeclared-name error, got: %v", err)
	}
}
