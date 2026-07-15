package lower

import (
	"strings"
	"testing"
)

// An object literal lowered against a contextual struct shape builds each member at the
// field's declared shape. When that field is a dynamic value.Value slot, which the inferred
// shape takes for an untyped destructured binding, a static member value has to box into the
// slot the way an argument crossing into an any parameter does. Without the coercion a
// { a: 1 } for a value.Value field emitted the untyped A: 1, which does not compile.

// TestObjectLiteralBoxesDynamicField proves the fixed reproducer runs: an untyped
// destructured parameter with a default beside it infers a fixed shape whose non-defaulted
// field is dynamic, and the { a: 1 } argument boxes into it. The program compiles and prints
// the sum a + b with b taking its default.
func TestObjectLiteralBoxesDynamicField(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"number into dynamic field",
			"function f({ a, b = 5 }) { return a + b; }\nconsole.log(f({ a: 1 }));\n",
			"6\n",
		},
		{
			"string into dynamic field",
			"function f({ a, b = \"z\" }) { return a + b; }\nconsole.log(f({ a: \"x\" }));\n",
			"xz\n",
		},
		{
			"both members supplied",
			"function f({ a, b = 5 }) { return a + b; }\nconsole.log(f({ a: 2, b: 3 }));\n",
			"5\n",
		},
		{
			"nested object into dynamic field",
			"function g({ a, b = 0 }) { return a.p + b; }\nconsole.log(g({ a: { p: 3 } }));\n",
			"3\n",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := runProgramGoTolerant(t, tc.src)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestObjectLiteralBoxesDynamicFieldEmitsBox proves the member value is boxed into the
// value.Value slot rather than left as the untyped constant that would not compile.
func TestObjectLiteralBoxesDynamicFieldEmitsBox(t *testing.T) {
	source := renderProgramTolerant(t, "function f({ a, b = 5 }) { return a + b; }\nconsole.log(f({ a: 1 }));\n")
	if !strings.Contains(source, "A: value.Number(1)") {
		t.Fatalf("the member value was not boxed into the dynamic field, want `A: value.Number(1)`:\n%s", source)
	}
}
