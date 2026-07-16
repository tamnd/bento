package lower

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This file covers the exception lowering: constructing a built-in error,
// throwing it, and reporting a throw that escapes. The unit cases pin the emitted
// shape without a toolchain (the throw call, the deferred reporter, the hand-back
// on an unsupported operand), and the end-to-end case compiles and runs a throwing
// program to prove the panic, the recover, the uncaught-error line, and the
// non-zero exit are the ones the Go toolchain actually produces.

// TestThrowEmitsPanicAndReporter pins that a throw lowers to a value.Throw of the
// constructed error and that the program defers value.ReportUncaught, the two
// halves of the raise-and-report model section 7.7 fixes.
func TestThrowEmitsPanicAndReporter(t *testing.T) {
	const src = `throw new Error("boom");
`
	source := renderProgram(t, src)
	if !strings.Contains(source, `value.Throw(value.NewError(value.FromGoString("boom")))`) {
		t.Errorf("throw did not lower to a value.Throw of the error:\n%s", source)
	}
	if !strings.Contains(source, "defer value.ReportUncaught()") {
		t.Errorf("a throwing program did not defer the uncaught-error reporter:\n%s", source)
	}
}

// TestErrorConstructorsLowerByName proves each built-in error constructor lowers
// to its value constructor, so a TypeError throws a TypeError and a RangeError a
// RangeError, and a zero-argument new Error carries an empty message.
func TestErrorConstructorsLowerByName(t *testing.T) {
	cases := map[string]string{
		`throw new TypeError("t");`:  "value.NewTypeError(value.FromGoString(\"t\"))",
		`throw new RangeError("r");`: "value.NewRangeError(value.FromGoString(\"r\"))",
		`throw new Error();`:         "value.NewError(value.FromGoString(\"\"))",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestNoThrowKeepsMainClean proves a program that cannot throw defers nothing, so
// the reporter and its import ride only on programs that need them and every
// existing golden is untouched.
func TestNoThrowKeepsMainClean(t *testing.T) {
	source := renderProgram(t, "console.log(\"hi\");\n")
	if strings.Contains(source, "ReportUncaught") {
		t.Errorf("a non-throwing program deferred the uncaught-error reporter:\n%s", source)
	}
}

// TestThrowStringLowers proves a thrown string wraps in the runtime's
// ThrownString, so it rides the panic path with the string as the reported
// name; test262's $DONOTEVALUATE throws this shape.
func TestThrowStringLowers(t *testing.T) {
	source := renderProgram(t, "throw \"boom\";\n")
	if !strings.Contains(source, "value.Throw(value.ThrownString(") {
		t.Errorf("thrown string did not wrap in value.ThrownString:\n%s", source)
	}
}

// TestThrowNonErrorValueLowers proves throwing a value that is neither a built-in
// error nor a string boxes into the runtime's ThrownValue carrier, so a bare number
// rides the panic path with the value behind it rather than handing back.
func TestThrowNonErrorValueLowers(t *testing.T) {
	source := renderProgram(t, "throw 42;\n")
	if !strings.Contains(source, "value.Throw(value.NewThrownValue(") {
		t.Errorf("thrown number did not box into value.NewThrownValue:\n%s", source)
	}
}

// handsBack renders a snippet and fails unless the lowering hands it back, the
// contract for a construct that is outside the covered subset and must fall
// through to the engine rather than emit unsound Go.
func handsBack(t *testing.T, src string) {
	t.Helper()
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatalf("snippet lowered, want a hand-back:\n%s", src)
	}
}

// TestTryCatchLowersToRecoverClosure pins the catch shape: the try body runs in an
// immediately invoked closure whose deferred recover converts the panic with
// value.Caught and runs the catch body, the panic/recover encoding of try/catch.
func TestTryCatchLowersToRecoverClosure(t *testing.T) {
	const src = `try { throw new Error("boom"); } catch (e) { if (e instanceof Error) { console.log(e.message); } }
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"defer func() {",
		"if rec := recover(); rec != nil {",
		"e := value.Caught(rec)",
		`e.IsA("Error")`,
		"e.Message()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("try/catch lowering missing %q:\n%s", want, source)
		}
	}
}

// TestTryFinallyLowersToDeferredClosure proves a try/finally with no catch lowers
// to a closure whose deferred function is the finally body and which never
// recovers, so a throw inside the try still propagates after the finally runs.
func TestTryFinallyLowersToDeferredClosure(t *testing.T) {
	const src = `try { console.log("body"); } finally { console.log("fin"); }
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "defer func() {") {
		t.Errorf("try/finally did not defer the finally body:\n%s", source)
	}
	if strings.Contains(source, "recover()") {
		t.Errorf("a try with no catch recovered a panic it should let propagate:\n%s", source)
	}
}

