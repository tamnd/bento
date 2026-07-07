package lower

import (
	"strings"
	"testing"
)

// TestDynamicStrictEqualsLowers pins that === over two dynamic operands routes
// through value.StrictEquals, the runtime compare that knows the kinds, and that
// !== is the same call negated.
func TestDynamicStrictEqualsLowers(t *testing.T) {
	src := "function f(a: any, b: any): boolean { return a === b; }\nconsole.log(f(1, 1));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.StrictEquals(a, b)") {
		t.Errorf("dynamic === did not lower to value.StrictEquals:\n%s", source)
	}
}

// TestDynamicStrictNotEqualsNegates pins the NaN self-test shape assert.js leans
// on: a !== a is the StrictEquals call under a not.
func TestDynamicStrictNotEqualsNegates(t *testing.T) {
	src := "function f(a: any): boolean { return a !== a; }\nconsole.log(f(1));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "!value.StrictEquals(a, a)") {
		t.Errorf("dynamic !== did not lower to a negated value.StrictEquals:\n%s", source)
	}
}

// TestDynamicUndefinedComparePresence pins that === undefined on a dynamic
// operand is the box's own presence test, not a StrictEquals against a built
// undefined: m === undefined reads m.IsUndefined() and m !== undefined negates it.
func TestDynamicUndefinedComparePresence(t *testing.T) {
	src := "function f(m: any): boolean { return m === undefined; }\nfunction g(m: any): boolean { return m !== undefined; }\nconsole.log(f(1));\nconsole.log(g(1));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "return m.IsUndefined()") {
		t.Errorf("dynamic === undefined did not lower to IsUndefined:\n%s", source)
	}
	if !strings.Contains(source, "return !m.IsUndefined()") {
		t.Errorf("dynamic !== undefined did not lower to a negated IsUndefined:\n%s", source)
	}
}

// TestDynamicNullComparePresence pins the null sibling: m === null reads
// m.IsNull(), the one-tag test the comparison means.
func TestDynamicNullComparePresence(t *testing.T) {
	src := "function f(m: any): boolean { return m === null; }\nconsole.log(f(1));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "return m.IsNull()") {
		t.Errorf("dynamic === null did not lower to IsNull:\n%s", source)
	}
}

// TestDynamicArithmeticCoerces pins the multiplicative operators over a dynamic
// operand: each non-number side coerces through value.ToNumber and the operator
// stays the native float64 form, so 1 / a keeps the division that distinguishes
// the signed zeros, and % takes math.Mod like every float remainder.
func TestDynamicArithmeticCoerces(t *testing.T) {
	src := "function f(a: any, b: any): number { return 1 / a; }\nfunction g(a: any, b: any): number { return a * b; }\nfunction h(a: any, b: any): number { return a % b; }\nconsole.log(f(2, 0));\nconsole.log(g(2, 3));\nconsole.log(h(7, 3));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "1 / value.ToNumber(a)") && !strings.Contains(source, "1/value.ToNumber(a)") {
		t.Errorf("dynamic division did not coerce the operand through ToNumber:\n%s", source)
	}
	if !strings.Contains(source, "value.ToNumber(a) * value.ToNumber(b)") &&
		!strings.Contains(source, "value.ToNumber(a)*value.ToNumber(b)") {
		t.Errorf("dynamic multiply did not stay a native float64 operator:\n%s", source)
	}
	if !strings.Contains(source, "math.Mod(value.ToNumber(a), value.ToNumber(b))") {
		t.Errorf("dynamic remainder did not lower to math.Mod:\n%s", source)
	}
}

// TestDynamicRelationalLowers pins that the four relational operators over a
// dynamic operand each route to their value-model helper, the readable spelling of
// the Abstract Relational Comparison the operand kinds decide at runtime.
func TestDynamicRelationalLowers(t *testing.T) {
	src := "function lt(a: any, b: any): boolean { return a < b; }\n" +
		"function le(a: any, b: any): boolean { return a <= b; }\n" +
		"function gt(a: any, b: any): boolean { return a > b; }\n" +
		"function ge(a: any, b: any): boolean { return a >= b; }\n" +
		"console.log(lt(1, 2));\nconsole.log(le(2, 2));\nconsole.log(gt(3, 2));\nconsole.log(ge(2, 2));\n"
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.Less(a, b)",
		"value.LessEqual(a, b)",
		"value.Greater(a, b)",
		"value.GreaterEqual(a, b)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("dynamic relational did not lower to %s:\n%s", want, source)
		}
	}
}

