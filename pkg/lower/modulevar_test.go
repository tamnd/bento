package lower

import (
	"strings"
	"testing"
)

// A module-level binding that a top-level function or class reads cannot stay a
// local of main, since the function is a package-level Go func and could not see
// it. Such a binding hoists to a package-level var so both the function and the
// rest of main share the one storage. A binding no function reads stays a local.

// TestModuleConstReadByFuncHoists proves a const the function divides by becomes a
// package-level var, not a main local.
func TestModuleConstReadByFuncHoists(t *testing.T) {
	const src = "const total = 100;\nfunction share(n: number): number { return total / n; }\nconsole.log(share(4));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var total float64 = 100") {
		t.Errorf("module const read by a function did not hoist to a package var:\n%s", source)
	}
	if strings.Contains(source, "total := ") {
		t.Errorf("module const hoisted but a main local was still emitted:\n%s", source)
	}
}

// TestModuleLetMutatedByFuncHoists proves a let the function reassigns becomes a
// package-level var so the mutation is visible to main afterward.
func TestModuleLetMutatedByFuncHoists(t *testing.T) {
	const src = "let count = 0;\nfunction bump(): void { count = count + 1; }\nbump();\nconsole.log(count);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var count float64 = 0") {
		t.Errorf("module let mutated by a function did not hoist to a package var:\n%s", source)
	}
}

// TestShadowingParamDoesNotHoist proves a parameter that shares a module binding's
// name is resolved by its own symbol, so the module binding is not mistaken for a
// read and stays a main local.
func TestShadowingParamDoesNotHoist(t *testing.T) {
	const src = "const s = 3;\nfunction show(s: number): number { return s * 2; }\nconsole.log(show(10));\nconsole.log(s);\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "var s float64") {
		t.Errorf("a shadowing parameter forced the module binding to hoist:\n%s", source)
	}
	if !strings.Contains(source, "s := 3") {
		t.Errorf("the module binding should have stayed a main local:\n%s", source)
	}
}

// TestModuleConstReadByFuncRuns builds and runs the emitted Go so the shared
// package var is proven to carry the const's value into the function.
func TestModuleConstReadByFuncRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const total = 100;
function share(n: number): number {
  return total / n;
}
console.log(share(4));
console.log(share(5));
`
	if got, want := runProgramGo(t, src), "25\n20\n"; got != want {
		t.Fatalf("shared module const printed %q, want %q", got, want)
	}
}

// TestModuleLetMutatedByFuncRuns builds and runs a counter that a function bumps so
// the package var is proven to hold the mutation main reads back.
func TestModuleLetMutatedByFuncRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let count = 0;
function bump(): void {
  count = count + 1;
}
bump();
bump();
bump();
console.log(count);
`
	if got, want := runProgramGo(t, src), "3\n"; got != want {
		t.Fatalf("mutated module let printed %q, want %q", got, want)
	}
}

// TestTDZConstCalledBeforeDeclHandsBack pins that a module const a function reads,
// where that function is called before the const's declaration, hands back rather than
// hoist the const to an eager package var. ES6 keeps the const in its temporal dead
// zone until its declaration runs, so the early call must throw ReferenceError; an eager
// package var is already set and would answer the value, a wrong answer. A handback keeps
// the shape safe until real TDZ enforcement lands.
func TestTDZConstCalledBeforeDeclHandsBack(t *testing.T) {
	const src = `function f() { return x + 1; }
const g = f();
const x = 1;
`
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("a const called through a function before its declaration lowered, want a hand-back:\n%s", src)
	}
	if !strings.Contains(err.Error(), "temporal dead zone") {
		t.Fatalf("handback reason = %q, want a temporal-dead-zone deferral", err.Error())
	}
}

// TestTDZConstCallNestedInClosureHandsBack pins the guard catches the call nested inside
// a closure argument, the exact shape test262 uses: assert.throws(ReferenceError, () =>
// f()). The reader's call sits inside a function expression the surrounding call invokes,
// still textually before the const's declaration, so the dead zone applies.
func TestTDZConstCallNestedInClosureHandsBack(t *testing.T) {
	const src = `function f() { return x + 1; }
(function () { f(); })();
const x = 1;
`
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("a const called through a nested closure before its declaration lowered, want a hand-back:\n%s", src)
	}
	if !strings.Contains(err.Error(), "temporal dead zone") {
		t.Fatalf("handback reason = %q, want a temporal-dead-zone deferral", err.Error())
	}
}

// TestModuleConstDeclaredBeforeCallStillHoists pins that the TDZ guard is narrow: a
// const declared before the function that reads it is called still hoists to a package
// var, since no call can observe the dead zone. This is the ordinary shape and must not
// regress to a handback.
func TestModuleConstDeclaredBeforeCallStillHoists(t *testing.T) {
	const src = "const total = 100;\nfunction share(n: number): number { return total / n; }\nconsole.log(share(4));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var total float64 = 100") {
		t.Errorf("a const declared before its reader's call did not hoist:\n%s", source)
	}
}

// TestModuleStringConstReadByFuncRuns proves a string binding hoists and carries
// its value across the function boundary, not just a number.
func TestModuleStringConstReadByFuncRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const label = "hi";
function greet(name: string): string {
  return label + ", " + name;
}
console.log(greet("sam"));
`
	if got, want := runProgramGo(t, src), "hi, sam\n"; got != want {
		t.Fatalf("shared module string printed %q, want %q", got, want)
	}
}
