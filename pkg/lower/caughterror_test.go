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

// TestCaughtErrorStringifyLowers pins that a caught error read in a string context
// lowers to the ToBStr method on the bound error rather than handing back. The
// three spellings, concatenation, String(err), and a template, all take the same
// coercion the way assert.sameValue builds its failure message.
func TestCaughtErrorStringifyLowers(t *testing.T) {
	cases := map[string]string{
		"concat":   `try { throw new TypeError("x"); } catch (e: any) { let s: string = "caught " + e; console.log(s); }`,
		"String":   `try { throw new TypeError("x"); } catch (e: any) { let s: string = String(e); console.log(s); }`,
		"template": "try { throw new TypeError(\"x\"); } catch (e: any) { let s: string = `got ${e}`; console.log(s); }",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, ".ToBStr()") {
				t.Fatalf("caught error in a string context did not lower to ToBStr:\n%s", out)
			}
		})
	}
}

// TestCaughtErrorStringifyRuns builds and runs the three string forms over a real
// caught error and checks each yields Error.prototype.toString's "Name: message"
// shape, with the name alone when the message is empty.
func TestCaughtErrorStringifyRuns(t *testing.T) {
	skipIfShort(t)
	src := `
try {
  throw new TypeError("boom");
} catch (error: any) {
  console.log("caught " + error);
  console.log(String(error));
  console.log(` + "`got ${error}`" + `);
}
try {
  throw new RangeError("");
} catch (error: any) {
  console.log("caught " + error);
}
`
	got := runProgramGo(t, src)
	want := "caught TypeError: boom\nTypeError: boom\ngot TypeError: boom\ncaught RangeError\n"
	if got != want {
		t.Fatalf("caught error stringify run mismatch:\n got %q\nwant %q", got, want)
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
