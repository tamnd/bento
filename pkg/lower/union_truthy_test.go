package lower

import (
	"strings"
	"testing"
)

// TestUnionTruthyEmitsToBoolean pins that a tagged-sum union in boolean position
// lowers to a ToBoolean method call and the union grows that method, each value arm
// switching on the tag to its inline JavaScript truthiness while the sentinel arm
// falls to the trailing false.
func TestUnionTruthyEmitsToBoolean(t *testing.T) {
	const src = `function f(x: number | string | undefined): string {
  if (x) {
    return "t";
  }
  return "f";
}
f(1);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"func (u NumOrStrOrUndef) ToBoolean() bool {",
		"case NumOrStrOrUndefNum:",
		"return u.num != 0 && u.num == u.num",
		"case NumOrStrOrUndefStr:",
		"return u.str.Length() > 0",
		"if x.ToBoolean() {",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("emitted Go missing %q\n%s", want, source)
		}
	}
	// The undefined arm carries no field, so it emits no case in ToBoolean and rides the
	// trailing false, the truth every sentinel has.
	toBool := source[strings.Index(source, "func (u NumOrStrOrUndef) ToBoolean()"):]
	if strings.Contains(toBool, "case NumOrStrOrUndefUndef:") {
		t.Fatalf("sentinel arm should have no ToBoolean case\n%s", source)
	}
}

// TestUnionTruthyNegationAndTernary pins the other boolean positions: a ! and a
// ternary condition over a union each read through the same ToBoolean.
func TestUnionTruthyNegationAndTernary(t *testing.T) {
	const src = `function f(x: number | string | null): string {
  const label = x ? "y" : "n";
  if (!x) return "empty";
  return label;
}
f(1);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "if x.ToBoolean() {") {
		t.Fatalf("ternary condition did not lower to ToBoolean\n%s", source)
	}
	if !strings.Contains(source, "if !x.ToBoolean() {") {
		t.Fatalf("negation did not lower to ToBoolean\n%s", source)
	}
}

// TestUnionTruthyOptionalStaysOpt pins that a two-member optional keeps the leaner
// value.Opt truthiness path and does not route to a tagged-sum ToBoolean.
func TestUnionTruthyOptionalStaysOpt(t *testing.T) {
	const src = `function f(x: number | undefined): string {
  if (x) return "t";
  return "f";
}
f(1);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "ToBoolean()") {
		t.Fatalf("number | undefined should not emit a union ToBoolean\n%s", source)
	}
	if !strings.Contains(source, "IsUndefined()") {
		t.Fatalf("number | undefined should keep the value.Opt presence test\n%s", source)
	}
}

// TestUnionTruthyRun builds and runs a program that reads a number | string |
// undefined and a number | string | null in boolean position across an if, a ternary,
// and a negation, so the ToBoolean method compiles and reproduces the JavaScript falsy
// set: undefined and null are falsy, a zero and an empty string are falsy, and any
// other number or string is truthy.
func TestUnionTruthyRun(t *testing.T) {
	skipIfShort(t)
	src := `
function classify(x: number | string | undefined): string {
  if (x) return "truthy";
  return "falsy";
}
console.log(classify(1));
console.log(classify(0));
console.log(classify("hi"));
console.log(classify(""));
console.log(classify(undefined));
function label(x: number | string | null): string {
  return !x ? "empty" : "full";
}
console.log(label(3));
console.log(label(0));
console.log(label(null));
console.log(label("x"));
`
	got := runProgramGo(t, src)
	want := "truthy\nfalsy\ntruthy\nfalsy\nfalsy\nfull\nempty\nempty\nfull\n"
	if got != want {
		t.Fatalf("union truthy run mismatch:\n got %q\nwant %q", got, want)
	}
}
