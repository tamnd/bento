package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestEnumEmits pins the shape of a plain numeric enum: a float64-backed constant
// per member, explicit initializers kept and the gaps auto-incremented, and a
// member read resolving to the member's Go constant.
func TestEnumEmits(t *testing.T) {
	const src = `enum Color { Red, Green = 5, Blue }
console.log(Color.Red, Color.Green, Color.Blue);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"const (",
		"ColorRed",
		"ColorGreen float64 = 5",
		"float64 = 0",
		"float64 = 6",
		"value.NumberToString(ColorRed)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("enum emit did not print %q:\n%s", want, source)
		}
	}
}

// TestEnumNegativeMembers pins the negative-sentinel form: a member initialized to
// a negated literal folds to the negative value, and the auto-increment resumes one
// past it, so A=-2 gives B=-1 and C=0.
func TestEnumNegativeMembers(t *testing.T) {
	const src = `enum Signed { A = -2, B, C }
console.log(Signed.A, Signed.B, Signed.C);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"SignedA float64 = -2",
		"SignedB float64 = -1",
		"SignedC float64 = 0",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("negative enum emit did not print %q:\n%s", want, source)
		}
	}
}

// TestEnumTypeIsFloat64 pins that an enum-typed annotation lowers to float64: a
// parameter and a return declared with the enum name become float64, since a
// registered numeric enum rides the number path the checker already gives its
// members.
func TestEnumTypeIsFloat64(t *testing.T) {
	const src = `enum E { X, Y }
function id(e: E): E { return e; }
console.log(id(E.X));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Id(e float64) float64") {
		t.Errorf("enum-typed signature did not lower to float64:\n%s", source)
	}
}

// TestConstEnumInlines pins that a const enum emits no package-level constants and
// inlines each member's value at the use site, the erasure TypeScript itself
// performs, so an expression over its members lowers to the plain numeric literals.
func TestConstEnumInlines(t *testing.T) {
	const src = `const enum Dir { Up = 1, Down = 2 }
console.log(Dir.Up + Dir.Down);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "DirUp") || strings.Contains(source, "DirDown") {
		t.Errorf("const enum leaked a member constant, want it inlined:\n%s", source)
	}
	if !strings.Contains(source, "1 + 2") {
		t.Errorf("const enum did not inline its member values:\n%s", source)
	}
}

// TestEnumHandsBack pins the boundaries of the numeric subset: a string enum and a
// member whose initializer references another member are both a later slice, so
// each hands the unit back with a named reason rather than emit a shape this file
// does not build.
func TestEnumHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"stringEnum",
			"enum S { A = \"x\", B = \"y\" }\nconsole.log(1);\n",
			"not a numeric literal",
		},
		{
			"memberReferenceInitializer",
			"enum C { A = 1, B = A }\nconsole.log(1);\n",
			"not a numeric literal",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := compile(t, tc.src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, tc.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", nyl.Reason, tc.want)
			}
		})
	}
}

// TestEnumRuns builds and runs enums end to end and matches the Node oracle: a
// plain enum's explicit and auto-incremented members, a negative sentinel, an
// enum-typed round trip through a function, a const enum's inlined arithmetic, and
// a member compared against its numeric value.
func TestEnumRuns(t *testing.T) {
	skipIfShort(t)
	const src = `enum Color { Red, Green = 5, Blue }
enum Signed { A = -2, B, C }
const enum Dir { Up = 1, Down = 2 }
function label(c: Color): Color {
  return c;
}
console.log(Color.Red, Color.Green, Color.Blue);
console.log(Signed.A, Signed.B, Signed.C);
console.log(label(Color.Blue));
console.log(Dir.Up, Dir.Down);
console.log(Color.Green === 5);
`
	got := runProgramGo(t, src)
	want := "0 5 6\n" +
		"-2 -1 0\n" +
		"6\n" +
		"1 2\n" +
		"true\n"
	if got != want {
		t.Fatalf("enum program printed %q, want %q", got, want)
	}
}
