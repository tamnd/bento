package lower

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runTolerantGo renders src through the tolerant front door, the path a program
// with a string or boolean arithmetic operand takes because the strict compile
// helper would reject the 2362/2363 diagnostic, then builds and runs the emitted
// Go and returns its standard output. It mirrors runProgramGo for the tolerant
// front door.
func runTolerantGo(t *testing.T, src string) string {
	t.Helper()
	source := renderTolerant(t, src)
	return cachedGoRun(t, source, func() string {
		dir, err := os.MkdirTemp(repoRoot(t), "arithrun-")
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
			t.Fatalf("go run failed: %v\n--- program ---\n%s\n--- stderr ---\n%s", err, source, stderr.String())
		}
		return stdout.String()
	})
}

// TestStringOperandSubtracts pins that a numeric string subtracted from a number
// coerces through ToNumber and runs: "5" - 1 is 4, exactly as JavaScript
// evaluates it, even though the checker rejects the string operand with 2362.
func TestStringOperandSubtracts(t *testing.T) {
	const src = `let s = "5";
let n = 1;
console.log(s - n);
`
	got := runTolerantGo(t, src)
	if got != "4\n" {
		t.Errorf("string minus number ran wrong\n got: %q\nwant: %q", got, "4\n")
	}
}

// TestBooleanOperandMultiplies pins that a boolean arithmetic operand coerces
// through ToNumber: true is 1 and false is 0, so true * 3 is 3 and false * 3 is 0.
func TestBooleanOperandMultiplies(t *testing.T) {
	const src = `let t = true;
let f = false;
let n = 3;
console.log(t * n);
console.log(f * n);
`
	got := runTolerantGo(t, src)
	if got != "3\n0\n" {
		t.Errorf("boolean times number ran wrong\n got: %q\nwant: %q", got, "3\n0\n")
	}
}

// TestNonNumericStringIsNaN pins the coercion's edge: a string ToNumber cannot
// parse is NaN, so "x" * 2 is NaN and prints as NaN, not a parse crash.
func TestNonNumericStringIsNaN(t *testing.T) {
	const src = `let s = "x";
console.log(s * 2);
`
	got := runTolerantGo(t, src)
	if got != "NaN\n" {
		t.Errorf("non-numeric string arithmetic ran wrong\n got: %q\nwant: %q", got, "NaN\n")
	}
}

// TestStringRemainderAndExponent pins that % and ** reach the coercion too, since
// they lower to math.Mod and value.Pow rather than a plain Go operator: "7" % 3
// is 1 and "2" ** 3 is 8.
func TestStringRemainderAndExponent(t *testing.T) {
	const src = `let a = "7";
let b = "2";
console.log(a % 3);
console.log(b ** 3);
`
	got := runTolerantGo(t, src)
	if got != "1\n8\n" {
		t.Errorf("string remainder or exponent ran wrong\n got: %q\nwant: %q", got, "1\n8\n")
	}
}

// TestStringBitwiseCoerces pins that a bitwise operator coerces a string operand
// to int32 the same way a number operand does: "6" & 3 is 2 and "1" << 4 is 16.
func TestStringBitwiseCoerces(t *testing.T) {
	const src = `let a = "6";
let b = "1";
console.log(a & 3);
console.log(b << 4);
`
	got := runTolerantGo(t, src)
	if got != "2\n16\n" {
		t.Errorf("string bitwise ran wrong\n got: %q\nwant: %q", got, "2\n16\n")
	}
}

// TestStringArithLowersToStringToNumber pins the emit shape: a string operand of
// an arithmetic operator lowers through value.StringToNumber, not a bare string
// the Go operator would reject.
func TestStringArithLowersToStringToNumber(t *testing.T) {
	const src = `let s = "5";
console.log(s - 1);
`
	source := renderTolerant(t, src)
	if !strings.Contains(source, "value.StringToNumber") {
		t.Errorf("string operand was not coerced through value.StringToNumber:\n%s", source)
	}
}

// TestPureNumberArithNotCoerced pins that the coercion path does not over-fire: a
// plain two-number subtraction stays the direct Go operator and never routes
// through value.StringToNumber, so the string and boolean case is the only one
// this slice changes.
func TestPureNumberArithNotCoerced(t *testing.T) {
	const src = `let a = 5;
let b = 1;
console.log(a - b);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "value.StringToNumber") || strings.Contains(source, "value.BoolToNumber") {
		t.Errorf("two-number arithmetic was needlessly coerced:\n%s", source)
	}
}

// TestObjectOperandArithHandsBack pins the zero-fail boundary: an object operand of
// an arithmetic operator is not a number-coercible primitive, so the operator is
// not folded into the string and boolean coercion and hands back rather than
// emitting Go the operator cannot take on a struct pointer.
func TestObjectOperandArithHandsBack(t *testing.T) {
	const src = `let o = { v: 1 };
let s = "5";
console.log(s * o);
`
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("object arithmetic operand lowered, want a hand-back")
	}
}
