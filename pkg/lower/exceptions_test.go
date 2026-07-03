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

// TestThrowNonErrorHandsBack proves throwing a value that is not a built-in error
// hands back rather than emit an unsound panic, since the runtime can only recover
// a value.Error today; boxing an arbitrary thrown value is a later slice.
func TestThrowNonErrorHandsBack(t *testing.T) {
	prog := compile(t, "throw \"boom\";\n")
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatal("throwing a string lowered, want a hand-back until arbitrary throws are boxed")
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