// TestTryCatchFinallyRunsFinallyLast proves the two handlers are deferred so the
// catch runs before the finally: the finally defer is emitted first, so under Go's
// last-in-first-out defer order it runs after the catch and after a normal
// completion, matching the language.
func TestTryCatchFinallyRunsFinallyLast(t *testing.T) {
	const src = `try { throw new TypeError("x"); } catch (e) { if (e instanceof TypeError) { console.log("caught"); } } finally { console.log("fin"); }
`
	source := renderProgram(t, src)
	fin := strings.Index(source, `value.FromGoString("fin")`)
	recover := strings.Index(source, "recover()")
	if fin < 0 || recover < 0 {
		t.Fatalf("expected both a finally body and a recover:\n%s", source)
	}
	if fin > recover {
		t.Errorf("finally defer was emitted after the catch defer, so it would run first, not last:\n%s", source)
	}
}

// TestCatchBindingNarrowsToReadNameAndMessage proves a caught error narrowed with
// instanceof reads its .name and .message as the value.Error methods, the property
// reads a catch does after narrowing the unknown binding.
func TestCatchBindingNarrowsToReadNameAndMessage(t *testing.T) {
	const src = `try { throw new RangeError("r"); } catch (e) { if (e instanceof RangeError) { console.log(e.name); console.log(e.message); } }
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "e.Name()") {
		t.Errorf("narrowed catch did not read .name as e.Name():\n%s", source)
	}
	if !strings.Contains(source, "e.Message()") {
		t.Errorf("narrowed catch did not read .message as e.Message():\n%s", source)
	}
}

// TestEmptyCatchStillRecovers proves a catch whose binding is unused still recovers
// and converts the payload, so a Go runtime panic re-raises through value.Caught
// rather than being silently swallowed, while the caught error is discarded.
func TestEmptyCatchStillRecovers(t *testing.T) {
	const src = `try { throw new Error("boom"); } catch (e) { console.log("handled"); }
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Caught(rec)") {
		t.Errorf("an unused catch binding skipped the payload conversion:\n%s", source)
	}
}

// TestCatchBindingReassignedHandsBack proves that reassigning the catch binding to
// another value hands back rather than storing into the *value.Error the recover
// bound. block-scope/shadowing/catch-parameter-shadowing-let-declaration reassigns
// the binding with a = 4, which would emit `a = value.Number(4)` into an *value.Error
// local and fail to build.
func TestCatchBindingReassignedHandsBack(t *testing.T) {
	const src = "try { throw \"x\"; } catch (a) { a = 4; console.log(String(a)); }\n"
	handsBack(t, src)
}

// TestCatchBindingShadowedHandsBack proves that shadowing the catch binding with a
// nested declaration of the same name hands back. block-scope/shadowing/
// let-declaration-shadowing-catch-parameter declares `let a = 3` inside the catch,
// and the body would read the inner number through the error's methods, which does
// not build.
func TestCatchBindingShadowedHandsBack(t *testing.T) {
	const src = "try { throw \"x\"; } catch (a) { { let a = 3; console.log(String(a)); } }\n"
	handsBack(t, src)
}

// TestReadOnlyCatchBindingStillLowers proves the rebound-or-shadowed guard does not
// swallow a catch that only reads its binding, the common shape, so the guard is
// scoped to a store or a redeclaration and nothing else.
func TestReadOnlyCatchBindingStillLowers(t *testing.T) {
	const src = "try { throw new Error(\"boom\"); } catch (e) { if (e instanceof Error) { console.log(e.message); } }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Caught(rec)") {
		t.Fatalf("a read-only catch binding should still lower:\n%s", source)
	}
}

