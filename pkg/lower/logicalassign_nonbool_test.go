package lower

import (
	"strings"
	"testing"
)

// TestLogicalAssignNonBoolEmits pins that ||= and &&= on a non-boolean target guard
// the assignment with the target's JavaScript truthiness rather than a boolean: a
// number tests against zero and NaN, a string against empty, and an object is always
// truthy so its &&= collapses to an unconditional assign.
func TestLogicalAssignNonBoolEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"number ||= guards on the falsy test",
			"function f(x: number): number { x ||= 7; return x; }\nconsole.log(f(0));\n",
			"if !(x != 0 && x == x) {",
		},
		{
			"string ||= guards on the empty test",
			"function f(s: string): string { s ||= \"z\"; return s; }\nconsole.log(f(\"\"));\n",
			"if !(s.Length() > 0) {",
		},
		{
			"object &&= is always truthy so it assigns unconditionally",
			"function f(o: { a: number }): number { o &&= { a: 9 }; return o.a; }\nconsole.log(f({ a: 1 }));\n",
			"if true {",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("logical assignment did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestLogicalAssignNonBoolRuns builds and runs ||= and &&= on non-boolean targets
// and matches the truthiness contract: a zero and an empty string trigger ||=, a
// non-zero triggers &&=, and an object always triggers &&= since it is truthy.
func TestLogicalAssignNonBoolRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function orNum(x: number): number {
  x ||= 7;
  return x;
}
function andNum(x: number): number {
  x &&= 7;
  return x;
}
function orStr(s: string): string {
  s ||= "fallback";
  return s;
}
console.log(orNum(0));
console.log(orNum(3));
console.log(andNum(0));
console.log(andNum(5));
console.log(orStr(""));
`
	got := runProgramGo(t, src)
	want := "7\n3\n0\n7\nfallback\n"
	if got != want {
		t.Fatalf("logical assignment program printed %q, want %q", got, want)
	}
}