// TestDynamicLooseEqualsLowers pins that == over a dynamic operand routes through
// value.LooseEquals, the coercing sibling of StrictEquals, and that != is the
// same call negated.
func TestDynamicLooseEqualsLowers(t *testing.T) {
	src := "function f(a: any, b: any): boolean { return a == b; }\nfunction g(a: any, b: any): boolean { return a != b; }\nconsole.log(f(1, 1));\nconsole.log(g(1, 2));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.LooseEquals(a, b)") {
		t.Errorf("dynamic == did not lower to value.LooseEquals:\n%s", source)
	}
	if !strings.Contains(source, "!value.LooseEquals(a, b)") {
		t.Errorf("dynamic != did not lower to a negated value.LooseEquals:\n%s", source)
	}
}

// TestTypeofDynamicCompareLowers pins typeof x !== "object" over a dynamic x: the
// typeof side reads the runtime tag through TypeOf and the compare is the same
// BStr.Equal every string equality takes, negated for !==.
func TestTypeofDynamicCompareLowers(t *testing.T) {
	src := "function f(x: any): boolean { return typeof x !== 'object'; }\nconsole.log(f(1));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "!x.TypeOf().Equal(value.FromGoString(\"object\"))") {
		t.Errorf("typeof compare over a dynamic operand did not lower to TypeOf().Equal:\n%s", source)
	}
}

// TestDynamicTruthinessLowers pins a dynamic operand in boolean position: the
// whole falsy set is one value.ToBoolean call, and ! wraps it.
func TestDynamicTruthinessLowers(t *testing.T) {
	src := "function f(x: any): boolean { return !x; }\nfunction g(x: any): string { if (x) { return \"t\"; } return \"f\"; }\nconsole.log(f(0));\nconsole.log(g(1));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "!value.ToBoolean(x)") {
		t.Errorf("! on a dynamic operand did not lower through ToBoolean:\n%s", source)
	}
	if !strings.Contains(source, "if value.ToBoolean(x) {") {
		t.Errorf("a dynamic if condition did not lower through ToBoolean:\n%s", source)
	}
}

// TestPackageNameParamRenames pins the shadow guard: a parameter named value,
// which test262's harness uses everywhere, takes the trailing underscore so the
// emitted qualifiers into the value package still resolve.
func TestPackageNameParamRenames(t *testing.T) {
	src := "function f(value: any): boolean { return !value; }\nconsole.log(f(0));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func F(value_ value.Value) bool {") {
		t.Errorf("a parameter named value did not rename away from the package:\n%s", source)
	}
	if !strings.Contains(source, "!value.ToBoolean(value_)") {
		t.Errorf("the renamed parameter did not carry into its reads:\n%s", source)
	}
}

// TestNaNAndInfinityGlobalsLower pins the ambient number globals: NaN reads
// math.NaN(), Infinity reads math.Inf(1), and the negated form rides the same
// unary minus every number takes.
func TestNaNAndInfinityGlobalsLower(t *testing.T) {
	src := "let a: number = NaN;\nlet b: number = Infinity;\nlet c: number = -Infinity;\nconsole.log(a);\nconsole.log(b);\nconsole.log(c);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "math.NaN()") {
		t.Errorf("NaN did not lower to math.NaN():\n%s", source)
	}
	if !strings.Contains(source, "math.Inf(1)") {
		t.Errorf("Infinity did not lower to math.Inf(1):\n%s", source)
	}
}

// TestNegativeZeroLiteralKeepsSign pins the signed zero: a Go constant folds -0
// to +0, so the literal lowers through math.Copysign(0, -1), the double the
// source names.
func TestNegativeZeroLiteralKeepsSign(t *testing.T) {
	src := "function f(a: any): boolean { return a === 0 && 1 / a === -Infinity; }\nconsole.log(f(-0));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "math.Copysign(0, -1)") {
		t.Errorf("the -0 literal did not lower through math.Copysign:\n%s", source)
	}
}

// TestDynamicStrictEqualsRuns builds and runs the SameValue kernel assert.js
// carries and matches the JavaScript answers: the signed zeros differ, NaN
// matches itself, equal strings match, and a number never matches its string
// spelling.
func TestDynamicStrictEqualsRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function isSameValue(a: any, b: any): boolean {
  if (a === b) {
    return a !== 0 || 1 / a === 1 / b;
  }
  return a !== a && b !== b;
}
console.log(isSameValue(0, -0));
console.log(isSameValue(NaN, NaN));
console.log(isSameValue("a", "a"));
console.log(isSameValue(1, "1"));
console.log(isSameValue(2, 2));
`
	got := runProgramGo(t, src)
	want := "false\ntrue\ntrue\nfalse\ntrue\n"
	if got != want {
		t.Fatalf("SameValue program printed %q, want %q", got, want)
	}
}
