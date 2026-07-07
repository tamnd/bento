package lower

import (
	"strings"
	"testing"
)

// TestTypeofCaughtErrorFolds pins that typeof over a caught error folds to the
// "object" tag: the runtime holds every caught value as an error object, so the
// tag is known without lowering the binding.
func TestTypeofCaughtErrorFolds(t *testing.T) {
	src := "try { throw new TypeError(\"x\"); } catch (e: any) { let s: string = typeof e; console.log(s); }"
	out := renderProgram(t, src)
	if !strings.Contains(out, `value.FromGoString("object")`) {
		t.Fatalf("typeof caught error did not fold to the object tag:\n%s", out)
	}
}

// TestCaughtErrorEqualsNullFolds pins that a caught error compared to null folds
// to a Go constant, false for === and true for !==, since a caught value is a
// non-nil error and never null.
func TestCaughtErrorEqualsNullFolds(t *testing.T) {
	src := "try { throw new TypeError(\"x\"); } catch (e: any) { let a: boolean = e === null; let b: boolean = e !== null; console.log(a); console.log(b); }"
	out := renderProgram(t, src)
	if !strings.Contains(out, "a := false") {
		t.Fatalf("caught error === null did not fold to false:\n%s", out)
	}
	if !strings.Contains(out, "b := true") {
		t.Fatalf("caught error !== null did not fold to true:\n%s", out)
	}
}

// TestCaughtErrorEqualsUndefinedFolds pins the same fold against undefined, which
// a caught value also never is.
func TestCaughtErrorEqualsUndefinedFolds(t *testing.T) {
	src := "try { throw new TypeError(\"x\"); } catch (e: any) { let a: boolean = e === undefined; console.log(a); }"
	out := renderProgram(t, src)
	if !strings.Contains(out, "a := false") {
		t.Fatalf("caught error === undefined did not fold to false:\n%s", out)
	}
}

// TestCaughtErrorConstructorLowers pins that a caught error's .constructor lowers
// to the Constructor method on the bound error rather than handing back.
func TestCaughtErrorConstructorLowers(t *testing.T) {
	src := "try { throw new TypeError(\"x\"); } catch (e: any) { let c: any = e.constructor; console.log(c === TypeError); }"
	out := renderProgram(t, src)
	if !strings.Contains(out, ".Constructor()") {
		t.Fatalf("caught error .constructor did not lower to the Constructor method:\n%s", out)
	}
}

// TestCaughtErrorConstructorRuns builds and runs the assert.throws comparison: a
// caught error's constructor compares equal by identity to the matching built-in
// and answers its name, and unequal to a different constructor.
func TestCaughtErrorConstructorRuns(t *testing.T) {
	skipIfShort(t)
	src := `
try {
  throw new TypeError("boom");
} catch (thrown: any) {
  console.log(thrown.constructor === TypeError);
  console.log(thrown.constructor === RangeError);
  let cn: string = thrown.constructor.name;
  console.log(cn);
}
`
	got := runProgramGo(t, src)
	want := "true\nfalse\nTypeError\n"
	if got != want {
		t.Fatalf("caught error constructor run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestCaughtErrorGuardRuns builds and runs the assert.throws guard shape, typeof
// thrown !== 'object' || thrown === null, over a real thrown error, and checks it
// takes the else branch the way the prelude needs it to for a real error.
func TestCaughtErrorGuardRuns(t *testing.T) {
	skipIfShort(t)
	src := `
try {
  throw new TypeError("boom");
} catch (thrown: any) {
  if (typeof thrown !== "object" || thrown === null) {
    console.log("not an object");
  } else {
    console.log("object");
  }
  console.log(thrown.name);
}
`
	got := runProgramGo(t, src)
	want := "object\nTypeError\n"
	if got != want {
		t.Fatalf("caught error guard run mismatch:\n got %q\nwant %q", got, want)
	}
}