// TestTryEscapeEmits pins the two forms a returning try takes. A try whose
// every path returns or throws, with a catch that always returns, compiles to
// returning the closure directly with a plain named result. One that control
// can run past carries the done result, and the call site turns done back into
// the enclosing function's return.
func TestTryEscapeEmits(t *testing.T) {
	const src = `function safeInvert(x: number): number {
  try {
    if (x === 0) {
      throw new Error("div by zero");
    }
    return 1 / x;
  } catch (e) {
    return -1;
  }
}
function guard(x: number): string {
  try {
    if (x < 0) {
      throw new Error("neg");
    }
  } catch (e) {
    return "caught";
  }
  return "ok";
}
console.log(safeInvert(2));
console.log(guard(1));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"return func() (ret float64) {",
		"ret = -1",
		"return 1 / x",
		"if ret, done := func() (ret value.BStr, done bool) {",
		"ret, done = value.FromGoString(\"caught\"), true",
		"; done {",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("try escape emit did not print %q:\n%s", want, source)
		}
	}
}

// TestTryEscapeHandsBack pins the boundaries the escape form keeps: a source
// name that would collide with the closure's named results, and a returning
// try nested inside a catch body, whose handler already runs deferred.
func TestTryEscapeHandsBack(t *testing.T) {
	handsBack(t, "function f(): number { let done = false; try { if (done) { return 1; } } catch (e) { return 2; } return 0; }\nf();\n")
	handsBack(t, "function f(): number { try { return 1; } catch (e) { try { return 2; } catch (e2) { return 3; } } }\nf();\n")
}

// TestTryEscapeRuns builds and runs both escape forms end to end against the
// Node answers: the always form returns from the try and from the catch, the
// done form returns from the catch of a void-bodied try while its finally still
// runs, and a finally return overrides the value the try body computed.
func TestTryEscapeRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function safeInvert(x: number): number {
  try {
    if (x === 0) {
      throw new Error("div by zero");
    }
    return 1 / x;
  } catch (e) {
    return -1;
  }
}
function guard(x: number): string {
  try {
    if (x < 0) {
      throw new Error("neg");
    }
  } catch (e) {
    return "caught";
  } finally {
    console.log("fin");
  }
  return "ok";
}
function overridden(): number {
  try {
    return 1;
  } finally {
    return 2;
  }
}
console.log(safeInvert(4));
console.log(safeInvert(0));
console.log(guard(1));
console.log(guard(-1));
console.log(overridden());
`
	got := runProgramGo(t, src)
	want := "0.25\n" +
		"-1\n" +
		"fin\n" +
		"ok\n" +
		"fin\n" +
		"caught\n" +
		"2\n"
	if got != want {
		t.Fatalf("try escape program printed %q, want %q", got, want)
	}
}

// TestTryBranchEscapeHandsBack proves a break or continue inside a try, catch, or
// finally body that targets a loop or switch enclosing the whole try hands back.
// Those bodies lower inside a Go closure, so such a branch would compile to a
// break or continue with no loop around it, which the toolchain rejects; handing
// back keeps the emitted Go sound until the branch is lowered for real.
func TestTryBranchEscapeHandsBack(t *testing.T) {
	// Unlabeled break out of the try to the enclosing for.
	handsBack(t, "for (let i = 0; i < 3; i = i + 1) { try { break; } catch (e) {} }\n")
	// Unlabeled continue out of the try to the enclosing while.
	handsBack(t, "while (true) { try { continue; } finally { console.log(\"x\"); } }\n")
	// Labeled break whose label is declared outside the try.
	handsBack(t, "outer: for (let i = 0; i < 3; i = i + 1) { try { break outer; } catch (e) {} }\n")
	// The branch in a catch body escapes just the same.
	handsBack(t, "for (let i = 0; i < 3; i = i + 1) { try { throw new Error(\"x\"); } catch (e) { break; } }\n")
}

