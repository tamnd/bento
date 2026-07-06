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

// TestThrowNonErrorHandsBack proves throwing a value with no Thrown lowering (a
// bare number) still hands back rather than emit an unsound panic; boxing the
// remaining primitive throws is a later slice.
func TestThrowNonErrorHandsBack(t *testing.T) {
	prog := compile(t, "throw 42;\n")
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatal("throwing a number lowered, want a hand-back until arbitrary throws are boxed")
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

// TestReturningTryHandsBack proves a return inside a try, catch, or finally body
// hands the whole statement back, because a Go return inside the emitted closure
// would leave the closure rather than the enclosing function, an abrupt completion
// this slice does not carry out of the construct.
func TestReturningTryHandsBack(t *testing.T) {
	handsBack(t, "function f(): number { try { return 1; } finally { console.log(\"fin\"); } }\nf();\n")
	handsBack(t, "function f(): number { try { console.log(\"body\"); } catch (e) { return 2; } return 0; }\nf();\n")
	handsBack(t, "function f(): number { try { console.log(\"body\"); } finally { return 3; } }\nf();\n")
}

// TestInstanceofOutsideCatchHandsBack proves instanceof on a value that is not a
// caught error hands back, since class-instance narrowing is a later slice and
// only a caught built-in error is covered here.
func TestInstanceofOutsideCatchHandsBack(t *testing.T) {
	handsBack(t, "class C {}\nconst c: unknown = new C();\nif (c instanceof C) { console.log(\"c\"); }\n")
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
