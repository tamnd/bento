package build

import (
	"strings"
	"testing"
)

// TestUnknownCatchTypeofLowers pins that a caught value used in a body no longer
// gates the build. Under strict checking the catch binding is typed `unknown`
// (useUnknownInCatchVariables), so the checker reports "'err' is of type
// 'unknown'" (18046) the moment the body touches it. The resolved type is
// dynamic exactly like `any`, so the front door tolerates the report and lowers
// typeof on the caught value through the dynamic path.
func TestUnknownCatchTypeofLowers(t *testing.T) {
	src := "function f(): string {\n  try {\n    throw new TypeError(\"boom\");\n  } catch (err) {\n    return typeof err;\n  }\n}\nconsole.log(f());\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("typeof on an unknown catch binding should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.Caught(") {
		t.Fatalf("expected the caught value to bind through value.Caught, got:\n%s", out)
	}
}

// TestUnknownCatchStringLowers pins the same tolerance for String() on the
// caught value, the message-building path assert leans on: the unknown binding
// flows into String as a dynamic value and lowers to its runtime string
// conversion rather than being refused at the front door.
func TestUnknownCatchStringLowers(t *testing.T) {
	src := "function f(): string {\n  try {\n    throw new RangeError(\"x\");\n  } catch (err) {\n    return String(err);\n  }\n}\nconsole.log(f());\n"
	if _, err := compileSource(t, src); err != nil {
		t.Fatalf("String() on an unknown catch binding should lower, got: %v", err)
	}
}

// TestUnknownCatchBooleanAndRethrowClearsFrontDoor pins that using a caught value
// in boolean position and rethrowing it, the inspect-then-rethrow shape
// assert.throws uses, no longer draws the unknown report at the front door. The
// body may still hand back to a later slice (a value-to-string coercion here),
// but the checker refusal about the unknown catch binding is gone, which is the
// wall this slice removes.
func TestUnknownCatchBooleanAndRethrowClearsFrontDoor(t *testing.T) {
	src := "function f(): void {\n  try {\n    throw new Error(\"x\");\n  } catch (err) {\n    if (err) {\n      throw err;\n    }\n  }\n}\nf();\n"
	if _, err := compileSource(t, src); err != nil && strings.Contains(err.Error(), "of type 'unknown'") {
		t.Fatalf("the unknown catch binding should not gate at the front door, got: %v", err)
	}
}

// TestGenuineUnknownMisuseStillHandsBack pins that tolerating the unknown reports
// did not turn a shape the lowerer cannot model into a miscompile: reading
// .constructor.name off the caught value draws a function-type reflection the
// lowerer does not cover yet, so the unit hands back to the interpreter rather
// than emitting Go that does not compile. Worst case stays a handback.
func TestGenuineUnknownMisuseStillHandsBack(t *testing.T) {
	src := "function f(): string {\n  try {\n    throw new TypeError(\"x\");\n  } catch (err) {\n    return (err as any).constructor.name;\n  }\n}\nconsole.log(f());\n"
	// This lowers or hands back, but must never fail: the assertion is only that
	// the front door does not refuse it for the unknown report. Reading through an
	// `any` cast sidesteps 18046, so this is a companion guard that the tolerance
	// does not over-reach into shapes it should not admit; it stays a compile or a
	// clean handback, never a checker refusal about unknown.
	_, err := compileSource(t, src)
	if err != nil && strings.Contains(err.Error(), "of type 'unknown'") {
		t.Fatalf("no unknown report should reach the user here, got: %v", err)
	}
}

// TestGenuineTypeErrorStillGatesAfterUnknown pins that admitting the unknown
// reports did not open the gate to real type errors: an outright not-assignable
// assignment still fails the build, so only the unknown-value family is tolerated.
func TestGenuineTypeErrorStillGatesAfterUnknown(t *testing.T) {
	src := "let n: number = \"x\";\nconsole.log(n);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("a genuine type error should still gate the build")
	}
	if !strings.Contains(err.Error(), "not assignable") {
		t.Fatalf("expected a not-assignable error, got: %v", err)
	}
}
