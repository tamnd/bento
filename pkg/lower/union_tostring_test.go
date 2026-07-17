package lower

import (
	"strings"
	"testing"
)

// TestUnionToStringEmitsMethod pins that a tagged-sum union coerced to a string
// lowers to a ToString method call and the union grows that method, each arm
// switching on the tag to its JavaScript string form: a number through
// value.NumberToString, a string as itself, and the undefined sentinel as the
// literal "undefined".
func TestUnionToStringEmitsMethod(t *testing.T) {
	const src = `function f(x: number | string | undefined): string {
  return String(x);
}
f(1);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"func (u NumOrStrOrUndef) ToString() value.BStr {",
		"case NumOrStrOrUndefNum:",
		"return value.NumberToString(u.num)",
		"case NumOrStrOrUndefStr:",
		"return u.str",
		"case NumOrStrOrUndefUndef:",
		`return value.FromGoString("undefined")`,
		"return x.ToString()",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("emitted Go missing %q\n%s", want, source)
		}
	}
}

// TestUnionToStringConcatAndTemplate pins the other two coercion positions: a
// template substitution and a string concatenation over a union each read through
// the same ToString the String() call takes, so the three paths agree.
func TestUnionToStringConcatAndTemplate(t *testing.T) {
	const tmpl = `function f(x: number | string | null): string {
  return ` + "`v:${x}`" + `;
}
f(1);
`
	if source := renderProgram(t, tmpl); !strings.Contains(source, "x.ToString()") {
		t.Fatalf("template substitution did not lower to ToString\n%s", source)
	}
	const concat = `function f(x: number | string | null): string {
  return "v:" + x;
}
f(1);
`
	if source := renderProgram(t, concat); !strings.Contains(source, "x.ToString()") {
		t.Fatalf("concatenation did not lower to ToString\n%s", source)
	}
}

// TestUnionToStringBigIntArmHandsBack pins the ceiling: a union carrying a bigint
// arm has no ToString case this slice spells, so unionToStringSupported reports false
// and String() over it keeps the handback rather than emit a method with a missing
// arm.
func TestUnionToStringBigIntArmHandsBack(t *testing.T) {
	const src = `function f(x: number | bigint): string {
  return String(x);
}
f(1);
`
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "string") {
		t.Fatalf("reason = %q, want a string-coercion handback", reason)
	}
}

// TestUnionToStringRun builds and runs the three coercion positions over a
// number/string/undefined union and a number/boolean/null union against the oracle,
// so a number arm renders through Number::toString, a string as itself, a boolean as
// "true"/"false", and the undefined and null sentinels as "undefined" and "null".
func TestUnionToStringRun(t *testing.T) {
	skipIfShort(t)
	src := `
function viaString(x: number | string | undefined): string { return String(x); }
function viaTemplate(x: number | string | undefined): string { return ` + "`[${x}]`" + `; }
function viaConcat(x: number | boolean | null): string { return "v=" + x; }
console.log(viaString(42));
console.log(viaString("hi"));
console.log(viaString(undefined));
console.log(viaTemplate(7));
console.log(viaTemplate(undefined));
console.log(viaConcat(3));
console.log(viaConcat(true));
console.log(viaConcat(false));
console.log(viaConcat(null));
`
	got := runProgramGo(t, src)
	want := "42\nhi\nundefined\n[7]\n[undefined]\nv=3\nv=true\nv=false\nv=null\n"
	if got != want {
		t.Fatalf("union ToString run mismatch:\n got %q\nwant %q", got, want)
	}
}