// TestTryContainedBranchLowers proves the guard is precise: a break or continue
// whose target loop or switch sits inside the try body stays within the closure
// and lowers normally rather than tripping the hand-back.
func TestTryContainedBranchLowers(t *testing.T) {
	// break targets the for inside the try.
	forSrc := renderProgram(t, "try { for (let i = 0; i < 3; i = i + 1) { if (i === 1) { break; } console.log(i); } } catch (e) {}\n")
	if !strings.Contains(forSrc, "break") {
		t.Errorf("a break contained by a loop inside the try was dropped:\n%s", forSrc)
	}
	// continue targets the while inside the try.
	whileSrc := renderProgram(t, "try { let i = 0; while (i < 3) { i = i + 1; if (i === 2) { continue; } console.log(i); } } catch (e) {}\n")
	if !strings.Contains(whileSrc, "continue") {
		t.Errorf("a continue contained by a loop inside the try was dropped:\n%s", whileSrc)
	}
}

// TestInstanceofOutsideCatchHandsBack proves instanceof on a value that is not a
// caught error hands back, since class-instance narrowing is a later slice and
// only a caught built-in error is covered here.
func TestInstanceofOutsideCatchHandsBack(t *testing.T) {
	handsBack(t, "class C {}\nconst c: unknown = new C();\nif (c instanceof C) { console.log(\"c\"); }\n")
}

// TestDestructuredCatchThrowValueLowers proves the two halves now meet: the throw
// side boxes an object or array literal into the ThrownValue carrier and the catch
// side destructures the caught value's boxed form, so throwing a plain object or
// array and destructuring it in the catch lowers end to end rather than deferring on
// either side. Both snippets box the thrown value, so each emits the carrier call.
func TestDestructuredCatchThrowValueLowers(t *testing.T) {
	for _, src := range []string{
		"try {\n  throw { code: 1 };\n} catch ({ code }: any) {\n  console.log(code);\n}\n",
		"try {\n  throw [1, 2];\n} catch ([a, b]: any) {\n  console.log(a + b);\n}\n",
	} {
		source := renderProgram(t, src)
		if !strings.Contains(source, "value.Throw(value.NewThrownValue(") {
			t.Errorf("catch throw-value did not box into value.NewThrownValue:\n%s", source)
		}
	}
}

// TestTryCatchFinallyRunsEndToEnd proves the whole construct end to end: a program
// that throws inside a try, catches with an instanceof narrowing, reads the
// message, and runs a finally compiles to a binary whose output is the catch line
// then the finally line, in that order, and exits zero because the throw was
// caught.
func TestTryCatchFinallyRunsEndToEnd(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the try/catch/finally test builds and runs generated Go")
	}
	const src = `try {
  throw new RangeError("out of range");
} catch (e) {
  if (e instanceof RangeError) {
    console.log("caught: " + e.message);
  }
} finally {
  console.log("cleanup");
}
`
	source := renderProgram(t, src)
	dir, err := os.MkdirTemp(repoRoot(t), "tryrun-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("a caught throw exited non-zero: %v\n--- program ---\n%s\n--- stderr ---\n%s", err, source, stderr.String())
	}
	if got := stdout.String(); got != "caught: out of range\ncleanup\n" {
		t.Errorf("try/catch/finally printed %q, want the catch line then the finally line", got)
	}
}

// TestUncaughtThrowReportsAndExits proves the raise-and-report path end to end: a
// program that throws an uncaught error compiles to a binary that prints the
// uncaught-error line to standard error and exits non-zero, the way a runtime
// reports an unhandled exception. Building and running is the oracle, since the
// whole point is that the panic the throw emits is the one Go unwinds into the
// deferred reporter.
func TestUncaughtThrowReportsAndExits(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the uncaught-throw test builds and runs generated Go")
	}
	source := renderProgram(t, "throw new RangeError(\"out of range\");\n")
	dir, err := os.MkdirTemp(repoRoot(t), "throwrun-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Fatalf("an uncaught throw exited zero:\n--- program ---\n%s", source)
	}
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("go run failed to launch: %v\n--- program ---\n%s\n--- stderr ---\n%s", err, source, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "Uncaught RangeError: out of range") {
		t.Errorf("uncaught throw printed %q to stderr, want the uncaught-error line", got)
	}
	if stdout.String() != "" {
		t.Errorf("an uncaught throw wrote %q to stdout, want nothing", stdout.String())
	}
}
