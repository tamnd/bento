package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// compileSource writes src to a temp .ts entry and runs the front half through
// Compile, which type-checks and lowers but stops before the toolchain. It
// returns the generated Go and any front-door error, so a test can assert an
// untyped form lowers rather than being refused without paying for a real build.
func compileSource(t *testing.T, src string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	entry := filepath.Join(dir, "entry.ts")
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	return Compile(entry)
}

// TestImplicitAnyParameterLowers pins that an untyped parameter no longer gates
// the build. The checker reports "Parameter 'x' implicitly has an 'any' type"
// under strict mode, but the resolved type is already `any`, so the front door
// tolerates the report and lowers the body through the dynamic value path.
func TestImplicitAnyParameterLowers(t *testing.T) {
	src := "function f(x) { return x + 1; }\nconsole.log(f(2));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("untyped parameter should lower, got: %v", err)
	}
	if !strings.Contains(out, "func F(") {
		t.Fatalf("expected the untyped function to lower, got:\n%s", out)
	}
}

// TestImplicitAnyVariableLowers pins the same tolerance for a variable with no
// annotation and no initializer, whose type the checker cannot determine
// ("Variable 'v' implicitly has type 'any' in some locations ...").
func TestImplicitAnyVariableLowers(t *testing.T) {
	src := "let v;\nv = 10;\nconsole.log(v + 5);\n"
	if _, err := compileSource(t, src); err != nil {
		t.Fatalf("untyped variable should lower, got: %v", err)
	}
}

// TestImplicitAnyMemberLowers pins tolerance for a class member with no
// annotation, both a field and a getter, which strict mode reports as
// implicitly-any members but which lower through the class path.
func TestImplicitAnyMemberLowers(t *testing.T) {
	src := "class C {\n  x = 1;\n  get y() { return this.x; }\n}\nconsole.log(new C().y);\n"
	if _, err := compileSource(t, src); err != nil {
		t.Fatalf("untyped class member should lower, got: %v", err)
	}
}

// TestUntypedDestructuredParamLowers pins that an untyped destructured object
// parameter now lowers rather than gating. The checker reports "Binding element
// 'a' implicitly has an 'any' type" for each element, but the front door tolerates
// it and the lowerer gives the parameter one boxed value.Value slot whose shorthand
// names read out through the dynamic Get protocol.
func TestUntypedDestructuredParamLowers(t *testing.T) {
	src := "function g({ a, b }) { return a; }\nconsole.log(String(g({ a: 3, b: 4 })));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("untyped destructured parameter should lower, got: %v", err)
	}
	if !strings.Contains(out, "func G(__0 value.Value)") {
		t.Fatalf("expected the destructured parameter to take a dynamic slot, got:\n%s", out)
	}
	if !strings.Contains(out, `__0.Get(value.FromGoString("a"))`) {
		t.Fatalf("expected the bound name to read through the dynamic Get protocol, got:\n%s", out)
	}
}

// TestUntypedArrayDestructuredParamStillGates pins the honest boundary: an untyped
// array destructuring parameter reads through the dynamic index protocol, a later
// slice, so it hands back at the front door rather than emitting broken Go.
func TestUntypedArrayDestructuredParamStillGates(t *testing.T) {
	src := "function g([a, b]) { return a; }\nconsole.log(String(g([3, 4])));\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("untyped array destructured parameter should still gate at the front door")
	}
}

// TestGenuineTypeErrorStillGates pins that tolerating implicit-any did not open
// the gate to real type errors: an outright not-assignable assignment still
// fails the build, so only the untyped-form family is admitted.
func TestGenuineTypeErrorStillGates(t *testing.T) {
	src := "let n: number = \"x\";\nconsole.log(n);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("a genuine type error should still gate the build")
	}
	if !strings.Contains(err.Error(), "not assignable") {
		t.Fatalf("expected a not-assignable error, got: %v", err)
	}
}
